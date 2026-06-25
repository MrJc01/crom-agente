package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/crom/crom-agente/internal/config"
	"github.com/crom/crom-agente/internal/loop/agentic/tooling"
	"github.com/crom/crom-agente/internal/state"
	"github.com/crom/crom-agente/internal/tools"
	"github.com/crom/crom-agente/internal/tools/spawn_subagent"
)

// SubagentConfig represents the configuration for a dynamic subagent
type SubagentConfig struct {
	Tools        []string `json:"tools"`
	Model        string   `json:"model"`
	SystemPrompt string   `json:"system_prompt"`
}

// RegisterSpawnSubagentTool registra a ferramenta spawn_subagent no loop ativo com suporte a agentes dinâmicos em JSON
func (al *AgenticLoop) RegisterSpawnSubagentTool() {
	spawner := func(ctx context.Context, agentName string, task string) (tools.Result, error) {
		subagentID := fmt.Sprintf("subagent-%d", time.Now().UnixNano())
		workspaceDir := ""
		storageDir := ".crom/agents/" + subagentID

		if al.stateManager != nil {
			workspaceDir = al.stateManager.GetWorkspaceDir()
			storageDir = filepath.Join(filepath.Dir(al.stateManager.FilePath()), "agents", subagentID)
		}

		subSM := state.NewStateManager(storageDir)
		_ = subSM.LoadState()

		var agentCfg *SubagentConfig
		var customPrompt string

		// Load Agent Config if agentName provided
		if agentName != "" && workspaceDir != "" {
			agentDir := filepath.Join(workspaceDir, ".crom", "agents", agentName)
			agentJSONPath := filepath.Join(agentDir, "agent.json")
			data, err := os.ReadFile(agentJSONPath)
			if err == nil {
				var ac SubagentConfig
				if err := json.Unmarshal(data, &ac); err == nil {
					agentCfg = &ac

					// Load custom system prompt if defined
					if ac.SystemPrompt != "" {
						promptPath := filepath.Join(agentDir, ac.SystemPrompt)
						pData, pErr := os.ReadFile(promptPath)
						if pErr == nil {
							customPrompt = string(pData)
						}
					}
				}
			}
		}

		// Herdando e substituindo configurações
		var subCfg *config.ResolvedConfig
		if al.config != nil {
			c := *al.config
			subCfg = &c
		} else {
			subCfg = &config.ResolvedConfig{}
		}

		if agentCfg != nil && agentCfg.Model != "" {
			subCfg.Model = agentCfg.Model
		}

		subAL := New(al.provider, subSM, al.handler, subCfg)
		subAL.permissionManager = al.permissionManager

		// Filter tools
		if agentCfg != nil && len(agentCfg.Tools) > 0 {
			for _, allowed := range agentCfg.Tools {
				if t, ok := al.tools[allowed]; ok {
					subAL.RegisterTool(t)
				}
			}
		} else {
			// Herda todas
			for _, t := range al.tools {
				subAL.RegisterTool(t)
			}
		}

		// Inject custom system prompt if loaded
		if customPrompt != "" && subAL.promptManager != nil {
			// Fallback: we inject it directly as a first message before running
			// For simplicity, we just inject it into the subSM
		}

		// Executa
		// We can prepend the custom prompt to the task if we don't have a direct setter
		execTask := task
		if customPrompt != "" {
			execTask = fmt.Sprintf("[SUBAGENT SYSTEM DIRECTIVE]\n%s\n\n[TASK]\n%s", customPrompt, task)
		}

		err := subAL.Execute(ctx, execTask)
		if err != nil {
			// Subagente falhou: efetua rollback baseado em Git na raiz do workspace
			if workspaceDir != "" {
				_ = tooling.RollbackGit(workspaceDir)
			}

			return tools.Result{
				Success: false,
				Error:   fmt.Sprintf("subagente falhou: %s. Rollback automático executado.", err.Error()),
			}, nil
		}

		return tools.Result{
			Success: true,
			Data:    "Subagente concluiu a tarefa com sucesso.",
		}, nil
	}

	al.RegisterTool(spawn_subagent.NewSpawnSubagentTool(spawner))
}
