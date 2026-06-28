package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crom/crom-agente/internal/config"
	"github.com/crom/crom-agente/internal/llm"
	"github.com/crom/crom-agente/internal/llm/providers"
	"github.com/crom/crom-agente/internal/state"
	"github.com/crom/crom-agente/internal/loop/agentic/prompting"
	"github.com/crom/crom-agente/internal/tools"
)

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



