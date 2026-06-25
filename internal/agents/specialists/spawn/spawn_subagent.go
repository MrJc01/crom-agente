package spawn

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

func init() {
	var err error
	metadata, err = tools.ParseMetadata(metadataJSON)
	if err != nil {
		panic("falha ao carregar metadados de spawn_subagent: " + err.Error())
	}

	// Auto-registro global do especialista native "spawn_subagent"
	agents.RegisterAgent("spawn_subagent", func(cfg agents.Config) core.Agent {
		return NewSpawnAgent(cfg)
	})
}

// SpawnAgent permite criar agentes filhos assíncronos que executam loops ReAct isolados
type SpawnAgent struct {
	core.BaseAgent
	workspacePath string
}

// NewSpawnAgent cria uma nova instância do SpawnAgent
func NewSpawnAgent(cfg agents.Config) *SpawnAgent {
	sa := &SpawnAgent{
		workspacePath: cfg.WorkspacePath,
	}
	sa.AgentName = "spawn_subagent"
	sa.AgentDescription = metadata.Description
	sa.LLMProvider = cfg.LLMProvider
	sa.AllowedToolIDs = []string{"terminal_command", "read_file", "write_file"}
	return sa
}

// Name retorna o nome do especialista
func (s *SpawnAgent) Name() string {
	return s.AgentName
}

// Description retorna a descrição
func (s *SpawnAgent) Description() string {
	return s.AgentDescription
}

// SystemPrompt retorna o prompt de sistema do especialista
func (s *SpawnAgent) SystemPrompt() string {
	return s.AgentSysPrompt
}

// Execute executa o subagente iniciando um AgenticLoop isolado
func (s *SpawnAgent) Execute(ctx context.Context, prompt string, priorSummary string) (core.AgentResult, error) {
	subagentID := fmt.Sprintf("subagent-%d", time.Now().UnixNano())
	storageDir := filepath.Join(s.workspacePath, ".crom", "agents", subagentID)

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
	subAL := agenticcore.New(s.LLMProvider, subSM, nil, subCfg)

	// Registra as ferramentas nativas básicas para o subagente trabalhar
	builtinTools := registry.GetBuiltinTools(registry.RegistrationConfig{
		WorkspacePath: s.workspacePath,
		WorkspaceJail: true,
		StateManager:  subSM,
	})

	for _, t := range builtinTools {
		subAL.RegisterTool(t)
	}

	// Roda o loop
	err := subAL.Execute(ctx, execTask)
	if err != nil {
		if s.workspacePath != "" {
			_ = tooling.RollbackGit(s.workspacePath)
		}
		return core.AgentResult{}, fmt.Errorf("falha na execução do agente filho: %w", err)
	}

	// Extrai a última mensagem do assistente como output final
	output := "Subagente concluiu a tarefa com sucesso."
	msgs := subSM.GetMessages()
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "assistant" && msgs[i].Content != "" {
			output = msgs[i].Content
			break
		}
	}

	// Sumariza a execução do subagente
	newSummary, _ := core.CompressHistory(ctx, s.LLMProvider, prompt, output, priorSummary)

	return core.AgentResult{
		Success:        true,
		Output:         output,
		ContextSummary: newSummary,
	}, nil
}
