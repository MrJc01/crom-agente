package spawn_subagent

import (
	"context"
	_ "embed"
	"encoding/json"

	"github.com/crom/crom-agente/internal/tools"
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
}

// SpawnSubagentTool permite criar agentes filhos assíncronos
type SpawnSubagentTool struct {
	spawnFunc func(ctx context.Context, agentName string, task string) (tools.Result, error)
}

// NewSpawnSubagentTool cria a ferramenta spawn_subagent
func NewSpawnSubagentTool(spawnFunc func(ctx context.Context, agentName string, task string) (tools.Result, error)) *SpawnSubagentTool {
	return &SpawnSubagentTool{
		spawnFunc: spawnFunc,
	}
}

// ID retorna o identificador da ferramenta
func (t *SpawnSubagentTool) ID() string {
	return metadata.ID
}

// Description retorna a descrição legível
func (t *SpawnSubagentTool) Description() string {
	return metadata.Description
}

// ParametersSchema define a assinatura JSON Schema da ferramenta
func (t *SpawnSubagentTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"agent_name": {
				"type": "string",
				"description": "Nome do subagente a instanciar (ex: reviewer, documenter, tester). Se vazio, cria genérico."
			},
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
func (t *SpawnSubagentTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		AgentName string `json:"agent_name"`
		Task      string `json:"task"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Result{Success: false, Error: "argumentos inválidos de JSON"}, nil
	}

	if t.spawnFunc == nil {
		return tools.Result{Success: false, Error: "spawner de subagente não configurado"}, nil
	}

	return t.spawnFunc(ctx, input.AgentName, input.Task)
}
