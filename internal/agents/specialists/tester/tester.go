package tester

import (
	"context"
	_ "embed"
	"fmt"
	"path/filepath"
	"time"

	"github.com/crom/crom-agente/internal/agents"
	"github.com/crom/crom-agente/internal/agents/core"
	"github.com/crom/crom-agente/internal/config"
	agenticcore "github.com/crom/crom-agente/internal/loop/agentic/core"
	"github.com/crom/crom-agente/internal/loop/agentic/tooling"
	"github.com/crom/crom-agente/internal/state"
	"github.com/crom/crom-agente/internal/tools"
	"github.com/crom/crom-agente/internal/tools/registry"
)

//go:embed metadata.json
var metadataJSON []byte

var metadata tools.ToolMetadata

const defaultSystemPrompt = `Você é o Tester Agent, um especialista em testes de software e controle de qualidade do CROM-Agente.
Sua principal responsabilidade é garantir a corretude do código na base de código através de testes automatizados (unitários, integração e E2E).

Ao receber uma tarefa de teste:
1. Localize a suíte de testes existente (ex: go test, pytest, jest) ou crie novos casos de teste temporários focados na funcionalidade desejada.
2. Escreva novos casos de teste ou arquivos de testes auxiliares temporários (ex: terminando com _aux_test.go ou test_aux_*.py) para cobrir limites extremos, inputs maliciosos e garantir que as alterações não causam nenhuma regressão.
3. Execute os testes usando a ferramenta terminal_command ou run_tests.
4. Se os testes falharem, analise detalhadamente a saída de erro e o código-fonte para isolar a causa raiz (ex: identificar a linha exata que disparou AssertionError ou pânico).
5. Corrija o código ou os testes conforme necessário, utilizando as ferramentas de leitura e escrita de arquivos.
6. Execute os testes novamente até que todos passem com sucesso.
7. Forneça um relatório final markdown estruturado com:
   - Resumo dos testes executados (quais suítes, quantidade de testes que passaram/falharam).
   - Detalhamento de qualquer falha encontrada e como foi corrigida.
   - Diagnóstico final atestando o funcionamento do código.

Mantenha seu raciocínio focado em testes de regressão e casos de borda.`

func init() {
	var err error
	metadata, err = tools.ParseMetadata(metadataJSON)
	if err != nil {
		panic("falha ao carregar metadados de tester: " + err.Error())
	}

	// Auto-registro global do especialista native "tester"
	agents.RegisterAgent("tester", func(cfg agents.Config) core.Agent {
		return NewTesterAgent(cfg)
	})
}

// TesterAgent executa tarefas de testes e correção de bugs de forma autônoma
type TesterAgent struct {
	core.BaseAgent
	workspacePath string
}

// NewTesterAgent cria uma nova instância do TesterAgent
func NewTesterAgent(cfg agents.Config) *TesterAgent {
	ta := &TesterAgent{
		workspacePath: cfg.WorkspacePath,
	}
	ta.AgentName = "tester"
	ta.AgentDescription = metadata.Description
	ta.AgentSysPrompt = defaultSystemPrompt
	ta.LLMProvider = cfg.LLMProvider
	ta.AllowedToolIDs = []string{"terminal_command", "run_tests", "read_file", "write_file", "diff_replace", "grep", "tree"}
	return ta
}

// Name retorna o nome do especialista
func (t *TesterAgent) Name() string {
	return t.AgentName
}

// Description retorna a descrição
func (t *TesterAgent) Description() string {
	return t.AgentDescription
}

// SystemPrompt retorna o prompt de sistema do especialista
func (t *TesterAgent) SystemPrompt() string {
	return t.AgentSysPrompt
}

// Execute executa o subagente iniciando um AgenticLoop isolado focado em testes
func (t *TesterAgent) Execute(ctx context.Context, prompt string, priorSummary string) (core.AgentResult, error) {
	subagentID := fmt.Sprintf("tester-%d", time.Now().UnixNano())
	if parentSession, ok := ctx.Value("session_name").(string); ok && parentSession != "" {
		subagentID = fmt.Sprintf("session-%s-tester", parentSession)
	}
	storageDir := filepath.Join(t.workspacePath, ".crom", "agents", subagentID)

	subSM := state.NewStateManager(storageDir)
	_ = subSM.LoadState()

	// Injeta o priorSummary como contextualização se disponível
	execTask := prompt
	if priorSummary != "" {
		execTask = fmt.Sprintf("[HISTÓRICO ANTERIOR]\n%s\n\n[TAREFA ATUAL]\n%s", priorSummary, prompt)
	}

	subCfg := &config.ResolvedConfig{
		MaxIterations:             15,
		MaxConsecutiveFail:        3,
		ToolTimeoutSeconds:        30,
		MaxMessageHistory:         40,
		AutoVerify:                true,
		PermissionMode:            "scoped",
		WorkspaceJail:             true,
		DisablePromptOptimization: true,
	}

	// Instancia o loop ReAct
	subAL := agenticcore.New(t.LLMProvider, subSM, nil, subCfg)

	// Registra as ferramentas nativas básicas para o subagente trabalhar
	builtinTools := registry.GetBuiltinTools(registry.RegistrationConfig{
		WorkspacePath: t.workspacePath,
		WorkspaceJail: true,
		StateManager:  subSM,
		LLMProvider:  t.LLMProvider,
	})

	for _, bt := range builtinTools {
		subAL.RegisterTool(bt)
	}

	// Roda o loop
	err := subAL.Execute(ctx, execTask)
	if err != nil {
		if t.workspacePath != "" {
			_ = tooling.RollbackGit(t.workspacePath)
		}
		return core.AgentResult{}, fmt.Errorf("falha na execução do agente tester: %w", err)
	}

	// Extrai a última mensagem do assistente como output final
	output := "O agente de testes concluiu sua análise com sucesso."
	msgs := subSM.GetMessages()
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "assistant" && msgs[i].Content != "" {
			output = msgs[i].Content
			break
		}
	}

	// Sumariza a execução do subagente
	newSummary, _ := core.CompressHistory(ctx, t.LLMProvider, prompt, output, priorSummary)

	// Registra os tokens consumidos pelo subagente no StateManager principal (Task 7)
	if recordFn, ok := ctx.Value("token_recorder_callback").(func(int)); ok && subSM != nil {
		recordFn(subSM.GetState().TokensGastos)
	}

	return core.AgentResult{
		Success:        true,
		Output:         output,
		ContextSummary: newSummary,
	}, nil
}
