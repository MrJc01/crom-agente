package ast_analyzer

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
	"regexp"
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
		panic("falha ao carregar metadados de ast_analyzer: " + err.Error())
	}
}

type ASTAnalyzerTool struct {
	workspaceRoot string
	workspaceJail bool
}

func NewASTAnalyzerTool(workspaceRoot string, workspaceJail bool) *ASTAnalyzerTool {
	return &ASTAnalyzerTool{
		workspaceRoot: workspaceRoot,
		workspaceJail: workspaceJail,
	}
}

func (t *ASTAnalyzerTool) ID() string { return metadata.ID }

func (t *ASTAnalyzerTool) Description() string { return metadata.Description }

func (t *ASTAnalyzerTool) RequiresApproval() bool { return false }

func (t *ASTAnalyzerTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Caminho relativo ou absoluto para o arquivo de código"
			}
		},
		"required": ["path"]
	}`)
}

type FuncDecl struct {
	Name     string `json:"name"`
	Receiver string `json:"receiver,omitempty"`
	Sig      string `json:"signature"`
	Line     int    `json:"line"`
}

type TypeDecl struct {
	Name string `json:"name"`
	Kind string `json:"kind"` // struct, interface, class
	Line int    `json:"line"`
}

type AnalysisResult struct {
	FilePath  string     `json:"file_path"`
	Functions []FuncDecl `json:"functions"`
	Types     []TypeDecl `json:"types"`
}

func (t *ASTAnalyzerTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Result{Success: false, Error: "argumentos inválidos"}, nil
	}

	targetPath := input.Path
	if !filepath.IsAbs(targetPath) {
		targetPath = filepath.Join(t.workspaceRoot, targetPath)
	}

	// Simplificado check jail
	if t.workspaceJail {
		rel, err := filepath.Rel(t.workspaceRoot, targetPath)
		if err != nil || strings.HasPrefix(rel, "..") {
			return tools.Result{Success: false, Error: "acesso negado: fora do workspace"}, nil
		}
	}

	contentBytes, err := os.ReadFile(targetPath)
	if err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("falha ao ler arquivo: %s", err)}, nil
	}

	ext := strings.ToLower(filepath.Ext(targetPath))
	var result AnalysisResult
	result.FilePath, _ = filepath.Rel(t.workspaceRoot, targetPath)

	if ext == ".go" {
		fset := token.NewFileSet()
		node, err := parser.ParseFile(fset, targetPath, contentBytes, parser.ParseComments)
		if err != nil {
			return tools.Result{Success: false, Error: fmt.Sprintf("erro no parser Go AST: %s", err)}, nil
		}

		for _, decl := range node.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				fn := FuncDecl{
					Name: d.Name.Name,
					Line: fset.Position(d.Pos()).Line,
				}

				// Receiver (método)
				if d.Recv != nil && len(d.Recv.List) > 0 {
					var recvType strings.Builder
					for _, field := range d.Recv.List {
						ast.Fprint(&recvType, fset, field.Type, nil)
					}
					fn.Receiver = strings.TrimSpace(recvType.String())
				}

				// Assinatura
				var params []string
				if d.Type.Params != nil {
					for _, param := range d.Type.Params.List {
						var paramType strings.Builder
						_ = ast.Fprint(&paramType, fset, param.Type, nil)
						pType := strings.TrimSpace(paramType.String())
						if len(param.Names) > 0 {
							for _, n := range param.Names {
								params = append(params, n.Name+" "+pType)
							}
						} else {
							params = append(params, pType)
						}
					}
				}

				var results []string
				if d.Type.Results != nil {
					for _, res := range d.Type.Results.List {
						var resType strings.Builder
						_ = ast.Fprint(&resType, fset, res.Type, nil)
						rType := strings.TrimSpace(resType.String())
						if len(res.Names) > 0 {
							for _, n := range res.Names {
								results = append(results, n.Name+" "+rType)
							}
						} else {
							results = append(results, rType)
						}
					}
				}

				fn.Sig = fmt.Sprintf("func (%s) (%s)", strings.Join(params, ", "), strings.Join(results, ", "))
				result.Functions = append(result.Functions, fn)

			case *ast.GenDecl:
				if d.Tok == token.TYPE {
					for _, spec := range d.Specs {
						if ts, ok := spec.(*ast.TypeSpec); ok {
							kind := "type"
							switch ts.Type.(type) {
							case *ast.StructType:
								kind = "struct"
							case *ast.InterfaceType:
								kind = "interface"
							}
							result.Types = append(result.Types, TypeDecl{
								Name: ts.Name.Name,
								Kind: kind,
								Line: fset.Position(ts.Pos()).Line,
							})
						}
					}
				}
			}
		}
	} else if ext == ".py" {
		// AST real via python3 subprocess (Task 75)
		if pyResult, ok := t.analyzePythonAST(ctx, targetPath); ok {
			result.Functions = append(result.Functions, pyResult.Functions...)
			result.Types = append(result.Types, pyResult.Types...)
		} else {
			// Fallback regex para Python se python3 não estiver disponível
			t.analyzePythonRegex(string(contentBytes), &result)
		}
	} else {
		// Regex-based fallback para JS/TS
		content := string(contentBytes)
		lines := strings.Split(content, "\n")

		jsFuncRegex := regexp.MustCompile(`(?:function\s+([a-zA-Z0-9_]+)|const\s+([a-zA-Z0-9_]+)\s*=\s*(?:\([^)]*\)|[a-zA-Z0-9_]+)\s*=>)`)
		jsClassRegex := regexp.MustCompile(`^\s*class\s+([a-zA-Z0-9_]+)`)

		for lineIdx, line := range lines {
			lineNum := lineIdx + 1
			if m := jsFuncRegex.FindStringSubmatch(line); m != nil {
				name := m[1]
				if name == "" {
					name = m[2]
				}
				result.Functions = append(result.Functions, FuncDecl{
					Name: name,
					Line: lineNum,
				})
			} else if m := jsClassRegex.FindStringSubmatch(line); m != nil {
				result.Types = append(result.Types, TypeDecl{
					Name: m[1],
					Kind: "class",
					Line: lineNum,
				})
			}
		}
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return tools.Result{
		Success: true,
		Data:    string(data),
	}, nil
}

// pythonASTScript é o script Python inline para análise AST real
const pythonASTScript = `
import ast, json, sys

def analyze(filepath):
    with open(filepath, 'r', encoding='utf-8', errors='replace') as f:
        source = f.read()
    try:
        tree = ast.parse(source, filename=filepath)
    except SyntaxError as e:
        print(json.dumps({"error": str(e)}))
        return
    
    functions = []
    types = []
    
    for node in ast.walk(tree):
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
            decorators = [d.id if isinstance(d, ast.Name) else ast.dump(d) for d in node.decorator_list[:3]]
            args = []
            for arg in node.args.args:
                annotation = ""
                if arg.annotation:
                    try:
                        annotation = ast.unparse(arg.annotation)
                    except:
                        annotation = "?"
                args.append({"name": arg.arg, "type": annotation})
            
            ret_type = ""
            if node.returns:
                try:
                    ret_type = ast.unparse(node.returns)
                except:
                    ret_type = "?"
            
            sig_parts = []
            for a in args:
                if a["type"]:
                    sig_parts.append(f"{a['name']}: {a['type']}")
                else:
                    sig_parts.append(a['name'])
            sig = f"({', '.join(sig_parts)})"
            if ret_type:
                sig += f" -> {ret_type}"
            
            prefix = "async def" if isinstance(node, ast.AsyncFunctionDef) else "def"
            functions.append({
                "name": node.name,
                "signature": f"{prefix} {node.name}{sig}",
                "line": node.lineno,
                "decorators": decorators
            })
        
        elif isinstance(node, ast.ClassDef):
            bases = []
            for base in node.bases[:5]:
                try:
                    bases.append(ast.unparse(base))
                except:
                    bases.append("?")
            types.append({
                "name": node.name,
                "kind": "class",
                "line": node.lineno,
                "bases": bases
            })
    
    print(json.dumps({"functions": functions, "types": types}))

analyze(sys.argv[1])
`

// analyzePythonAST executa o AST parser Python real via subprocess
func (t *ASTAnalyzerTool) analyzePythonAST(ctx context.Context, filePath string) (*AnalysisResult, bool) {
	// Timeout de 5s para o subprocess
	pyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(pyCtx, "python3", "-c", pythonASTScript, filePath)
	out, err := cmd.Output()
	if err != nil {
		return nil, false
	}

	var pyResult struct {
		Functions []struct {
			Name       string   `json:"name"`
			Signature  string   `json:"signature"`
			Line       int      `json:"line"`
			Decorators []string `json:"decorators"`
		} `json:"functions"`
		Types []struct {
			Name  string   `json:"name"`
			Kind  string   `json:"kind"`
			Line  int      `json:"line"`
			Bases []string `json:"bases"`
		} `json:"types"`
		Error string `json:"error"`
	}

	if err := json.Unmarshal(out, &pyResult); err != nil || pyResult.Error != "" {
		return nil, false
	}

	result := &AnalysisResult{}
	for _, f := range pyResult.Functions {
		fd := FuncDecl{
			Name: f.Name,
			Sig:  f.Signature,
			Line: f.Line,
		}
		if len(f.Decorators) > 0 {
			fd.Receiver = "@" + strings.Join(f.Decorators, ", @")
		}
		result.Functions = append(result.Functions, fd)
	}
	for _, t := range pyResult.Types {
		td := TypeDecl{
			Name: t.Name,
			Kind: t.Kind,
			Line: t.Line,
		}
		if len(t.Bases) > 0 {
			td.Kind = "class(" + strings.Join(t.Bases, ", ") + ")"
		}
		result.Types = append(result.Types, td)
	}

	return result, true
}

// analyzePythonRegex fallback de análise Python usando regex (para sistemas sem python3)
func (t *ASTAnalyzerTool) analyzePythonRegex(content string, result *AnalysisResult) {
	lines := strings.Split(content, "\n")
	pyFuncRegex := regexp.MustCompile(`^\s*(?:async\s+)?def\s+([a-zA-Z0-9_]+)\s*\((.*?)\)`)
	pyClassRegex := regexp.MustCompile(`^\s*class\s+([a-zA-Z0-9_]+)`)

	for lineIdx, line := range lines {
		lineNum := lineIdx + 1
		if m := pyFuncRegex.FindStringSubmatch(line); m != nil {
			result.Functions = append(result.Functions, FuncDecl{
				Name: m[1],
				Sig:  m[2],
				Line: lineNum,
			})
		} else if m := pyClassRegex.FindStringSubmatch(line); m != nil {
			result.Types = append(result.Types, TypeDecl{
				Name: m[1],
				Kind: "class",
				Line: lineNum,
			})
		}
	}
}

