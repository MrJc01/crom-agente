package core

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/crom/crom-agente/internal/config"
	"github.com/crom/crom-agente/internal/llm"
	"github.com/crom/crom-agente/internal/llm/providers"
	"github.com/crom/crom-agente/internal/state"
	"github.com/crom/crom-agente/internal/tools"
)

func TestAgenticLoop_SimpleTextResponse(t *testing.T) {
	provider := providers.NewMockProvider(
		providers.MockTextResponse("Olá! Tarefa concluída.", 100),
	)
	sm := state.NewStateManager(t.TempDir())
	handler := &testEventHandler{}

	al := New(provider, sm, handler)
	err := al.Execute(context.Background(), "Diga olá")

	if err != nil {
		t.Fatalf("esperado sucesso, obteve erro: %v", err)
	}
	if provider.TotalCalls() != 1 {
		t.Fatalf("esperado 1 chamada ao LLM, obteve %d", provider.TotalCalls())
	}

	// Verifica que o status final é "finished"
	lastStatus := handler.StatusChanges[len(handler.StatusChanges)-1]
	if lastStatus != "finished" {
		t.Fatalf("esperado status final 'finished', obteve '%s'", lastStatus)
	}
}

func TestAgenticLoop_EmptyResponseAutoCorrection(t *testing.T) {
	provider := providers.NewMockProvider(
		// 1ª: resposta vazia
		providers.MockEmptyResponse(),
		// 2ª: resposta vazia novamente
		providers.MockEmptyResponse(),
		// 3ª: resposta vazia (3ª falha consecutiva → retry com cancelamento)
		providers.MockEmptyResponse(),
	)
	sm := state.NewStateManager(t.TempDir())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := &cancelOnRetryHandler{
		cancelFunc: cancel,
	}

	al := New(provider, sm, handler)
	al.failureRetryDelay = 1 * time.Millisecond
	err := al.Execute(ctx, "Faça algo")

	if err == nil {
		t.Fatal("esperado erro por cancelamento de contexto, obteve nil")
	}
	if err != context.Canceled && !strings.Contains(err.Error(), "canceled") && !strings.Contains(err.Error(), "cancelado") {
		t.Fatalf("erro inesperado: %v", err)
	}

	// Deve ter emitido o evento retry
	hasRetry := false
	for _, ev := range handler.Events {
		if ev.Event == "retry" {
			hasRetry = true
			if ev.Data["reason"] != "consecutive_failures" {
				t.Fatalf("esperado reason=consecutive_failures, obteve %v", ev.Data["reason"])
			}
			if ev.Data["error_type"] != "empty_llm_response" {
				t.Fatalf("esperado error_type=empty_llm_response, obteve %v", ev.Data["error_type"])
			}
		}
	}
	if !hasRetry {
		t.Fatal("esperado evento 'retry' no handler")
	}
}

func TestAgenticLoop_LLMError(t *testing.T) {
	provider := providers.NewMockProvider(
		providers.MockErrorResponse("connection refused"),
	)
	sm := state.NewStateManager(t.TempDir())

	al := New(provider, sm, nil)
	err := al.Execute(context.Background(), "Qualquer coisa")

	if err == nil {
		t.Fatal("esperado erro, obteve nil")
	}
	if !strings.Contains(err.Error(), "falha na chamada ao LLM") {
		t.Fatalf("erro inesperado: %v", err)
	}
}

func TestAgenticLoop_ContextCancellation(t *testing.T) {
	// O LLM nunca responde porque o contexto é cancelado antes
	provider := providers.NewMockProvider(
		providers.MockTextResponse("nunca deveria chegar aqui", 100),
	)
	sm := state.NewStateManager(t.TempDir())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancela imediatamente

	al := New(provider, sm, nil)
	err := al.Execute(ctx, "Tarefa cancelada")

	if err == nil {
		t.Fatal("esperado erro de cancelamento, obteve nil")
	}
}

func TestAgenticLoop_PromptOptimization(t *testing.T) {
	provider := providers.NewMockProvider(
		// 1ª chamada: Otimização do prompt
		providers.MockTextResponse("PROMPT OTIMIZADO: crie uma API REST Go com cobertura de testes", 50),
		// 2ª chamada: Resposta do ReAct loop
		providers.MockTextResponse("Olá! Processando o prompt otimizado.", 100),
	)
	sm := state.NewStateManager(t.TempDir())
	handler := &testEventHandler{}

	al := New(provider, sm, handler, &config.ResolvedConfig{
		MaxIterations:             15,
		MaxConsecutiveFail:        3,
		ToolTimeoutSeconds:        30,
		MaxMessageHistory:         40,
		AutoVerify:                true,
		PermissionMode:            "scoped",
		DisablePromptOptimization: false,
	})
	err := al.Execute(context.Background(), "Crie api go")
	if err != nil {
		t.Fatalf("esperado sucesso, obteve erro: %v", err)
	}

	// O prompt original deve ter sido substituído pelo otimizado
	messages := sm.GetMessages()
	if len(messages) < 2 {
		t.Fatalf("esperado histórico de mensagens contendo o prompt otimizado, obteve tamanho %d", len(messages))
	}

	// A primeira mensagem de role user deve ser o prompt otimizado
	var userMsg llm.Message
	foundUser := false
	for _, m := range messages {
		if m.Role == "user" {
			userMsg = m
			foundUser = true
			break
		}
	}
	if !foundUser || !strings.Contains(userMsg.Content, "PROMPT OTIMIZADO") {
		t.Errorf("esperado prompt otimizado na primeira mensagem do usuário, obteve: %+v", userMsg)
	}
}

func TestAgenticLoop_SupportsSystemPromptFallback(t *testing.T) {
	provider := providers.NewMockProvider(
		providers.MockTextResponse("Processado com sucesso.", 100),
	)
	provider.DisableSystemPrompt = true // Simular que não suporta System Prompt!

	sm := state.NewStateManager(t.TempDir())
	handler := &testEventHandler{}

	al := New(provider, sm, handler, &config.ResolvedConfig{
		MaxIterations:             1,
		MaxConsecutiveFail:        1,
		ToolTimeoutSeconds:        30,
		MaxMessageHistory:         40,
		AutoVerify:                false,
		PermissionMode:            "scoped",
		DisablePromptOptimization: true,
	})

	err := al.Execute(context.Background(), "Tarefa de teste")
	if err != nil {
		t.Fatalf("esperado sucesso, obteve erro: %v", err)
	}

	// Verificar se o CallLog no provider não contém mensagens de sistema
	if len(provider.CallLog) == 0 {
		t.Fatalf("esperada pelo menos uma chamada ao LLM")
	}

	lastCallMsgs := provider.CallLog[0]
	for _, m := range lastCallMsgs {
		if m.Role == "system" {
			t.Errorf("encontrada mensagem com role system enviada ao LLM, mas o provedor declarou que não suporta!")
		}
	}

	// Verificar se as instruções de sistema foram mescladas no prompt do usuário
	var sentUserMsg llm.Message
	foundSentUser := false
	for _, m := range lastCallMsgs {
		if m.Role == "user" {
			sentUserMsg = m
			foundSentUser = true
			break
		}
	}

	if !foundSentUser {
		t.Fatalf("não foi encontrada nenhuma mensagem do usuário no payload enviado")
	}

	if !strings.Contains(sentUserMsg.Content, "=== INSTRUÇÕES DO SISTEMA ===") {
		t.Errorf("esperado que as instruções do sistema fossem mescladas no prompt do usuário, mas obteve: %s", sentUserMsg.Content)
	}

	if !strings.Contains(sentUserMsg.Content, "Tarefa de teste") {
		t.Errorf("esperado que a tarefa original do usuário estivesse presente na mensagem mesclada, mas obteve: %s", sentUserMsg.Content)
	}
}

func TestAgenticLoop_SimpleIntentFastPath(t *testing.T) {
	provider := providers.NewMockProvider(
		providers.MockTextResponse("Olá! Como posso ajudar você?", 10),
	)
	sm := state.NewStateManager(t.TempDir())
	handler := &testEventHandler{}

	al := New(provider, sm, handler)

	err := al.Execute(context.Background(), "olá")
	if err != nil {
		t.Fatalf("esperava sucesso no fast path, obteve: %v", err)
	}

	msgs := sm.GetMessages()
	if len(msgs) != 2 {
		t.Fatalf("esperava 2 mensagens salvas (user e assistant), obteve %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "olá" {
		t.Errorf("primeira mensagem incorreta: %+v", msgs[0])
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != "Olá! Como posso ajudar você?" {
		t.Errorf("segunda mensagem incorreta: %+v", msgs[1])
	}

	s := sm.GetState()
	if s.StatusOperacional != state.StatusIdle {
		t.Errorf("esperava status final 'idle', obteve '%s'", s.StatusOperacional)
	}

	if provider.TotalCalls() != 1 {
		t.Fatalf("esperava 1 chamada ao provider na primeira execução, obteve %d", provider.TotalCalls())
	}

	// Limpar mensagens do state manager para forçar nova detecção de simples intenção no loop
	_ = sm.SetMessages(nil)

	// Segunda execução: deve bater no cache
	err = al.Execute(context.Background(), "olá")
	if err != nil {
		t.Fatalf("esperava sucesso no fast path (segunda chamada), obteve: %v", err)
	}

	if provider.TotalCalls() != 1 {
		t.Fatalf("esperava que a segunda chamada batesse no cache e não incrementasse TotalCalls (esperava 1, obteve %d)", provider.TotalCalls())
	}
}

func TestAgenticLoop_TextOnlyModeFallback(t *testing.T) {
	mockResp1 := providers.MockTextResponse("Vou criar o arquivo index.html.\n```html\n<html><body>Hello</body></html>\n```", 100)
	mockResp1.Response.ToolUseDisabled = true

	mockResp2 := providers.MockTextResponse("Arquivo criado com sucesso. Concluí a tarefa.", 100)
	mockResp2.Response.ToolUseDisabled = true

	provider := providers.NewMockProvider(mockResp1, mockResp2)
	sm := state.NewStateManager(t.TempDir())
	handler := &testEventHandler{}
	cfg := &config.ResolvedConfig{
		MaxIterations:             5,
		MaxConsecutiveFail:        3,
		DisablePromptOptimization: true,
	}

	al := New(provider, sm, handler, cfg)

	var lastPath string
	var lastContent string
	al.RegisterTool(&mockTool{
		id:          "write_file",
		description: "Escreve em um arquivo",
		executeFunc: func(ctx context.Context, args json.RawMessage) (tools.Result, error) {
			var m map[string]string
			_ = json.Unmarshal(args, &m)
			lastPath = m["path"]
			lastContent = m["content"]
			return tools.Result{Success: true, Data: "arquivo gravado"}, nil
		},
	})

	err := al.Execute(context.Background(), "Crie index.html")
	if err != nil {
		t.Fatalf("erro inesperado no loop: %v", err)
	}

	if lastPath != "index.html" {
		t.Errorf("esperava gravar em index.html, obteve: %s", lastPath)
	}
	if lastContent != "<html><body>Hello</body></html>" {
		t.Errorf("conteúdo incorreto gravado: %q", lastContent)
	}
}

