package dependency_graph

import (
	"context"
	_ "embed"
	"encoding/json"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
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
		panic("falha ao carregar metadados de dependency_graph: " + err.Error())
	}
}

type DependencyGraphTool struct {
	workspaceRoot string
	workspaceJail bool
}

func NewDependencyGraphTool(workspaceRoot string, workspaceJail bool) *DependencyGraphTool {
	return &DependencyGraphTool{
		workspaceRoot: workspaceRoot,
		workspaceJail: workspaceJail,
	}
}

func (t *DependencyGraphTool) ID() string { return metadata.ID }

func (t *DependencyGraphTool) Description() string { return metadata.Description }

func (t *DependencyGraphTool) RequiresApproval() bool { return false }

func (t *DependencyGraphTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"target_path": {
				"type": "string",
				"description": "Caminho relativo do arquivo ou diretório de interesse"
			}
		},
		"required": ["target_path"]
	}`)
}

type GraphResult struct {
	TargetPath   string   `json:"target_path"`
	Dependencies []string `json:"dependencies"` // Arquivos/Pacotes que o alvo importa
	Dependents   []string `json:"dependents"`   // Arquivos locais que importam o alvo
}

func (t *DependencyGraphTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		TargetPath string `json:"target_path"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Result{Success: false, Error: "argumentos inválidos"}, nil
	}

	absTarget := input.TargetPath
	if !filepath.IsAbs(absTarget) {
		absTarget = filepath.Join(t.workspaceRoot, absTarget)
	}

	if t.workspaceJail {
		rel, err := filepath.Rel(t.workspaceRoot, absTarget)
		if err != nil || strings.HasPrefix(rel, "..") {
			return tools.Result{Success: false, Error: "acesso negado: fora do workspace"}, nil
		}
	}

	// 1. Escanear todos os arquivos do projeto para mapear imports
	importsMap := make(map[string][]string) // file rel path -> list of import paths or package paths
	var allFiles []string

	_ = filepath.Walk(t.workspaceRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == ".crom" || name == "node_modules" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		if ext == ".go" || ext == ".py" || ext == ".js" || ext == ".ts" {
			allFiles = append(allFiles, path)
		}
		return nil
	})

	pyImportRegex := regexp.MustCompile(`^\s*(?:import\s+([a-zA-Z0-9_.,\s]+)|from\s+([a-zA-Z0-9_.]+)\s+import)`)
	jsImportRegex := regexp.MustCompile(`(?:import\s+.*from\s+['"]([^'"]+)['"]|require\s*\(\s*['"]([^'"]+)['"]\s*\))`)

	for _, file := range allFiles {
		relFile, _ := filepath.Rel(t.workspaceRoot, file)
		ext := filepath.Ext(file)

		if ext == ".go" {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, file, nil, parser.ImportsOnly)
			if err == nil {
				for _, imp := range f.Imports {
					pathVal := strings.Trim(imp.Path.Value, `"`)
					importsMap[relFile] = append(importsMap[relFile], pathVal)
				}
			}
		} else {
			contentBytes, err := os.ReadFile(file)
			if err == nil {
				lines := strings.Split(string(contentBytes), "\n")
				for _, line := range lines {
					if ext == ".py" {
						if m := pyImportRegex.FindStringSubmatch(line); m != nil {
							impName := m[1]
							if impName == "" {
								impName = m[2]
							}
							importsMap[relFile] = append(importsMap[relFile], strings.TrimSpace(impName))
						}
					} else if ext == ".js" || ext == ".ts" {
						if m := jsImportRegex.FindStringSubmatch(line); m != nil {
							impName := m[1]
							if impName == "" {
								impName = m[2]
							}
							importsMap[relFile] = append(importsMap[relFile], strings.TrimSpace(impName))
						}
					}
				}
			}
		}
	}

	// 2. Identificar dependências e dependentes do alvo
	relTarget, _ := filepath.Rel(t.workspaceRoot, absTarget)
	var result GraphResult
	result.TargetPath = relTarget

	// Adiciona dependências do alvo
	if deps, ok := importsMap[relTarget]; ok {
		result.Dependencies = deps
	}

	// Identificar quem importa o alvo
	// Para Go: podemos usar o nome do pacote ou o subdiretório
	// Para Python/JS: podemos checar se o import contem o nome ou caminho relativo do alvo
	targetBase := filepath.Base(relTarget)
	targetDir := filepath.Dir(relTarget)
	targetModuleName := strings.TrimSuffix(targetBase, filepath.Ext(targetBase))

	for file, deps := range importsMap {
		if file == relTarget {
			continue
		}
		imported := false
		for _, dep := range deps {
			// Checagens aproximadas/semânticas
			if strings.Contains(dep, targetModuleName) ||
				(targetDir != "." && strings.Contains(dep, targetDir)) ||
				strings.HasSuffix(dep, relTarget) {
				imported = true
				break
			}
		}
		if imported {
			result.Dependents = append(result.Dependents, file)
		}
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return tools.Result{
		Success: true,
		Data:    string(data),
	}, nil
}
