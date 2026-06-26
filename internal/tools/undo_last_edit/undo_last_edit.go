package undo_last_edit

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
		panic("falha ao carregar metadados de undo_last_edit: " + err.Error())
	}
}

type UndoLastEditTool struct {
	workspaceRoot string
	jail          bool
}

func NewUndoLastEditTool(workspaceRoot string, jail bool) *UndoLastEditTool {
	return &UndoLastEditTool{
		workspaceRoot: workspaceRoot,
		jail:          jail,
	}
}

func (t *UndoLastEditTool) ID() string {
	return metadata.ID
}

func (t *UndoLastEditTool) Description() string {
	return metadata.Description
}

func (t *UndoLastEditTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Caminho relativo ou absoluto do arquivo para reverter a última edição"
			}
		},
		"required": ["path"]
	}`)
}

func (t *UndoLastEditTool) RequiresApproval() bool {
	return false
}

func (t *UndoLastEditTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
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

	backupFile := targetFile + ".bak"

	if _, err := os.Stat(backupFile); os.IsNotExist(err) {
		return tools.Result{Success: false, Error: fmt.Sprintf("Nenhum backup encontrado para '%s'. Não há o que reverter.", input.Path)}, nil
	}

	data, err := os.ReadFile(backupFile)
	if err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("erro ao ler backup: %v", err)}, nil
	}

	err = os.WriteFile(targetFile, data, 0644)
	if err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("erro ao restaurar arquivo: %v", err)}, nil
	}

	// Remove o backup após restaurar (ou mantemos para segurança? Vamos remover para indicar que consumiu o undo)
	_ = os.Remove(backupFile)

	return tools.Result{Success: true, Data: fmt.Sprintf("Arquivo '%s' restaurado para a versão anterior com sucesso.", input.Path)}, nil
}
