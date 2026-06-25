package core

import (
	"context"
)

// AgentMetadata contém metadados sobre o especialista
type AgentMetadata struct {
	Version    string `json:"version"`
	Author     string `json:"author"`
	MCPVersion string `json:"mcp_version,omitempty"`
}

// AgentResult representa a saída de uma chamada de subagente especialista
type AgentResult struct {
	Success        bool   `json:"success"`
	Output         string `json:"output"`
	ContextSummary string `json:"context_summary"`
}

// Agent define a interface comum para todos os subagentes especialistas (nativos, MCP ou externos)
type Agent interface {
	// Name retorna o identificador único ou nome do especialista (ex: "browser", "spawn")
	Name() string

	// Description descreve a especialidade do agente e quando chamá-lo
	Description() string

	// SystemPrompt retorna a instrução de sistema que guia o comportamento do especialista
	SystemPrompt() string

	// ToolIDs retorna a lista de IDs das ferramentas que o especialista precisa utilizar
	ToolIDs() []string

	// Execute executa o subagente em uma tarefa com o histórico resumido anterior (priorSummary)
	Execute(ctx context.Context, prompt string, priorSummary string) (AgentResult, error)
}
