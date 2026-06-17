package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WriteFileTool grava ou sobrescreve arquivos dentro do workspace
type WriteFileTool struct {
	workspaceRoot string
	jail          bool
}

// NewWriteFileTool cria a ferramenta write_file
func NewWriteFileTool(workspaceRoot string, jail bool) *WriteFileTool {
	return &WriteFileTool{
		workspaceRoot: workspaceRoot,
		jail:          jail,
	}
}

// ID retorna o identificador da ferramenta
func (t *WriteFileTool) ID() string {
	return "write_file"
}

// Description retorna a descrição legível
func (t *WriteFileTool) Description() string {
	return "Escreve ou sobrescreve conteúdo textual em um arquivo."
}

// ParametersSchema define a assinatura JSON Schema da ferramenta
func (t *WriteFileTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Caminho relativo ou absoluto do arquivo para gravar"
			},
			"content": {
				"type": "string",
				"description": "Conteúdo textual completo a ser gravado no arquivo"
			}
		},
		"required": ["path", "content"]
	}`)
}

// RequiresApproval define se a ferramenta precisa de HITL
func (t *WriteFileTool) RequiresApproval() bool {
	return true
}

// Execute roda a escrita do arquivo
func (t *WriteFileTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var input struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return Result{Success: false, Error: "argumentos inválidos de JSON"}, nil
	}

	targetFile, err := ValidatePath(t.workspaceRoot, input.Path, t.jail)
	if err != nil {
		return Result{Success: false, Error: err.Error()}, nil
	}

	if err := os.MkdirAll(filepath.Dir(targetFile), 0755); err != nil {
		return Result{Success: false, Error: fmt.Sprintf("erro ao criar diretórios pai: %s", err.Error())}, nil
	}

	if err := os.WriteFile(targetFile, []byte(input.Content), 0644); err != nil {
		return Result{Success: false, Error: fmt.Sprintf("erro ao gravar arquivo: %s", err.Error())}, nil
	}

	return Result{Success: true, Data: "Arquivo gravado com sucesso."}, nil
}
