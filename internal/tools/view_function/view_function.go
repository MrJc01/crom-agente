package view_function

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/crom/crom-agente/internal/tools"
)

//go:embed metadata.json
var metadataJSON []byte

var metadata tools.ToolMetadata

func init() {
	var err error
	metadata, err = tools.ParseMetadata(metadataJSON)
	if err != nil {
		panic("falha ao carregar metadados de view_function: " + err.Error())
	}
}

type ViewFunctionTool struct {
	workspaceRoot string
	jail          bool
}

func NewViewFunctionTool(workspaceRoot string, jail bool) *ViewFunctionTool {
	return &ViewFunctionTool{
		workspaceRoot: workspaceRoot,
		jail:          jail,
	}
}

func (t *ViewFunctionTool) ID() string {
	return metadata.ID
}

func (t *ViewFunctionTool) Description() string {
	return metadata.Description
}

func (t *ViewFunctionTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Caminho relativo ou absoluto do arquivo"
			},
			"name": {
				"type": "string",
				"description": "Nome exato da função ou classe que você quer visualizar"
			}
		},
		"required": ["path", "name"]
	}`)
}

func (t *ViewFunctionTool) RequiresApproval() bool {
	return false
}

func (t *ViewFunctionTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}

	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Result{Success: false, Error: "argumentos inválidos de JSON"}, nil
	}

	targetPath, err := tools.ValidatePath(t.workspaceRoot, input.Path, t.jail)
	if err != nil {
		return tools.Result{Success: false, Error: err.Error()}, nil
	}

	contentBytes, err := os.ReadFile(targetPath)
	if err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("falha ao ler arquivo: %v", err)}, nil
	}

	ext := strings.ToLower(filepath.Ext(targetPath))
	var startLine, endLine int

	if ext == ".go" {
		fset := token.NewFileSet()
		node, err := parser.ParseFile(fset, targetPath, contentBytes, 0)
		if err != nil {
			return tools.Result{Success: false, Error: fmt.Sprintf("erro no parser Go AST: %s", err)}, nil
		}

		for _, decl := range node.Decls {
			if fd, ok := decl.(*ast.FuncDecl); ok {
				if fd.Name.Name == input.Name {
					startLine = fset.Position(fd.Pos()).Line
					endLine = fset.Position(fd.End()).Line
					break
				}
			}
			if gd, ok := decl.(*ast.GenDecl); ok && gd.Tok == token.TYPE {
				for _, spec := range gd.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok {
						if ts.Name.Name == input.Name {
							startLine = fset.Position(gd.Pos()).Line
							endLine = fset.Position(gd.End()).Line
							break
						}
					}
				}
			}
		}
	} else if ext == ".py" {
		// Python AST extrator inline
		pyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(pyCtx, "python3", "-c", pythonASTExtractorScript, targetPath, input.Name)
		out, errCmd := cmd.Output()
		if errCmd != nil {
			return tools.Result{Success: false, Error: fmt.Sprintf("falha ao extrair node AST do python: %v", errCmd)}, nil
		}
		var res struct {
			Start int `json:"start"`
			End   int `json:"end"`
		}
		if err = json.Unmarshal(out, &res); err == nil && res.Start > 0 {
			startLine = res.Start
			endLine = res.End
		}
	} else {
		return tools.Result{Success: false, Error: "view_function suporta apenas arquivos .go e .py atualmente."}, nil
	}

	if startLine == 0 {
		return tools.Result{Success: false, Error: fmt.Sprintf("Função ou Classe '%s' não encontrada no arquivo.", input.Name)}, nil
	}

	lines := strings.Split(string(contentBytes), "\n")
	if startLine > len(lines) {
		startLine = len(lines)
	}
	if endLine > len(lines) {
		endLine = len(lines)
	}

	extracted := strings.Join(lines[startLine-1:endLine], "\n")
	return tools.Result{Success: true, Data: fmt.Sprintf("Visualizando linhas %d a %d:\n\n%s", startLine, endLine, extracted)}, nil
}

const pythonASTExtractorScript = `
import ast, sys, json

filepath = sys.argv[1]
target_name = sys.argv[2]

with open(filepath, 'r', encoding='utf-8', errors='replace') as f:
    source = f.read()

try:
    tree = ast.parse(source, filename=filepath)
except SyntaxError:
    print("{}")
    sys.exit(1)

for node in ast.walk(tree):
    if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef, ast.ClassDef)):
        if node.name == target_name:
            print(json.dumps({"start": node.lineno, "end": node.end_lineno}))
            sys.exit(0)

print("{}")
`
