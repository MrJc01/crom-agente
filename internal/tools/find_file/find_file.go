package find_file

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/crom/crom-agente/internal/tools"
)

//go:embed metadata.json
var metadataJSON []byte

var metadata tools.ToolMetadata

func init() {
	var err error
	metadata, err = tools.ParseMetadata(metadataJSON)
	if err != nil {
		panic("falha ao carregar metadados de find_file: " + err.Error())
	}
}

// FindFileTool busca arquivos pelo nome
type FindFileTool struct {
	workspaceRoot string
	jail          bool
}

// NewFindFileTool cria a ferramenta find_file
func NewFindFileTool(workspaceRoot string, jail bool) *FindFileTool {
	return &FindFileTool{
		workspaceRoot: workspaceRoot,
		jail:          jail,
	}
}

// ID retorna o identificador da ferramenta
func (t *FindFileTool) ID() string {
	return metadata.ID
}

// Description retorna a descrição da ferramenta
func (t *FindFileTool) Description() string {
	return metadata.Description
}

// ParametersSchema define os parâmetros aceitos via JSON
func (t *FindFileTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {
				"type": "string",
				"description": "Padrão de busca do arquivo (ex: *.py ou main.go)"
			},
			"path": {
				"type": "string",
				"description": "Subdiretório para iniciar a busca (opcional)"
			}
		},
		"required": ["pattern"]
	}`)
}

// RequiresApproval indica que esta ferramenta não requer HITL
func (t *FindFileTool) RequiresApproval() bool {
	return false
}

// Execute roda a busca
func (t *FindFileTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}

	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Result{Success: false, Error: "argumentos inválidos de JSON"}, nil
	}

	startDir, err := tools.ValidatePath(t.workspaceRoot, input.Path, t.jail)
	if err != nil {
		return tools.Result{Success: false, Error: err.Error()}, nil
	}

	var results []string
	err = filepath.Walk(startDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		
		// Ignore common massive dirs
		if info.IsDir() {
			base := info.Name()
			if base == ".git" || base == "node_modules" || base == "venv" || base == ".venv" || base == "__pycache__" || base == "build" || base == "dist" {
				return filepath.SkipDir
			}
			return nil
		}

		matched, err := filepath.Match(input.Pattern, info.Name())
		if err == nil && matched {
			rel, _ := filepath.Rel(t.workspaceRoot, path)
			results = append(results, rel)
		}
		return nil
	})

	if err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("erro ao buscar: %v", err)}, nil
	}

	if len(results) == 0 {
		return tools.Result{Success: true, Data: fmt.Sprintf("Nenhum arquivo correspondente a '%s' encontrado.", input.Pattern)}, nil
	}

	return tools.Result{Success: true, Data: strings.Join(results, "\n")}, nil
}
