package read_session_messages

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/crom/crom-agente/internal/tools"
)

//go:embed metadata.json
var metadataJSON []byte

var metadata tools.ToolMetadata

func init() {
	var err error
	metadata, err = tools.ParseMetadata(metadataJSON)
	if err != nil {
		panic("falha ao carregar metadados de read_session_messages: " + err.Error())
	}
}

// ReadSessionMessagesTool lê mensagens de estado de sessões do agente
type ReadSessionMessagesTool struct {
	workspaceRoot string
}

// NewReadSessionMessagesTool cria a ferramenta
func NewReadSessionMessagesTool(workspaceRoot string) *ReadSessionMessagesTool {
	return &ReadSessionMessagesTool{
		workspaceRoot: workspaceRoot,
	}
}

// ID retorna o identificador da ferramenta
func (t *ReadSessionMessagesTool) ID() string {
	return metadata.ID
}

// Description retorna a descrição
func (t *ReadSessionMessagesTool) Description() string {
	return metadata.Description
}

// ParametersSchema define a assinatura da ferramenta
func (t *ReadSessionMessagesTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"session_id": {
				"type": "string",
				"description": "ID da sessão que se deseja analisar (ex: session-1782423890302-990753)"
			}
		},
		"required": ["session_id"]
	}`)
}

// RequiresApproval define se precisa de HITL
func (t *ReadSessionMessagesTool) RequiresApproval() bool {
	return false
}

// Execute executa a leitura das mensagens da sessão
func (t *ReadSessionMessagesTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Result{Success: false, Error: "argumentos inválidos de JSON"}, nil
	}

	if input.SessionID == "" {
		return tools.Result{Success: false, Error: "session_id não pode ser vazio"}, nil
	}

	sessionFilePath := filepath.Join(t.workspaceRoot, ".crom", "sessions", input.SessionID, "session.json")
	
	data, err := os.ReadFile(sessionFilePath)
	if err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("sessão não encontrada ou erro ao ler: %s", err.Error())}, nil
	}

	var sessionState struct {
		Messages []interface{} `json:"messages"`
	}
	if err := json.Unmarshal(data, &sessionState); err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("erro ao decodificar JSON de sessão: %s", err.Error())}, nil
	}

	respBytes, err := json.Marshal(sessionState.Messages)
	if err != nil {
		return tools.Result{Success: false, Error: "erro ao codificar mensagens de resposta: " + err.Error()}, nil
	}

	return tools.Result{Success: true, Data: string(respBytes)}, nil
}
