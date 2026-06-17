package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DeleteFileTool remove permanentemente arquivos ou pastas do workspace
type DeleteFileTool struct {
	workspaceRoot string
	jail          bool
}

// NewDeleteFileTool cria a ferramenta delete_file
func NewDeleteFileTool(workspaceRoot string, jail bool) *DeleteFileTool {
	return &DeleteFileTool{
		workspaceRoot: workspaceRoot,
		jail:          jail,
	}
}

// ID retorna o identificador da ferramenta
func (t *DeleteFileTool) ID() string {
	return "delete_file"
}

// Description retorna a descrição da ferramenta
func (t *DeleteFileTool) Description() string {
	return "Deleta permanentemente um arquivo ou pasta dentro do workspace. Possui travas de segurança para arquivos vitais do repositório."
}

// ParametersSchema define os parâmetros aceitos via JSON
func (t *DeleteFileTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Caminho do arquivo ou diretório a ser excluído permanentemente"
			}
		},
		"required": ["path"]
	}`)
}

// RequiresApproval indica que esta ferramenta requer aprovação HITL (nível Crítica)
func (t *DeleteFileTool) RequiresApproval() bool {
	return true
}

// Execute realiza a deleção do arquivo ou pasta com verificações críticas
func (t *DeleteFileTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var input struct {
		Path string `json:"path"`
	}

	if err := json.Unmarshal(args, &input); err != nil {
		return Result{Success: false, Error: "argumentos inválidos de JSON"}, nil
	}

	targetFile, err := ValidatePath(t.workspaceRoot, input.Path, t.jail)
	if err != nil {
		return Result{Success: false, Error: err.Error()}, nil
	}

	absWorkspace, err := filepath.Abs(t.workspaceRoot)
	if err != nil {
		return Result{Success: false, Error: "workspace root inválido"}, nil
	}

	// 1. Impedir deletar o próprio diretório raiz do workspace
	if targetFile == absWorkspace {
		return Result{Success: false, Error: "acesso negado: não é permitido deletar o diretório raiz do workspace"}, nil
	}

	// 2. Extrair caminhos relativos para validação
	rel, err := filepath.Rel(absWorkspace, targetFile)
	if err != nil {
		return Result{Success: false, Error: "caminho relativo inválido"}, nil
	}
	relClean := filepath.Clean(rel)
	parts := strings.Split(relClean, string(filepath.Separator))

	// 3. Bloquear deleção de repositório git ou pasta github
	for _, part := range parts {
		if part == ".git" || part == ".github" {
			return Result{Success: false, Error: "acesso negado: não é permitido deletar diretórios do Git ou GitHub (.git/.github)"}, nil
		}
	}

	// 4. Bloquear arquivos de configuração do crom-agente
	if relClean == ".crom" || strings.HasPrefix(relClean, ".crom"+string(filepath.Separator)) || relClean == ".crom_state.json" {
		return Result{Success: false, Error: "acesso negado: não é permitido deletar arquivos ou pastas de configuração do crom-agente"}, nil
	}

	// 5. Bloquear go.mod principal da raiz
	if relClean == "go.mod" || relClean == "go.sum" {
		return Result{Success: false, Error: "acesso negado: não é permitido deletar o go.mod ou go.sum do projeto raiz"}, nil
	}

	// 6. Verificar se o caminho realmente existe antes de deletar
	if _, err := os.Stat(targetFile); err != nil {
		if os.IsNotExist(err) {
			return Result{Success: false, Error: fmt.Sprintf("arquivo ou diretório não existe: %s", input.Path)}, nil
		}
		return Result{Success: false, Error: err.Error()}, nil
	}

	// 7. Executar a exclusão de fato
	if err := os.RemoveAll(targetFile); err != nil {
		return Result{Success: false, Error: fmt.Sprintf("erro ao deletar arquivo/diretório: %s", err.Error())}, nil
	}

	return Result{
		Success: true,
		Data:    fmt.Sprintf("arquivo ou pasta deletado com sucesso: %s", input.Path),
	}, nil
}
