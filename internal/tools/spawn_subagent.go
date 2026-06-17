package tools

import (
	"context"
	"encoding/json"
)

// SpawnSubagentTool permite criar agentes filhos assíncronos
type SpawnSubagentTool struct {
	spawnFunc func(ctx context.Context, task string) (Result, error)
}

// NewSpawnSubagentTool cria a ferramenta spawn_subagent
func NewSpawnSubagentTool(spawnFunc func(ctx context.Context, task string) (Result, error)) *SpawnSubagentTool {
	return &SpawnSubagentTool{
		spawnFunc: spawnFunc,
	}
}

// ID retorna o identificador da ferramenta
func (t *SpawnSubagentTool) ID() string {
	return "spawn_subagent"
}

// Description retorna a descrição legível
func (t *SpawnSubagentTool) Description() string {
	return "Dispara um subagente em background em uma tarefa isolada."
}

// ParametersSchema define a assinatura JSON Schema da ferramenta
func (t *SpawnSubagentTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"task": {
				"type": "string",
				"description": "Instrução específica e detalhada para o subagente executar"
			}
		},
		"required": ["task"]
	}`)
}

// RequiresApproval define se a ferramenta precisa de HITL
func (t *SpawnSubagentTool) RequiresApproval() bool {
	return true
}

// Execute executa o callback de spawn
func (t *SpawnSubagentTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var input struct {
		Task string `json:"task"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return Result{Success: false, Error: "argumentos inválidos de JSON"}, nil
	}

	if t.spawnFunc == nil {
		return Result{Success: false, Error: "spawner de subagente não configurado"}, nil
	}

	return t.spawnFunc(ctx, input.Task)
}
