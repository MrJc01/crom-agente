package loop

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/crom/crom-agente/internal/state"
	"github.com/crom/crom-agente/internal/tools"
)

// formatToolError gera um erro visualmente formatado para o contexto de mensagens do LLM
func formatToolError(toolID string, errMsg string) string {
	border := strings.Repeat("═", 50)
	return fmt.Sprintf("\n╔%s╗\n║ ERROR: Tool \"%s\" failed\n╟%s╢\n║ %s\n╚%s╝\n",
		border, toolID, border, errMsg, border)
}

// RegisterSpawnSubagentTool registra a ferramenta spawn_subagent no loop ativo
func (al *AgenticLoop) RegisterSpawnSubagentTool() {
	spawner := func(ctx context.Context, task string) (tools.Result, error) {
		subagentID := fmt.Sprintf("subagent-%d", time.Now().UnixNano())
		storageDir := filepath.Join(filepath.Dir(al.stateManager.FilePath()), "agents", subagentID)

		subSM := state.NewStateManager(storageDir)
		_ = subSM.LoadState()

		// Herdando configurações
		subAL := New(al.provider, subSM, al.handler, al.config)
		subAL.permissionManager = al.permissionManager
		for _, t := range al.tools {
			subAL.RegisterTool(t)
		}

		// Executa
		err := subAL.Execute(ctx, task)
		if err != nil {
			// Subagente falhou: efetua rollback baseado em Git na raiz do workspace
			workspaceDir := al.stateManager.GetWorkspaceDir()
			_ = rollbackGit(workspaceDir)

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

	al.RegisterTool(tools.NewSpawnSubagentTool(spawner))
}

// rollbackGit desfaz qualquer modificação efetuada nos arquivos locais
func rollbackGit(workspaceDir string) error {
	cmd := exec.Command("git", "reset", "--hard", "HEAD")
	cmd.Dir = workspaceDir
	return cmd.Run()
}
