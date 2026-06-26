package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crom/crom-agente/internal/config"
	"github.com/crom/crom-agente/internal/llm"
	"github.com/crom/crom-agente/internal/llm/providers"
	"github.com/crom/crom-agente/internal/loop"
	"github.com/crom/crom-agente/internal/state"
	"github.com/crom/crom-agente/internal/loop/agentic/prompting"
	"github.com/crom/crom-agente/internal/loop/agentic/tooling"
	"github.com/crom/crom-agente/internal/tools"
)

const (
	MaxIterations          = 15
	MaxConsecutiveFailures = 3
)

// --- Mock Tool para testes ---

type mockTool struct {
	id          string
	description string
	approve     bool
	executeFunc func(ctx context.Context, args json.RawMessage) (tools.Result, error)
}

func (m *mockTool) ID() string                        { return m.id }
func (m *mockTool) Description() string               { return m.description }
func (m *mockTool) ParametersSchema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (m *mockTool) RequiresApproval() bool            { return m.approve }
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
	Events        []loop.AgentEvent
}

func (h *testEventHandler) OnStatusChange(s string) {
	h.StatusChanges = append(h.StatusChanges, s)
}
func (h *testEventHandler) OnStreamChunk(chunk string) {}

func (h *testEventHandler) OnMessage(role, content string) {
	h.Messages = append(h.Messages, struct{ Role, Content string }{role, content})
}
func (h *testEventHandler) OnEvent(event loop.AgentEvent) {
	h.Events = append(h.Events, event)
}

type cancelOnRetryHandler struct {
	StatusChanges []string
	Messages      []struct{ Role, Content string }
	Events        []loop.AgentEvent
	cancelFunc    context.CancelFunc
}

func (h *cancelOnRetryHandler) OnStatusChange(s string) {
	h.StatusChanges = append(h.StatusChanges, s)
}
func (h *cancelOnRetryHandler) OnMessage(role, content string) {
	h.Messages = append(h.Messages, struct{ Role, Content string }{role, content})
}
func (h *cancelOnRetryHandler) OnStreamChunk(chunk string) {}
func (h *cancelOnRetryHandler) OnEvent(event loop.AgentEvent) {
	h.Events = append(h.Events, event)
	if event.Event == "retry" && h.cancelFunc != nil {
		h.cancelFunc()
	}
}

// --- Testes ---

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

func TestAgenticLoop_ToolCallSuccess(t *testing.T) {
	provider := providers.NewMockProvider(
		// 1ª: LLM pede read_file
		providers.MockToolCallResponse("read_file", `{"path":"/tmp/test.txt"}`, 200),
		// 2ª: LLM responde com texto
		providers.MockTextResponse("Arquivo lido com sucesso.", 150),
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
	if provider.TotalCalls() != 2 {
		t.Fatalf("esperado 2 chamadas ao LLM (tool+final), obteve %d", provider.TotalCalls())
	}
}

func TestAgenticLoop_ToolCallFailure(t *testing.T) {
	provider := providers.NewMockProvider(
		// 1ª: LLM pede read_file
		providers.MockToolCallResponse("read_file", `{"path":"/nope"}`, 200),
		// 2ª: LLM responde com texto
		providers.MockTextResponse("O arquivo não foi encontrado.", 100),
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
	provider := providers.NewMockProvider(
		// LLM pede ferramenta inexistente
		providers.MockToolCallResponse("delete_universe", `{}`, 200),
		// LLM reconhece o erro
		providers.MockTextResponse("Ok, ferramenta não disponível.", 50),
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
		if strings.Contains(m.Content, "não existe") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("esperado mensagem de ferramenta não encontrada nos eventos")
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

func TestAgenticLoop_ConsecutiveToolFailuresAbort(t *testing.T) {
	responses := make([]providers.MockResponse, MaxConsecutiveFailures+1)
	for i := range responses {
		responses[i] = providers.MockToolCallResponse("bad_tool", fmt.Sprintf(`{"arg":%d}`, i), 10)
	}
	provider := providers.NewMockProvider(responses...)
	sm := state.NewStateManager(t.TempDir())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := &cancelOnRetryHandler{
		cancelFunc: cancel,
	}

	al := New(provider, sm, handler)
	al.failureRetryDelay = 1 * time.Millisecond
	al.RegisterTool(&mockTool{
		id:          "bad_tool",
		description: "Sempre falha",
		executeFunc: func(ctx context.Context, args json.RawMessage) (tools.Result, error) {
			return tools.Result{Success: false, Error: "always fails"}, nil
		},
	})

	err := al.Execute(ctx, "Use bad_tool")
	if err == nil {
		t.Fatal("esperado erro por cancelamento de contexto, obteve nil")
	}
	if err != context.Canceled && !strings.Contains(err.Error(), "canceled") && !strings.Contains(err.Error(), "cancelado") {
		t.Fatalf("erro inesperado: %v", err)
	}

	// Verifica se o evento retry foi emitido
	hasRetry := false
	for _, ev := range handler.Events {
		if ev.Event == "retry" {
			hasRetry = true
			if ev.Data["reason"] != "consecutive_failures" {
				t.Fatalf("esperado reason=consecutive_failures, obteve %v", ev.Data["reason"])
			}
			if ev.Data["error_type"] != "tool_failure" {
				t.Fatalf("esperado error_type=tool_failure, obteve %v", ev.Data["error_type"])
			}
		}
	}
	if !hasRetry {
		t.Fatal("esperado evento 'retry' no handler")
	}
}

func TestCompactMessages(t *testing.T) {
	provider := providers.NewMockProvider(
		providers.MockTextResponse("Resumo do histórico", 100),
	)
	al := New(provider, nil, &testEventHandler{})

	// Cria 50 mensagens (acima do limite de 40)
	msgs := make([]llm.Message, 50)
	for i := range msgs {
		msgs[i] = llm.Message{Role: "user", Content: fmt.Sprintf("msg-%d", i)}
	}

	compacted := prompting.CompactMessages(context.Background(), al.provider, al.config.MaxMessageHistory, al.handler, msgs)

	// A nova compactação cria 1 (intent) + 1 (resumo) + 15 (recentes) = 17
	if len(compacted) != 12 {
		t.Fatalf("esperado 12 mensagens após compactação inteligente, obteve %d", len(compacted))
	}

	// A primeira mensagem deve ser preservada
	if compacted[0].Content != "msg-0" {
		t.Fatalf("primeira mensagem não preservada: %s", compacted[0].Content)
	}

	// A segunda mensagem deve conter o resumo determinístico (não mais LLM)
	if !strings.Contains(compacted[1].Content, "RESUMO DO HISTÓRICO COMPACTADO") {
		t.Fatalf("esperado resumo determinístico, obteve: %s", compacted[1].Content)
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

	if !DetectRepetitiveLoop(msgs) {
		t.Fatal("esperado detecção de loop repetitivo direto")
	}

	msgs[5].Content = "different thing"
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

func TestAgenticLoop_AgenticIdentityInjection(t *testing.T) {
	provider := providers.NewMockProvider(
		providers.MockTextResponse("Entendido, posso criar arquivos no seu computador.", 100),
	)
	sm := state.NewStateManager(t.TempDir())
	handler := &testEventHandler{}

	al := New(provider, sm, handler)

	err := al.Execute(context.Background(), "Olá, crie um arquivo para mim")
	if err != nil {
		t.Fatalf("esperado sucesso, obteve erro: %v", err)
	}

	// Verificar se a mensagem [SYSTEM AGENTIC IDENTITY] foi injetada no histórico
	foundIdentity := false
	for _, msg := range sm.GetMessages() {
		if strings.Contains(msg.Content, "[SYSTEM AGENTIC IDENTITY]") {
			foundIdentity = true
			// Verificar que contém as palavras-chave essenciais
			if !strings.Contains(msg.Content, "AI Sênior") {
				t.Fatal("mensagem AGENTIC IDENTITY não contém 'AI Sênior'")
			}
			if !strings.Contains(msg.Content, "edit_file") {
				t.Fatal("mensagem AGENTIC IDENTITY não menciona 'edit_file'")
			}
			if !strings.Contains(msg.Content, "traceback") {
				t.Fatal("mensagem AGENTIC IDENTITY não contém 'traceback'")
			}
			break
		}
	}
	if !foundIdentity {
		t.Fatal("esperada mensagem [SYSTEM AGENTIC IDENTITY] no histórico da primeira iteração")
	}
}

func TestAgenticLoop_SpawnSubagentRollback(t *testing.T) {
	tempDir := t.TempDir()

	// Inicializa o diretório como um repositório Git
	runGit := func(args ...string) error {
		cmd := exec.Command("git", args...)
		cmd.Dir = tempDir
		return cmd.Run()
	}

	if err := runGit("init"); err != nil {
		t.Skipf("pulando teste de rollback (git init falhou): %v", err)
		return
	}
	_ = runGit("config", "user.email", "test@crom.ia")
	_ = runGit("config", "user.name", "Test User")

	initialFile := filepath.Join(tempDir, "initial.txt")
	if err := os.WriteFile(initialFile, []byte("original content"), 0644); err != nil {
		t.Fatalf("erro ao criar arquivo inicial: %v", err)
	}

	if err := runGit("add", "."); err != nil {
		t.Fatalf("git add falhou: %v", err)
	}
	if err := runGit("commit", "-m", "initial commit"); err != nil {
		t.Fatalf("git commit falhou: %v", err)
	}

	// Mock do Provider
	provider := providers.NewMockProvider(
		// 1ª: Chama spawn_subagent
		providers.MockToolCallResponse("spawn_subagent", `{"task":"modify initial.txt"}`, 100),
		// 2ª: LLM reconhece a falha e responde
		providers.MockTextResponse("O subagente falhou e o rollback foi executado.", 150),
	)

	_ = os.MkdirAll(filepath.Join(tempDir, ".crom"), 0755)
	sm := state.NewStateManager(tempDir)

	al := New(provider, sm, nil)

	// Registra spawn_subagent com a lógica de rollback do teste
	al.RegisterTool(&mockTool{
		id:          "spawn_subagent",
		description: "Simula o spawn",
		executeFunc: func(ctx context.Context, args json.RawMessage) (tools.Result, error) {
			// Simula a escrita de alterações que deverão ser desfeitas
			modifiedFile := filepath.Join(tempDir, "initial.txt")
			_ = os.WriteFile(modifiedFile, []byte("corrupted content"), 0644)
			newFile := filepath.Join(tempDir, "added_by_subagent.txt")
			_ = os.WriteFile(newFile, []byte("should be deleted"), 0644)

			// Executa rollback com base na lógica de rollbackGit
			_ = tooling.RollbackGit(tempDir)

			return tools.Result{
				Success: false,
				Error:   "subagente falhou de teste",
			}, nil
		},
	})

	err := al.Execute(context.Background(), "Subagente faça a tarefa")
	if err != nil {
		t.Fatalf("esperado sucesso no loop principal (que trata a falha), obteve erro: %v", err)
	}

	// Verifica se os arquivos foram restaurados ao commit inicial
	content, err := os.ReadFile(initialFile)
	if err != nil {
		t.Fatalf("erro ao ler arquivo inicial: %v", err)
	}
	if string(content) != "original content" {
		t.Errorf("esperava conteúdo 'original content', obteve '%s'", string(content))
	}

	// O novo arquivo não deve existir (reset --hard remove arquivos não rastreados criados pelo subagente)
	addedFile := filepath.Join(tempDir, "added_by_subagent.txt")
	if _, err := os.Stat(addedFile); err == nil || !os.IsNotExist(err) {
		t.Errorf("esperava que o arquivo criado pelo subagente tivesse sido removido pelo rollback")
	}
}

func TestAgenticLoop_ScreenshotFallback(t *testing.T) {
	provider := providers.NewMockProvider(
		// LLM chama o método inexistente "screenshot"
		providers.MockToolCallResponse("screenshot", `{"path":"my_screenshot.png"}`, 200),
		// LLM recebe a resposta traduzida e finaliza
		providers.MockTextResponse("Screenshot capturado com sucesso.", 100),
	)
	sm := state.NewStateManager(t.TempDir())
	handler := &testEventHandler{}

	al := New(provider, sm, handler)

	// Registra o browser_action como a ferramenta correta
	var receivedArgs string
	al.RegisterTool(&mockTool{
		id:          "browser_action",
		description: "Navegador",
		executeFunc: func(ctx context.Context, args json.RawMessage) (tools.Result, error) {
			receivedArgs = string(args)
			return tools.Result{Success: true, Data: "screenshot taken successfully"}, nil
		},
	})

	err := al.Execute(context.Background(), "Tire um print")
	if err != nil {
		t.Fatalf("esperado sucesso, obteve erro: %v", err)
	}

	// Verifica se a chamada foi redirecionada para a ferramenta "browser_action"
	if !strings.Contains(receivedArgs, `"action":"screenshot"`) {
		t.Errorf("esperava action 'screenshot' inserido nos argumentos, obteve: %s", receivedArgs)
	}
	if !strings.Contains(receivedArgs, `"path":"my_screenshot.png"`) {
		t.Errorf("esperava path preservado, obteve: %s", receivedArgs)
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

func TestAgenticLoop_AskUserSuspension(t *testing.T) {
	provider := providers.NewMockProvider(
		// 1ª: LLM pede ask_user
		providers.MockToolCallResponse("ask_user", `{"question":"Qual banco de dados usar?"}`, 100),
		// O loop deve sair imediatamente após a chamada do ask_user, então não deve haver 2ª chamada!
	)
	sm := state.NewStateManager(t.TempDir())
	handler := &testEventHandler{}

	al := New(provider, sm, handler)
	al.RegisterTool(&mockTool{
		id:          "ask_user",
		description: "Faz uma pergunta ao usuário",
		executeFunc: func(ctx context.Context, args json.RawMessage) (tools.Result, error) {
			return tools.Result{Success: true, Data: "Pergunta enviada."}, nil
		},
	})

	err := al.Execute(context.Background(), "Configure o banco de dados")
	if err != nil {
		t.Fatalf("esperado sucesso (loop suspenso com nil), obteve erro: %v", err)
	}

	// O status final deve ser "waiting_user_input"
	lastStatus := handler.StatusChanges[len(handler.StatusChanges)-1]
	if lastStatus != "waiting_user_input" {
		t.Fatalf("esperado status final 'waiting_user_input', obteve '%s'", lastStatus)
	}

	// O estado salvo no disco deve estar como "waiting_user_input"
	savedState := sm.GetState()
	if savedState.UltimoStatus != "waiting_user_input" {
		t.Errorf("esperava UltimoStatus 'waiting_user_input', obteve '%s'", savedState.UltimoStatus)
	}

	// Não deve ter havido uma segunda chamada cognitiva
	if provider.TotalCalls() != 1 {
		t.Fatalf("esperado apenas 1 chamada ao LLM, obteve %d", provider.TotalCalls())
	}
}

func TestAgenticLoop_CognitiveTransitions(t *testing.T) {
	provider := providers.NewMockProvider(
		// 1ª: LLM chama ferramenta de validação (terminal_command com go test)
		providers.MockToolCallResponse("terminal_command", `{"command":"go test ./..."}`, 100),
		// 2ª: LLM chama ferramenta que vai falhar (bad_tool)
		providers.MockToolCallResponse("bad_tool", `{}`, 100),
		// 3ª: LLM responde finalizando
		providers.MockTextResponse("Tudo pronto.", 150),
	)

	sm := state.NewStateManager(t.TempDir())
	// Pré-popular o plano para que não comece em "planning"
	_ = sm.SetPlan([]state.TaskItem{{Title: "Tarefa 1", Status: "completed"}})

	handler := &testEventHandler{}
	al := New(provider, sm, handler)

	// Registrar ferramentas
	al.RegisterTool(&mockTool{
		id:          "terminal_command",
		description: "roda comando",
		executeFunc: func(ctx context.Context, args json.RawMessage) (tools.Result, error) {
			return tools.Result{Success: true, Data: "tests passed"}, nil
		},
	})
	al.RegisterTool(&mockTool{
		id:          "bad_tool",
		description: "falha sempre",
		executeFunc: func(ctx context.Context, args json.RawMessage) (tools.Result, error) {
			return tools.Result{Success: false, Error: "error occurred"}, nil
		},
	})

	// Capturar os modos cognitivos salvos
	var modesRecorded []string
	// Para monitorar mudanças de modo cognitivo, podemos ler do StateManager durante a execução das ferramentas
	// ou injetar um callback se possível. Como o StateManager é persistido, podemos capturar o estado dentro dos mocks.
	// No terminal_command (1ª iteração):
	// O loop já determinou o modo cognitivo para a iteração 1 (executing) e salvou no StateManager.
	al.tools["terminal_command"].(*mockTool).executeFunc = func(ctx context.Context, args json.RawMessage) (tools.Result, error) {
		modesRecorded = append(modesRecorded, sm.GetState().ModoCognitivo)
		return tools.Result{Success: true, Data: "tests passed"}, nil
	}
	// No bad_tool (2ª iteração):
	// O loop já determinou o modo cognitivo para a iteração 2 (verifying, já que terminal_command foi validação) e salvou.
	al.tools["bad_tool"].(*mockTool).executeFunc = func(ctx context.Context, args json.RawMessage) (tools.Result, error) {
		modesRecorded = append(modesRecorded, sm.GetState().ModoCognitivo)
		return tools.Result{Success: false, Error: "error occurred"}, nil
	}

	err := al.Execute(context.Background(), "Rode validações")
	if err != nil {
		t.Fatalf("Execute falhou: %v", err)
	}

	// 3ª iteração (finalização textual):
	// O loop determinou o modo cognitivo para a iteração 3 (debugging, já que bad_tool falhou) e salvou.
	modesRecorded = append(modesRecorded, sm.GetState().ModoCognitivo)

	if len(modesRecorded) != 3 {
		t.Fatalf("esperava 3 registros de modos cognitivos, obteve %d: %v", len(modesRecorded), modesRecorded)
	}

	if modesRecorded[0] != state.ModoExecuting {
		t.Errorf("esperava 1º modo 'executing', obteve '%s'", modesRecorded[0])
	}
	if modesRecorded[1] != state.ModoVerifying {
		t.Errorf("esperava 2º modo 'verifying', obteve '%s'", modesRecorded[1])
	}
	if modesRecorded[2] != state.ModoDebugging {
		t.Errorf("esperava 3º modo 'debugging', obteve '%s'", modesRecorded[2])
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

func TestAgenticLoop_ConsecutiveFailuresRetryDisabled(t *testing.T) {
	responses := make([]providers.MockResponse, MaxConsecutiveFailures+1)
	for i := range responses {
		responses[i] = providers.MockToolCallResponse("bad_tool", fmt.Sprintf(`{"arg":%d}`, i), 10)
	}
	provider := providers.NewMockProvider(responses...)
	sm := state.NewStateManager(t.TempDir())

	cfg := &config.ResolvedConfig{
		MaxIterations:             15,
		MaxConsecutiveFail:        3,
		ConsecutiveFailureRetry:      false, // retry desabilitado
		ConsecutiveFailureRetryLimit: 0,
		ConsecutiveFailureRetryDelay: 0,
	}

	al := New(provider, sm, nil, cfg)
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

func TestAgenticLoop_ConsecutiveFailuresRetryLimit(t *testing.T) {
	responses := make([]providers.MockResponse, MaxConsecutiveFailures+5)
	for i := range responses {
		responses[i] = providers.MockToolCallResponse("bad_tool", fmt.Sprintf(`{"arg":%d}`, i), 10)
	}
	provider := providers.NewMockProvider(responses...)
	sm := state.NewStateManager(t.TempDir())

	handler := &testEventHandler{}

	cfg := &config.ResolvedConfig{
		MaxIterations:             15,
		MaxConsecutiveFail:        3,
		ConsecutiveFailureRetry:      true,
		ConsecutiveFailureRetryLimit: 2, // Limite de 2 retries
		ConsecutiveFailureRetryDelay: 0,
	}

	al := New(provider, sm, handler, cfg)
	al.failureRetryDelay = 1 * time.Millisecond
	al.RegisterTool(&mockTool{
		id:          "bad_tool",
		description: "Sempre falha",
		executeFunc: func(ctx context.Context, args json.RawMessage) (tools.Result, error) {
			return tools.Result{Success: false, Error: "always fails"}, nil
		},
	})

	err := al.Execute(context.Background(), "Use bad_tool")
	if err == nil {
		t.Fatal("esperado erro por atingir limite de retry, obteve nil")
	}
	if !strings.Contains(err.Error(), "falhas consecutivas") {
		t.Fatalf("erro inesperado: %v", err)
	}

	// Deve ter emitido o evento retry exatamente 2 vezes
	retryCount := 0
	for _, ev := range handler.Events {
		if ev.Event == "retry" {
			retryCount++
		}
	}
	if retryCount != 2 {
		t.Fatalf("esperado exatamente 2 eventos de retry, obteve %d", retryCount)
	}
}

func TestDetectHallucinatedToolCalls(t *testing.T) {
	toolsMap := map[string]tools.Tool{
		"write_file":       &mockTool{id: "write_file"},
		"terminal_command": &mockTool{id: "terminal_command"},
		"read_file":        &mockTool{id: "read_file"},
	}

	tests := []struct {
		name     string
		content  string
		expected int // número de ferramentas detectadas
	}{
		// Legado (padrões diretos)
		{"empty", "", 0},
		{"direct call parens", "vou usar write_file(foo, bar)", 1},
		{"direct call braces", "chame terminal_command {cmd: ls}", 1},
		{"direct tool_call prefix", "tool_call: write_file", 1},
		{"no hallucination", "nada de especial aqui", 0},

		// Padrões narrativos PT-BR
		{"narrative chamando bracket", "[Chamando write_file]", 1},
		{"narrative chamando ferramenta bracket", "[Chamando ferramenta terminal_command]", 1},
		{"narrative executar", "Agora vou executar write_file para criar o arquivo.", 1},
		{"narrative vou usar", "vou usar write_file para escrever o código", 1},
		{"narrative vou chamar", "vou chamar terminal_command para rodar o script", 1},
		{"narrative executando", "executando terminal_command no diretório", 1},
		{"narrative chamar", "preciso chamar read_file agora", 1},
		{"narrative invocar", "vou invocar write_file", 1},

		// Padrões narrativos EN
		{"narrative calling bracket", "[Calling write_file]", 1},
		{"narrative calling tool bracket", "[Calling tool terminal_command]", 1},
		{"narrative execute", "I will execute write_file to create the file", 1},
		{"narrative running", "running terminal_command now", 1},
		{"narrative using tool", "using tool read_file to read the contents", 1},
		{"narrative i'll call", "I'll call write_file next", 1},

		// Padrões JSON inline
		{"json inline tool", `Vou enviar: {"tool": "write_file", "args": {}}`, 1},
		{"json inline name", `resposta: {"name": "terminal_command", "arguments": {}}`, 1},
		{"json inline function", `{"function": "read_file", "params": {}}`, 1},
		{"json inline tool_name", `{"tool_name": "write_file", "input": {}}`, 1},

		// Dentro de bloco de código — NÃO deve detectar (falso positivo evitado)
		{"code block no detect", "Aqui está o código:\n```python\n# usando write_file(path, content)\nprint('ok')\n```\n", 0},
		{"code block markdown tool ref", "Veja:\n```go\n// tool_call: write_file\nfmt.Println(\"test\")\n```\n", 0},
		{"code block json inline", "```json\n{\"tool\": \"write_file\", \"args\": {}}\n```\n", 0},

		// Misto: texto fora do bloco detecta, bloco de código não
		{"mixed text and code", "vou executar write_file agora.\n```python\n# write_file(x, y)\n```\n", 1},

		// Múltiplas ferramentas
		{"multiple tools narrative", "vou chamar write_file e executar terminal_command", 2},

		// Tool code block — NÃO deve detectar
		{"tool_code block no detect", "/tool_code\nwrite_file.execute(path='a', content='b')\n/tool_code\n", 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := detectHallucinatedToolCalls(tc.content, toolsMap)
			if len(res) != tc.expected {
				t.Errorf("para %q esperado %d ferramentas, obteve %d: %v", tc.name, tc.expected, len(res), res)
			}
		})
	}
}

func TestStripCodeBlocks(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantLen int // aproximado: verifica que blocos foram removidos
	}{
		{
			"no code blocks",
			"texto normal sem blocos",
			1,
		},
		{
			"single markdown block",
			"antes\n```python\nprint('hello')\n```\ndepois",
			2, // "antes" e "depois"
		},
		{
			"tool_code block",
			"antes\n/tool_code\nwrite_file.execute()\n/tool_code\ndepois",
			2, // "antes" e "depois"
		},
		{
			"nested blocks",
			"texto\n```go\nfunc main() {}\n```\nmais texto\n```js\nconsole.log()\n```\nfim",
			3, // "texto", "mais texto", "fim"
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := stripCodeBlocks(tc.input)
			lines := strings.Split(strings.TrimSpace(result), "\n")
			nonEmpty := 0
			for _, l := range lines {
				if strings.TrimSpace(l) != "" {
					nonEmpty++
				}
			}
			if nonEmpty != tc.wantLen {
				t.Errorf("esperado %d linhas não-vazias, obteve %d. Resultado: %q", tc.wantLen, nonEmpty, result)
			}
		})
	}
}

func TestAgenticLoop_HallucinatedToolCallFormat(t *testing.T) {
	// Primeiro MockResponse emite menção de tool call alucinada (formato texto),
	// O segundo MockResponse emite uma resposta válida ou concluindo a execução.
	responses := []providers.MockResponse{
		providers.MockTextResponse("Eu gostaria de usar write_file(arquivo.txt, conteúdo).", 10),
		providers.MockTextResponse("Tudo bem, a tarefa foi concluída.", 10),
	}

	provider := providers.NewMockProvider(responses...)
	sm := state.NewStateManager(t.TempDir())
	handler := &testEventHandler{}
	cfg := &config.ResolvedConfig{
		MaxIterations:             5,
		MaxConsecutiveFail:        3,
		DisablePromptOptimization: true,
	}

	al := New(provider, sm, handler, cfg)
	al.RegisterTool(&mockTool{
		id:          "write_file",
		description: "Escreve em um arquivo",
		executeFunc: func(ctx context.Context, args json.RawMessage) (tools.Result, error) {
			return tools.Result{Success: true}, nil
		},
	})

	err := al.Execute(context.Background(), "Escreva oi")
	if err != nil {
		t.Fatalf("erro inesperado no loop: %v", err)
	}

	// Verifica se a mensagem de correção do sistema foi gerada
	msgs := sm.GetMessages()
	hasWarning := false
	for _, m := range msgs {
		if m.Role == "system" && strings.Contains(m.Content, "[INVALID_TOOL_CALL_FORMAT]") {
			hasWarning = true
			break
		}
	}

	if !hasWarning {
		t.Fatal("esperava que a mensagem do sistema contivesse o aviso de formato de chamada de ferramenta inválido")
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


func TestAgenticLoop_FileValidationFail(t *testing.T) {
	tempDir := t.TempDir()
	invalidFilePath := filepath.Join(tempDir, "invalid.py")

	provider := providers.NewMockProvider(
		// 1ª turn: LLM tenta escrever arquivo python inválido
		providers.MockToolCallResponse("write_file", fmt.Sprintf(`{"path":%q,"content":"def hello(\n    print('mismatched')"}`, invalidFilePath), 200),
		// 2ª turn: LLM vê o erro de validação e se desculpa
		providers.MockTextResponse("Desculpe, vou corrigir.", 100),
	)

	sm := state.NewStateManager(t.TempDir())
	handler := &testEventHandler{}
	cfg := &config.ResolvedConfig{
		MaxIterations:             5,
		MaxConsecutiveFail:        3,
		DisablePromptOptimization: true,
	}

	al := New(provider, sm, handler, cfg)
	al.RegisterTool(&mockTool{
		id:          "write_file",
		description: "Escreve arquivo",
		executeFunc: func(ctx context.Context, args json.RawMessage) (tools.Result, error) {
			var argsParsed struct {
				Path    string `json:"path"`
				Content string `json:"content"`
			}
			if err := json.Unmarshal(args, &argsParsed); err != nil {
				return tools.Result{Success: false, Error: err.Error()}, nil
			}
			err := os.WriteFile(argsParsed.Path, []byte(argsParsed.Content), 0644)
			if err != nil {
				return tools.Result{Success: false, Error: err.Error()}, nil
			}
			return tools.Result{Success: true, Data: "Arquivo escrito com sucesso."}, nil
		},
	})

	err := al.Execute(context.Background(), "Escreva um script Python que falha na compilação")
	if err != nil {
		t.Fatalf("esperado sucesso no loop geral, obteve erro: %v", err)
	}

	// O mock tool retornou Success: true mas o interceptador deve ter alterado para falha devido ao erro de validação.
	// Vamos verificar se nos logs de turnos a chamada da ferramenta foi gravada como falha ou contém VALIDATION_ERROR.
	traces := sm.GetMessages()
	hasValidationError := false
	for _, msg := range traces {
		if msg.Role == "tool" && strings.Contains(msg.Content, "[VALIDATION_ERROR]") {
			hasValidationError = true
			break
		}
	}
	if !hasValidationError {
		t.Fatal("esperava mensagem do tipo tool contendo [VALIDATION_ERROR] após a falha de validação sintática do python")
	}
}

func TestAgenticLoop_Metrics(t *testing.T) {
	tempDir := t.TempDir()
	validFilePath := filepath.Join(tempDir, "valid.py")

	provider := providers.NewMockProvider(
		// 1ª turn: LLM tenta escrever arquivo python válido
		providers.MockToolCallResponse("write_file", fmt.Sprintf(`{"path":%q,"content":"def hello():\n    print('ok')"}`, validFilePath), 200),
		// 2ª turn: LLM responde com texto final
		providers.MockTextResponse("Pronto, arquivo criado.", 100),
	)

	sm := state.NewStateManager(t.TempDir())
	handler := &testEventHandler{}
	cfg := &config.ResolvedConfig{
		MaxIterations:             5,
		MaxConsecutiveFail:        3,
		DisablePromptOptimization: true,
	}

	al := New(provider, sm, handler, cfg)
	al.RegisterTool(&mockTool{
		id:          "write_file",
		description: "Escreve arquivo",
		executeFunc: func(ctx context.Context, args json.RawMessage) (tools.Result, error) {
			var argsParsed struct {
				Path    string `json:"path"`
				Content string `json:"content"`
			}
			if err := json.Unmarshal(args, &argsParsed); err != nil {
				return tools.Result{Success: false, Error: err.Error()}, nil
			}
			err := os.WriteFile(argsParsed.Path, []byte(argsParsed.Content), 0644)
			if err != nil {
				return tools.Result{Success: false, Error: err.Error()}, nil
			}
			return tools.Result{Success: true, Data: "Arquivo escrito com sucesso."}, nil
		},
	})

	err := al.Execute(context.Background(), "Escreva um script Python válido")
	if err != nil {
		t.Fatalf("esperado sucesso no loop geral, obteve erro: %v", err)
	}

	s := sm.GetState()
	if s.FilesCreated != 1 {
		t.Errorf("esperava FilesCreated=1, obteve %d", s.FilesCreated)
	}
	if s.FilesValidated != 1 {
		t.Errorf("esperava FilesValidated=1, obteve %d", s.FilesValidated)
	}
	if s.ToolCallsEmitted != 1 {
		t.Errorf("esperava ToolCallsEmitted=1, obteve %d", s.ToolCallsEmitted)
	}
}



