package syntax_check

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
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
		panic("falha ao carregar metadados de syntax_check: " + err.Error())
	}
}

// SyntaxCheckTool verifica sintaxe de arquivos antes de testar
type SyntaxCheckTool struct {
	workspaceRoot string
	jail          bool
}

// NewSyntaxCheckTool cria a ferramenta de checagem de sintaxe
func NewSyntaxCheckTool(workspaceRoot string, jail bool) *SyntaxCheckTool {
	return &SyntaxCheckTool{
		workspaceRoot: workspaceRoot,
		jail:          jail,
	}
}

func (t *SyntaxCheckTool) ID() string { return metadata.ID }

func (t *SyntaxCheckTool) Description() string { return metadata.Description }

func (t *SyntaxCheckTool) RequiresApproval() bool { return false }

func (t *SyntaxCheckTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Caminho do arquivo no workspace para verificar a sintaxe"
			}
		},
		"required": ["path"]
	}`)
}

func (t *SyntaxCheckTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Result{Success: false, Error: "argumentos inválidos"}, nil
	}

	targetFile, err := tools.ValidatePath(t.workspaceRoot, input.Path, t.jail)
	if err != nil {
		return tools.Result{Success: false, Error: err.Error()}, nil
	}

	// Verifica se arquivo existe e é regular
	info, err := os.Stat(targetFile)
	if err != nil {
		return tools.Result{Success: false, Error: "arquivo não encontrado: " + err.Error()}, nil
	}
	if info.IsDir() {
		return tools.Result{Success: false, Error: "o caminho especificado é um diretório"}, nil
	}

	ext := filepath.Ext(targetFile)
	if ext == ".go" {
		fset := token.NewFileSet()
		_, err := parser.ParseFile(fset, targetFile, nil, parser.AllErrors)
		if err != nil {
			return tools.Result{
				Success: false,
				Error:   fmt.Sprintf("erro de sintaxe detectado no arquivo Go: %v", err),
			}, nil
		}
	}

	return tools.Result{
		Success: true,
		Data:    fmt.Sprintf("✓ Sintaxe do arquivo %s está correta.", input.Path),
	}, nil
}
