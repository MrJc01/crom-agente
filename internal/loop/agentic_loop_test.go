package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/crom/crom-agente/internal/llm"
	"github.com/crom/crom-agente/internal/state"
	"github.com/crom/crom-agente/internal/tools"
)

const (
	MaxIterations          = 15
	MaxConsecutiveFailures = 3
)


// --- Mock Tool para testes ---

type mockTool struct {
	id              string
	description     string
	approve         bool
	executeFunc     func(ctx context.Context, args json.RawMessage) (tools.Result, error)
}

func (m *mockTool) ID() string                         { return m.id }
func (m *mockTool) Description() string                { return m.description }
func (m *mockTool) ParametersSchema() json.RawMessage   { return json.RawMessage(`{"type":"object"}`) }
func (m *mockTool) RequiresApproval() bool              { return m.approve }
func (m *mockTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, args)
	}
	return tools.Result{Success: true, Data: "ok"}, nil
}

// --- Mock EventHandler para capturar eventos ---

type testEventHandler struct {
	StatusChanges []string
	Messages      []struct{ Role, Content string }
}

func (h *testEventHandler) OnStatusChange(s string) {
	h.StatusChanges = append(h.StatusChanges, s)
}
func (h *testEventHandler) OnMessage(role, content string) {
	h.Messages = append(h.Messages, struct{ Role, Content string }{role, content})
}
func (h *testEventHandler) OnEvent(event AgentEvent) {}

// --- Testes ---

func TestAgenticLoop_SimpleTextResponse(t *testing.T) {
	provider := llm.NewMockProvider(
		llm.MockTextResponse("Olá! Tarefa concluída.", 100),
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

func TestAgenticLoop_ToolCallSuccess(t *testing.T) {
	provider := llm.NewMockProvider(
		// 1ª: LLM pede read_file
		llm.MockToolCallResponse("read_file", `{"path":"/tmp/test.txt"}`, 200),
		// 2ª: LLM responde com texto (entra em fase de verificação)
		llm.MockTextResponse("Arquivo lido com sucesso.", 150),
		// 3ª: Fase de verificação concluída
		llm.MockTextResponse("Verificação ok.", 50),
	)
	sm := state.NewStateManager(t.TempDir())
	handler := &testEventHandler{}

	al := New(provider, sm, handler)
	al.RegisterTool(&mockTool{
		id:          "read_file",
		description: "Lê um arquivo do disco",
		executeFunc: func(ctx context.Context, args json.RawMessage) (tools.Result, error) {
			return tools.Result{Success: true, Data: "hello world"}, nil
		},
	})

	err := al.Execute(context.Background(), "Leia o arquivo /tmp/test.txt")
	if err != nil {
		t.Fatalf("esperado sucesso, obteve erro: %v", err)
	}
	if provider.TotalCalls() != 3 {
		t.Fatalf("esperado 3 chamadas ao LLM (tool+verify+final), obteve %d", provider.TotalCalls())
	}
}

func TestAgenticLoop_ToolCallFailure(t *testing.T) {
	provider := llm.NewMockProvider(
		// 1ª: LLM pede read_file
		llm.MockToolCallResponse("read_file", `{"path":"/nope"}`, 200),
		// 2ª: LLM responde com texto (entra em verificação)
		llm.MockTextResponse("O arquivo não foi encontrado.", 100),
		// 3ª: Fase de verificação
		llm.MockTextResponse("Nada mais a fazer.", 50),
	)
	sm := state.NewStateManager(t.TempDir())
	handler := &testEventHandler{}

	al := New(provider, sm, handler)
	al.RegisterTool(&mockTool{
		id:          "read_file",
		description: "Lê um arquivo",
		executeFunc: func(ctx context.Context, args json.RawMessage) (tools.Result, error) {
			return tools.Result{Success: false, Error: "file not found"}, nil
		},
	})

	err := al.Execute(context.Background(), "Leia /nope")
	if err != nil {
		t.Fatalf("esperado sucesso (loop encerrou), obteve erro: %v", err)
	}
}

func TestAgenticLoop_ToolNotFound(t *testing.T) {
	provider := llm.NewMockProvider(
		// LLM pede ferramenta inexistente
		llm.MockToolCallResponse("delete_universe", `{}`, 200),
		// LLM reconhece o erro (entra em verificação pois tool foi executada)
		llm.MockTextResponse("Ok, ferramenta não disponível.", 50),
		// Fase de verificação
		llm.MockTextResponse("Nada mais a fazer.", 30),
	)
	sm := state.NewStateManager(t.TempDir())
	handler := &testEventHandler{}

	al := New(provider, sm, handler)
	err := al.Execute(context.Background(), "Delete tudo")

	if err != nil {
		t.Fatalf("esperado sucesso, obteve erro: %v", err)
	}

	// Verifica que houve mensagem de ferramenta não encontrada
	found := false
	for _, m := range handler.Messages {
		if strings.Contains(m.Content, "não encontrada") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("esperado mensagem de ferramenta não encontrada nos eventos")
	}
}

func TestAgenticLoop_EmptyResponseAutoCorrection(t *testing.T) {
	provider := llm.NewMockProvider(
		// 1ª: resposta vazia
		llm.MockEmptyResponse(),
		// 2ª: resposta vazia novamente
		llm.MockEmptyResponse(),
		// 3ª: resposta vazia (3ª falha consecutiva → abort)
		llm.MockEmptyResponse(),
	)
	sm := state.NewStateManager(t.TempDir())

	al := New(provider, sm, nil)
	err := al.Execute(context.Background(), "Faça algo")

	if err == nil {
		t.Fatal("esperado erro por falhas consecutivas, obteve nil")
	}
	if !strings.Contains(err.Error(), "falhas consecutivas") {
		t.Fatalf("erro inesperado: %v", err)
	}
}

func TestAgenticLoop_LeakedToolCallAutoCorrection(t *testing.T) {
	provider := llm.NewMockProvider(
		// 1ª: LLM vaza tool call no texto
		llm.MockTextResponse(`Vou usar write_file para criar o arquivo {"name":"test"}`, 100),
		// 2ª: LLM corrige e faz texto normal
		llm.MockTextResponse("Desculpe, tarefa concluída sem necessidade de edição.", 80),
	)
	sm := state.NewStateManager(t.TempDir())
	handler := &testEventHandler{}

	al := New(provider, sm, handler)
	err := al.Execute(context.Background(), "Crie um arquivo")

	if err != nil {
		t.Fatalf("esperado sucesso, obteve erro: %v", err)
	}
	if provider.TotalCalls() != 2 {
		t.Fatalf("esperado 2 chamadas (1 autocorreção + 1 final), obteve %d", provider.TotalCalls())
	}
}

func TestAgenticLoop_VerificationPhase(t *testing.T) {
	provider := llm.NewMockProvider(
		// 1ª: LLM pede write_file
		llm.MockToolCallResponse("write_file", `{"path":"/tmp/a.go","content":"package main"}`, 300),
		// 2ª: LLM responde com texto (achou que terminou) → deve entrar em verificação
		llm.MockTextResponse("Arquivo criado com sucesso.", 100),
		// 3ª: LLM executa verificação e confirma
		llm.MockTextResponse("Verificação completa. Tudo certo.", 80),
	)
	sm := state.NewStateManager(t.TempDir())
	handler := &testEventHandler{}

	al := New(provider, sm, handler)
	al.RegisterTool(&mockTool{id: "write_file", description: "Escreve arquivo"})

	err := al.Execute(context.Background(), "Crie o arquivo a.go")
	if err != nil {
		t.Fatalf("esperado sucesso, obteve erro: %v", err)
	}
	if provider.TotalCalls() != 3 {
		t.Fatalf("esperado 3 chamadas (tool + texto + verificação), obteve %d", provider.TotalCalls())
	}

	// Verificar que entrou na fase de verificação
	found := false
	for _, m := range handler.Messages {
		if strings.Contains(m.Content, "verificação") || strings.Contains(m.Content, "Verificação") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("esperado mensagem sobre fase de verificação nos eventos")
	}
}

func TestAgenticLoop_RepetitiveLoopDetection(t *testing.T) {
	// Cenário: LLM fica chamando a mesma tool com os mesmos args repetidamente.
	// A detecção de loop olha para as mensagens do assistant repetidas.
	// Precisamos que o loop rode iterações suficientes para o detector disparar.
	provider := llm.NewMockProvider(
		// Iterações 1-3: tool calls iguais (conteúdo do assistant é vazio pois são tool calls)
		llm.MockToolCallResponse("read_file", `{"path":"/tmp/a.txt"}`, 50),
		llm.MockToolCallResponse("read_file", `{"path":"/tmp/a.txt"}`, 50),
		llm.MockToolCallResponse("read_file", `{"path":"/tmp/a.txt"}`, 50),
		// Iteração 4: finalmente responde com texto (após loop warning injetado)
		llm.MockTextResponse("Ok, mudei de estratégia.", 50),
		// Iteração 5: verificação
		llm.MockTextResponse("Verificação ok.", 50),
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
	if err != nil {
		t.Fatalf("esperado sucesso eventual, obteve erro: %v", err)
	}

	// Deve ter injetado aviso de loop repetitivo
	found := false
	for _, m := range handler.Messages {
		if strings.Contains(m.Content, "Loop repetitivo") || strings.Contains(m.Content, "REPETITIVE_LOOP") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("esperado aviso de loop repetitivo nos eventos")
	}
}

func TestAgenticLoop_LLMError(t *testing.T) {
	provider := llm.NewMockProvider(
		llm.MockErrorResponse("connection refused"),
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
	provider := llm.NewMockProvider(
		llm.MockTextResponse("nunca deveria chegar aqui", 100),
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

func TestAgenticLoop_MaxIterationsExceeded(t *testing.T) {
	// Gera respostas infinitas com tool calls para esgotar o limite
	responses := make([]llm.MockResponse, MaxIterations+1)
	for i := range responses {
		responses[i] = llm.MockToolCallResponse("echo", `{"msg":"loop"}`, 10)
	}

	provider := llm.NewMockProvider(responses...)
	sm := state.NewStateManager(t.TempDir())

	al := New(provider, sm, nil)
	al.RegisterTool(&mockTool{id: "echo", description: "Ecoa texto"})

	err := al.Execute(context.Background(), "Loop infinito")
	if err == nil {
		t.Fatal("esperado erro de limite de iterações, obteve nil")
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("limite de %d iterações", MaxIterations)) {
		t.Fatalf("erro inesperado: %v", err)
	}
}

func TestAgenticLoop_ConsecutiveToolFailuresAbort(t *testing.T) {
	responses := make([]llm.MockResponse, MaxConsecutiveFailures+1)
	for i := range responses {
		responses[i] = llm.MockToolCallResponse("bad_tool", `{}`, 10)
	}
	provider := llm.NewMockProvider(responses...)
	sm := state.NewStateManager(t.TempDir())

	al := New(provider, sm, nil)
	al.RegisterTool(&mockTool{
		id:          "bad_tool",
		description: "Sempre falha",
		executeFunc: func(ctx context.Context, args json.RawMessage) (tools.Result, error) {
			return tools.Result{Success: false, Error: "always fails"}, nil
		},
	})

	err := al.Execute(context.Background(), "Use bad_tool")
	if err == nil {
		t.Fatal("esperado erro por falhas consecutivas, obteve nil")
	}
	if !strings.Contains(err.Error(), "falhas consecutivas") {
		t.Fatalf("erro inesperado: %v", err)
	}
}

func TestCompactMessages(t *testing.T) {
	// Cria 50 mensagens (acima do limite de 40)
	msgs := make([]llm.Message, 50)
	for i := range msgs {
		msgs[i] = llm.Message{Role: "user", Content: fmt.Sprintf("msg-%d", i)}
	}

	compacted := compactMessages(msgs)
	if len(compacted) != 40 {
		t.Fatalf("esperado 40 mensagens após compactação, obteve %d", len(compacted))
	}

	// A primeira mensagem deve ser preservada
	if compacted[0].Content != "msg-0" {
		t.Fatalf("primeira mensagem não preservada: %s", compacted[0].Content)
	}

	// A última mensagem deve ser a mais recente
	if compacted[len(compacted)-1].Content != "msg-49" {
		t.Fatalf("última mensagem não preservada: %s", compacted[len(compacted)-1].Content)
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
	}

	if !detectRepetitiveLoop(msgs) {
		t.Fatal("esperado detecção de loop repetitivo")
	}

	msgs[5].Content = "different thing"
	if detectRepetitiveLoop(msgs) {
		t.Fatal("não deveria detectar loop com mensagens diferentes")
	}
}
