package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	Events        []AgentEvent
}

func (h *testEventHandler) OnStatusChange(s string) {
	h.StatusChanges = append(h.StatusChanges, s)
}
func (h *testEventHandler) OnMessage(role, content string) {
	h.Messages = append(h.Messages, struct{ Role, Content string }{role, content})
}
func (h *testEventHandler) OnEvent(event AgentEvent) {
	h.Events = append(h.Events, event)
}

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
		// 2ª: LLM responde com texto
		llm.MockTextResponse("Arquivo lido com sucesso.", 150),
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
	provider := llm.NewMockProvider(
		// 1ª: LLM pede read_file
		llm.MockToolCallResponse("read_file", `{"path":"/nope"}`, 200),
		// 2ª: LLM responde com texto
		llm.MockTextResponse("O arquivo não foi encontrado.", 100),
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
		// LLM reconhece o erro
		llm.MockTextResponse("Ok, ferramenta não disponível.", 50),
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



func TestAgenticLoop_AgenticIdentityInjection(t *testing.T) {
	provider := llm.NewMockProvider(
		llm.MockTextResponse("Entendido, posso criar arquivos no seu computador.", 100),
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
			if !strings.Contains(msg.Content, "agente autônomo") {
				t.Fatal("mensagem AGENTIC IDENTITY não contém 'agente autônomo'")
			}
			if !strings.Contains(msg.Content, "write_file") {
				t.Fatal("mensagem AGENTIC IDENTITY não menciona 'write_file'")
			}
			if !strings.Contains(msg.Content, "NUNCA") {
				t.Fatal("mensagem AGENTIC IDENTITY não contém proibição 'NUNCA'")
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
	provider := llm.NewMockProvider(
		// 1ª: Chama spawn_subagent
		llm.MockToolCallResponse("spawn_subagent", `{"task":"modify initial.txt"}`, 100),
		// 2ª: LLM reconhece a falha e responde
		llm.MockTextResponse("O subagente falhou e o rollback foi executado.", 150),
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
			_ = rollbackGit(tempDir)

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
	provider := llm.NewMockProvider(
		// LLM chama o método inexistente "screenshot"
		llm.MockToolCallResponse("screenshot", `{"path":"my_screenshot.png"}`, 200),
		// LLM recebe a resposta traduzida e finaliza
		llm.MockTextResponse("Screenshot capturado com sucesso.", 100),
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

