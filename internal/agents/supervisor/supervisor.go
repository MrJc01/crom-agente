package supervisor

import (
	"context"

	"github.com/crom/crom-agente/internal/agents/core"
)

// Supervisor representa o agente orquestrador principal de uma sessão
type Supervisor struct {
	core.BaseAgent
}

// NewSupervisor cria um novo agente supervisor
func NewSupervisor(name string, description string, sysPrompt string) *Supervisor {
	return &Supervisor{
		BaseAgent: core.BaseAgent{
			AgentName:        name,
			AgentDescription: description,
			AgentSysPrompt:   sysPrompt,
		},
	}
}

// Execute executa o orquestrador principal (a orquestração principal roda no AgenticLoop, mas essa struct serve como ponto de extensão)
func (s *Supervisor) Execute(ctx context.Context, prompt string, priorSummary string) (core.AgentResult, error) {
	// Fallback/Placeholder de execução direta
	return core.AgentResult{
		Output:         "Supervisor " + s.AgentName + " chamado com prompt: " + prompt,
		ContextSummary: priorSummary,
	}, nil
}
