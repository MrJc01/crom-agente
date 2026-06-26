package call_graph

import (
	"context"
	_ "embed"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
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
		panic("falha ao carregar metadados de call_graph: " + err.Error())
	}
}

type CallGraphTool struct {
	workspaceRoot string
}

func NewCallGraphTool(workspaceRoot string) *CallGraphTool {
	return &CallGraphTool{workspaceRoot: workspaceRoot}
}

func (t *CallGraphTool) ID() string { return metadata.ID }
func (t *CallGraphTool) Description() string { return metadata.Description }
func (t *CallGraphTool) RequiresApproval() bool { return false }

func (t *CallGraphTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"directory": {
				"type": "string",
				"description": "Caminho relativo para o diretório do pacote Go a ser analisado."
			}
		},
		"required": ["directory"]
	}`)
}

type CallNode struct {
	Caller  string   `json:"caller"`
	Callees []string `json:"callees"`
}

func (t *CallGraphTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		Directory string `json:"directory"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Result{Success: false, Error: "argumentos inválidos"}, nil
	}

	targetDir := input.Directory
	if !filepath.IsAbs(targetDir) {
		targetDir = filepath.Join(t.workspaceRoot, targetDir)
	}

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, targetDir, nil, parser.ParseComments)
	if err != nil {
		return tools.Result{Success: false, Error: "erro ao parsear diretório: " + err.Error()}, nil
	}

	var results []CallNode
	for _, pkg := range pkgs {
		graph := make(map[string]map[string]bool)

		for _, file := range pkg.Files {
			var currentFunc string

			ast.Inspect(file, func(n ast.Node) bool {
				switch node := n.(type) {
				case *ast.FuncDecl:
					// Mapear nome da função (e receiver se for método)
					name := node.Name.Name
					if node.Recv != nil && len(node.Recv.List) > 0 {
						switch typeNode := node.Recv.List[0].Type.(type) {
						case *ast.Ident:
							name = typeNode.Name + "." + name
						case *ast.StarExpr:
							if ident, ok := typeNode.X.(*ast.Ident); ok {
								name = ident.Name + "." + name
							}
						}
					}
					currentFunc = name
					if graph[currentFunc] == nil {
						graph[currentFunc] = make(map[string]bool)
					}

				case *ast.CallExpr:
					if currentFunc != "" {
						switch fun := node.Fun.(type) {
						case *ast.Ident: // função normal do pacote
							graph[currentFunc][fun.Name] = true
						case *ast.SelectorExpr: // método ou função de outro pacote
							// Pode ser obj.Method() ou pkg.Func()
							// Registramos o Seletor completo para contexto
							if xIdent, ok := fun.X.(*ast.Ident); ok {
								calleeName := xIdent.Name + "." + fun.Sel.Name
								graph[currentFunc][calleeName] = true
							}
						}
					}
				}
				return true
			})
		}

		for caller, calleesMap := range graph {
			if len(calleesMap) > 0 {
				var callees []string
				for c := range calleesMap {
					callees = append(callees, c)
				}
				results = append(results, CallNode{Caller: caller, Callees: callees})
			}
		}
	}

	if len(results) == 0 {
		return tools.Result{Success: true, Data: "Nenhuma chamada de função encontrada no pacote."}, nil
	}

	data, _ := json.MarshalIndent(results, "", "  ")
	return tools.Result{Success: true, Data: string(data)}, nil
}
