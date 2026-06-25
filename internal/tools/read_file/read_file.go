package read_file

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"

	"github.com/crom/crom-agente/internal/tools"
)

//go:embed metadata.json
var metadataJSON []byte

var metadata tools.ToolMetadata

func init() {
	var err error
	metadata, err = tools.ParseMetadata(metadataJSON)
	if err != nil {
		panic("falha ao carregar metadados de read_file: " + err.Error())
	}
}

// ReadFileTool lê arquivos de texto dentro do workspace
type ReadFileTool struct {
	workspaceRoot string
	jail          bool
}

// NewReadFileTool cria a ferramenta read_file
func NewReadFileTool(workspaceRoot string, jail bool) *ReadFileTool {
	return &ReadFileTool{
		workspaceRoot: workspaceRoot,
		jail:          jail,
	}
}

// ID retorna o identificador da ferramenta
func (t *ReadFileTool) ID() string {
	return metadata.ID
}

// Description retorna a descrição legível
func (t *ReadFileTool) Description() string {
	return metadata.Description
}

// ParametersSchema define a assinatura JSON Schema da ferramenta
func (t *ReadFileTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Caminho relativo ou absoluto do arquivo a ser lido"
			}
		},
		"required": ["path"]
	}`)
}

// RequiresApproval define se a ferramenta precisa de HITL
func (t *ReadFileTool) RequiresApproval() bool {
	return false
}

// Execute roda a leitura do arquivo
func (t *ReadFileTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Result{Success: false, Error: "argumentos inválidos de JSON"}, nil
	}

	targetFile, err := tools.ValidatePath(t.workspaceRoot, input.Path, t.jail)
	if err != nil {
		return tools.Result{Success: false, Error: err.Error()}, nil
	}

	data, err := os.ReadFile(targetFile)
	if err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("erro ao ler arquivo: %s", err.Error())}, nil
	}

	return tools.Result{Success: true, Data: string(data)}, nil
}
