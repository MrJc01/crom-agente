package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/crom/crom-agente/internal/agents/core"
)

// AgentToolAdapter adapta a interface core.Agent para tools.Tool
type AgentToolAdapter struct {
	InnerAgent core.Agent
}

// NewAgentToolAdapter cria um novo adaptador para o agente especialista
func NewAgentToolAdapter(agent core.Agent) *AgentToolAdapter {
	return &AgentToolAdapter{
		InnerAgent: agent,
	}
}

// ID retorna o identificador único da ferramenta
func (a *AgentToolAdapter) ID() string {
	return a.InnerAgent.Name()
}

// Description descreve a ferramenta e orienta o LLM sobre o histórico resumido
func (a *AgentToolAdapter) Description() string {
	return fmt.Sprintf("%s. Use o campo 'prior_summary' para passar resumos de execuções anteriores com este agente.", a.InnerAgent.Description())
}

// ParametersSchema define a assinatura esperada pelo Supervisor
func (a *AgentToolAdapter) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"prompt": {
				"type": "string",
				"description": "Instrução específica ou tarefa detalhada para o especialista executar."
			},
			"prior_summary": {
				"type": "string",
				"description": "Histórico resumido opcional retornado por execuções anteriores deste mesmo especialista."
			}
		},
		"required": ["prompt"]
	}`)
}

// RequiresApproval define se a chamada precisa de confirmação
func (a *AgentToolAdapter) RequiresApproval() bool {
	return true
}

// Execute executa o agente interno desempacotando o JSON e retornando o resultado formatado em JSON
func (a *AgentToolAdapter) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var input struct {
		Prompt       string `json:"prompt"`
		PriorSummary string `json:"prior_summary"`
	}

	if err := json.Unmarshal(args, &input); err != nil {
		// Fallback se falhar ao fazer parse: usa todo o JSON como prompt
		input.Prompt = string(args)
	} else if input.Prompt == "" {
		// Fallback se desempacotou mas o prompt veio vazio (ex: formato de steps legados)
		input.Prompt = string(args)
	}

	if a.InnerAgent == nil {
		return Result{Success: false, Error: "agente especialista interno não configurado"}, nil
	}

	res, err := a.InnerAgent.Execute(ctx, input.Prompt, input.PriorSummary)
	if err != nil {
		return Result{Success: false, Error: fmt.Sprintf("erro na execução do especialista: %s", err.Error())}, nil
	}

	respBytes, err := json.Marshal(res)
	if err != nil {
		return Result{Success: false, Error: "falha ao serializar resposta do agente: " + err.Error()}, nil
	}

	return Result{
		Success: res.Success,
		Data:    string(respBytes),
	}, nil
}
