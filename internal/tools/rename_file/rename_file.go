package rename_file

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
		panic("falha ao carregar metadados de rename_file: " + err.Error())
	}
}

// RenameFileTool move ou renomeia um arquivo ou pasta dentro do workspace
type RenameFileTool struct {
	workspaceRoot string
	jail          bool
}

// NewRenameFileTool cria a ferramenta rename_file
func NewRenameFileTool(workspaceRoot string, jail bool) *RenameFileTool {
	return &RenameFileTool{
		workspaceRoot: workspaceRoot,
		jail:          jail,
	}
}

// ID retorna o identificador da ferramenta
func (t *RenameFileTool) ID() string {
	return metadata.ID
}

// Description retorna a descrição da ferramenta
func (t *RenameFileTool) Description() string {
	return metadata.Description
}

// ParametersSchema define os parâmetros aceitos via JSON
func (t *RenameFileTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"src_path": {
				"type": "string",
				"description": "Caminho de origem do arquivo ou pasta a ser movido/renomeado"
			},
			"dest_path": {
				"type": "string",
				"description": "Caminho de destino do arquivo ou pasta"
			}
		},
		"required": ["src_path", "dest_path"]
	}`)
}

// RequiresApproval indica que esta ferramenta requer aprovação HITL (nível Média)
func (t *RenameFileTool) RequiresApproval() bool {
	return true
}

// Execute move ou renomeia o arquivo
func (t *RenameFileTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		SrcPath  string `json:"src_path"`
		DestPath string `json:"dest_path"`
	}

	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Result{Success: false, Error: "argumentos inválidos de JSON"}, nil
	}

	srcFile, err := tools.ValidatePath(t.workspaceRoot, input.SrcPath, t.jail)
	if err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("origem inválida: %s", err.Error())}, nil
	}

	destFile, err := tools.ValidatePath(t.workspaceRoot, input.DestPath, t.jail)
	if err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("destino inválido: %s", err.Error())}, nil
	}

	// Certificar que a pasta de destino existe
	destDir := filepath.Dir(destFile)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("erro ao criar subdiretórios de destino: %s", err.Error())}, nil
	}

	// Executar o rename
	if err := os.Rename(srcFile, destFile); err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("erro ao mover arquivo/diretório: %s", err.Error())}, nil
	}

	return tools.Result{
		Success: true,
		Data:    fmt.Sprintf("arquivo/diretório movido com sucesso de %s para %s", input.SrcPath, input.DestPath),
	}, nil
}
