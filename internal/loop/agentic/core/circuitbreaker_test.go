package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/crom/crom-agente/internal/config"
	"github.com/crom/crom-agente/internal/llm"
	"github.com/crom/crom-agente/internal/llm/providers"
	"github.com/crom/crom-agente/internal/state"
	"github.com/crom/crom-agente/internal/tools"
)

func TestAgenticLoop_RepetitiveLoopDetection(t *testing.T) {
	// Cenário: LLM fica chamando a mesma tool com os mesmos args repetidamente.
	// A detecção de loop olha para as mensagens do assistant repetidas.
	// Precisamos que o loop rode iterações suficientes para o detector disparar.
	provider := providers.NewMockProvider(
		// Iterações 1-3: tool calls iguais (conteúdo do assistant é vazio pois são tool calls)
		providers.MockToolCallResponse("read_file", `{"path":"/tmp/a.txt"}`, 50),
		providers.MockToolCallResponse("read_file", `{"path":"/tmp/a.txt"}`, 50),
		providers.MockToolCallResponse("read_file", `{"path":"/tmp/a.txt"}`, 50),
		// Iteração 4: finalmente responde com texto (após loop warning injetado)
		providers.MockTextResponse("Ok, mudei de estratégia.", 50),
	)
	sm := state.NewStateManager(t.TempDir())
	handler := &testEventHandler{}

	al := New(provider, sm, handler)
	al.RegisterTool(&mockTool{
		id:          "read_file",
		description: "Lê arquivo",
		executeFunc: func(ctx context.Context, args json.RawMessage) (tools.Result, error) {
			return tools.Result{Success: true, Data: "same content"}, nil
		},
	})

	err := al.Execute(context.Background(), "Analise o código")
	if err == nil {
		t.Fatalf("esperado erro de loop repetitivo, obteve sucesso")
	}
	if !strings.Contains(err.Error(), "loop repetitivo detectado") {
		t.Fatalf("erro inesperado: %v", err)
	}
}

func TestAgenticLoop_MaxIterationsExceeded(t *testing.T) {
	// Gera respostas infinitas com tool calls para esgotar o limite
	responses := make([]providers.MockResponse, MaxIterations+1)
	for i := range responses {
		responses[i] = providers.MockToolCallResponse("echo", fmt.Sprintf(`{"msg":"loop %d"}`, i), 10)
	}

	provider := providers.NewMockProvider(responses...)
	sm := state.NewStateManager(t.TempDir())

	al := New(provider, sm, nil, &config.ResolvedConfig{
		MaxIterations:     15,
		MaxMessageHistory: 100,
	})
	al.RegisterTool(&mockTool{id: "echo", description: "Ecoa texto"})

	err := al.Execute(context.Background(), "Loop infinito")
	if err == nil {
		t.Fatal("esperado erro de limite de iterações, obteve nil")
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("limite de %d iterações", MaxIterations)) {
		t.Fatalf("erro inesperado: %v", err)
	}
}

func TestDetectRepetitiveLoop(t *testing.T) {
	msgs := []llm.Message{
		{Role: "user", Content: "start"},
		{Role: "assistant", Content: "thinking..."},
		{Role: "user", Content: "ok"},
		{Role: "assistant", Content: "same thing"},
		{Role: "user", Content: "continue"},
		{Role: "assistant", Content: "same thing"},
		{Role: "user", Content: "continue again"},
		{Role: "assistant", Content: "same thing"},
	}

	if !DetectRepetitiveLoop(msgs) {
		t.Fatal("esperado detecção de loop repetitivo direto")
	}

	msgs[7].Content = "different thing"
	if DetectRepetitiveLoop(msgs) {
		t.Fatal("não deveria detectar loop com mensagens diferentes")
	}

	// Teste de loop de oscilação A -> B -> A -> B
	oscillationMsgs := []llm.Message{
		{Role: "user", Content: "start"},
		{Role: "assistant", Content: "Action A"},
		{Role: "tool", Content: "Result A"},
		{Role: "assistant", Content: "Action B"},
		{Role: "tool", Content: "Result B"},
		{Role: "assistant", Content: "Action A"},
		{Role: "tool", Content: "Result A"},
		{Role: "assistant", Content: "Action B"},
	}

	if !DetectRepetitiveLoop(oscillationMsgs) {
		t.Fatal("esperado detecção de loop de oscilação A -> B -> A -> B")
	}
}

func TestAgenticLoop_CircuitBreaker(t *testing.T) {
	// 3 responses with text only (no tools), for a task that requires file changes.
	resp1 := providers.MockTextResponse("Estou pensando sobre o arquivo...", 10)
	resp2 := providers.MockTextResponse("Ainda pensando em como criar o arquivo...", 10)
	resp3 := providers.MockTextResponse("Quase terminando de planejar o arquivo...", 10)
	resp4 := providers.MockTextResponse("Decidindo a estrutura final...", 10)
	resp5 := providers.MockTextResponse("Pronto para começar...", 10)

	provider := providers.NewMockProvider(resp1, resp2, resp3, resp4, resp5)
	sm := state.NewStateManager(t.TempDir())
	_ = sm.SetPlan([]state.TaskItem{
		{
			Title:  "Criar o arquivo script.py",
			Status: "pending",
		},
	})
	handler := &testEventHandler{}
	cfg := &config.ResolvedConfig{
		MaxIterations:             5, // Limit iterations to 5 so it finishes quickly
		MaxConsecutiveFail:        3,
		DisablePromptOptimization: true,
	}

	al := New(provider, sm, handler, cfg)

	err := al.Execute(context.Background(), "Crie o arquivo script.py")
	if err == nil {
		t.Fatal("esperava erro de limite de iterações atingido, mas o loop concluiu com sucesso")
	}

	if !strings.Contains(err.Error(), "limite de 5 iterações atingido") {
		t.Errorf("mensagem de erro inesperada: %v", err)
	}

	// Verifica se o evento de aviso (warning) do circuit breaker foi emitido
	hasCircuitBreakerWarning := false
	for _, ev := range handler.Events {
		if ev.Event == "warning" {
			dataMap := ev.Data
			msgStr, ok := dataMap["message"].(string)
			if ok && strings.Contains(msgStr, "circuit breaker triggered") {
				hasCircuitBreakerWarning = true
				break
			}
		}
	}

	if !hasCircuitBreakerWarning {
		t.Fatal("esperava que o handler recebesse o evento de aviso (warning) correspondente ao circuit breaker")
	}

	// Verifica se a mensagem de aviso do sistema foi adicionada ao histórico
	fmt.Printf("MESSAGES DUMP:\n")
        for _, msg := range sm.GetMessages() {
            fmt.Printf("Role: %s, Content: %s\n", msg.Role, msg.Content)
        }
        foundSystemWarning := false
	for _, msg := range sm.GetMessages() {
		if msg.Role == "system" && strings.Contains(msg.Content, "Você está há 3 turnos sem chamar ferramentas") {
			foundSystemWarning = true
			break
		}
	}

	if !foundSystemWarning {
		t.Fatal("esperava que o histórico de mensagens contivesse o aviso de inatividade de ferramentas")
	}
}

func TestAgenticLoop_CircuitBreaker_ReadOnly(t *testing.T) {
	// 3 responses with tool calls, but they are all read-only (e.g. read_file)
	responses := make([]providers.MockResponse, 10)
	for i := range responses {
		responses[i] = providers.MockToolCallResponse("read_file", fmt.Sprintf(`{"path":"somefile%d.txt"}`, i), 10)
	}
	
	

	provider := providers.NewMockProvider(responses...)
	sm := state.NewStateManager(t.TempDir())
	_ = sm.SetPlan([]state.TaskItem{
		{
			Title:  "Modificar arquivo.py",
			Status: "pending",
		},
	})
	handler := &testEventHandler{}
	cfg := &config.ResolvedConfig{
		MaxIterations:             5, // Limit iterations to 5 so it finishes quickly
		MaxConsecutiveFail:        3,
		DisablePromptOptimization: true,
	}

	al := New(provider, sm, handler, cfg)
	
	// mock read_file tool to succeed
	al.RegisterTool(&mockTool{
		id: "read_file",
		executeFunc: func(ctx context.Context, args json.RawMessage) (tools.Result, error) {
			return tools.Result{Success: true, Data: "file content"}, nil
		},
	})

	err := al.Execute(context.Background(), "Modifique o arquivo.py")
	if err == nil {
		t.Fatal("esperava erro de limite de iterações atingido, mas o loop concluiu com sucesso")
	}

	// Verifica se a mensagem de aviso de arquivos inalterados foi adicionada ao histórico
	fmt.Printf("MESSAGES DUMP:\n")
        for _, msg := range sm.GetMessages() {
            fmt.Printf("Role: %s, Content: %s\n", msg.Role, msg.Content)
        }
        foundSystemWarning := false
	for _, msg := range sm.GetMessages() {
		if msg.Role == "system" && strings.Contains(msg.Content, "sem modificar arquivos ou chamar ferramentas de escrita/execução") {
			foundSystemWarning = true
			break
		}
	}

	if !foundSystemWarning {
		t.Fatal("esperava que o histórico de mensagens contivesse o aviso de inatividade de escrita de arquivos")
	}
}


