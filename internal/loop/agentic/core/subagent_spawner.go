package core

import (
	"github.com/crom/crom-agente/internal/agents"
	"github.com/crom/crom-agente/internal/tools"
)

// RegisterSpawnSubagentTool registra o SpawnAgent sob a assinatura de tool para compatibilidade retrógrada nos testes do loop
func (al *AgenticLoop) RegisterSpawnSubagentTool() {
	workspaceDir := ""
	if al.stateManager != nil {
		workspaceDir = al.stateManager.GetWorkspaceDir()
	}

	agent, ok := agents.GetAgentInst("spawn_subagent", agents.Config{
		WorkspacePath: workspaceDir,
		LLMProvider:   al.provider,
	})
	if ok {
		al.RegisterTool(tools.NewAgentToolAdapter(agent))
	}
}
