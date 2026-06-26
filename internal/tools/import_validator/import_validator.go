package import_validator

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
		panic("falha ao carregar metadados de import_validator: " + err.Error())
	}
}

type ImportValidatorTool struct {
	workspaceRoot string
	workspaceJail bool
}

func NewImportValidatorTool(workspaceRoot string, workspaceJail bool) *ImportValidatorTool {
	return &ImportValidatorTool{
		workspaceRoot: workspaceRoot,
		workspaceJail: workspaceJail,
	}
}

func (t *ImportValidatorTool) ID() string { return metadata.ID }

func (t *ImportValidatorTool) Description() string { return metadata.Description }

func (t *ImportValidatorTool) RequiresApproval() bool { return false }

func (t *ImportValidatorTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"target_dir": {
				"type": "string",
				"description": "Diretório inicial para validar. Se vazio, valida todo o workspace."
			}
		},
		"required": []
	}`)
}

type ImportIssue struct {
	Type        string   `json:"type"`        // circular ou broken
	Files       []string `json:"files"`       // Arquivos envolvidos
	Description string   `json:"description"` // Detalhes
}

type ValidationResult struct {
	Valid  bool          `json:"valid"`
	Issues []ImportIssue `json:"issues"`
}

func (t *ImportValidatorTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		TargetDir string `json:"target_dir"`
	}
	_ = json.Unmarshal(args, &input)

	startDir := t.workspaceRoot
	if input.TargetDir != "" {
		startDir = input.TargetDir
		if !filepath.IsAbs(startDir) {
			startDir = filepath.Join(t.workspaceRoot, startDir)
		}
	}

	if t.workspaceJail {
		rel, err := filepath.Rel(t.workspaceRoot, startDir)
		if err != nil || strings.HasPrefix(rel, "..") {
			return tools.Result{Success: false, Error: "acesso negado: fora do workspace"}, nil
		}
	}

	// 1. Escanear arquivos
	var allFiles []string
	_ = filepath.Walk(startDir, func(path string, info os.FileInfo, err error) error {
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

	importsMap := make(map[string][]string) // arquivo rel -> imports
	brokenImports := make(map[string][]string)

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
					// Ignora imports do stdlib/externos normais
					if strings.Contains(pathVal, "/") {
						importsMap[relFile] = append(importsMap[relFile], pathVal)
					}
				}
			}
		} else {
			contentBytes, err := os.ReadFile(file)
			if err != nil {
				continue
			}
			lines := strings.Split(string(contentBytes), "\n")
			for _, line := range lines {
				var impPath string
				if ext == ".py" {
					if m := pyImportRegex.FindStringSubmatch(line); m != nil {
						impPath = m[1]
						if impPath == "" {
							impPath = m[2]
						}
					}
				} else if ext == ".js" || ext == ".ts" {
					if m := jsImportRegex.FindStringSubmatch(line); m != nil {
						impPath = m[1]
						if impPath == "" {
							impPath = m[2]
						}
					}
				}

				if impPath != "" {
					impPath = strings.TrimSpace(impPath)
					importsMap[relFile] = append(importsMap[relFile], impPath)

					// Validação básica de broken relative imports
					if strings.HasPrefix(impPath, ".") {
						dir := filepath.Dir(file)
						resolved := filepath.Clean(filepath.Join(dir, impPath))
						// Tenta resolver com extensões
						found := false
						for _, checkExt := range []string{"", ".js", ".ts", ".py"} {
							if stat, statErr := os.Stat(resolved + checkExt); statErr == nil {
								_ = stat
								found = true
								break
							}
						}
						if !found {
							brokenImports[relFile] = append(brokenImports[relFile], impPath)
						}
					}
				}
			}
		}
	}

	var issues []ImportIssue

	// 1. Relatar imports quebrados
	for file, brokens := range brokenImports {
		issues = append(issues, ImportIssue{
			Type:        "broken",
			Files:       []string{file},
			Description: "Importações relativas quebradas: " + strings.Join(brokens, ", "),
		})
	}

	// 2. Detecção de Ciclos (Circular Imports) usando DFS
	// Construindo grafo de dependências físicas
	adj := make(map[string][]string)
	for file, deps := range importsMap {
		for _, dep := range deps {
			// Resolve o dependente local aproximado
			for otherFile := range importsMap {
				otherBase := filepath.Base(otherFile)
				otherModuleName := strings.TrimSuffix(otherBase, filepath.Ext(otherBase))
				otherDir := filepath.Dir(otherFile)

				if strings.Contains(dep, otherModuleName) ||
					(otherDir != "." && strings.Contains(dep, otherDir)) ||
					strings.HasSuffix(dep, otherFile) {
					adj[file] = append(adj[file], otherFile)
				}
			}
		}
	}

	// DFS com 3 estados (0 = unvisited, 1 = visiting, 2 = visited)
	state := make(map[string]int)
	var cyclePath []string

	var dfs func(u string, path []string) bool
	dfs = func(u string, path []string) bool {
		state[u] = 1
		path = append(path, u)

		for _, v := range adj[u] {
			if state[v] == 1 {
				// Achar o início do ciclo no path
				startIdx := -1
				for i, node := range path {
					if node == v {
						startIdx = i
						break
					}
				}
				if startIdx != -1 {
					cyclePath = append([]string{}, path[startIdx:]...)
					cyclePath = append(cyclePath, v)
				}
				return true
			} else if state[v] == 0 {
				if dfs(v, path) {
					return true
				}
			}
		}

		state[u] = 2
		return false
	}

	for node := range adj {
		if state[node] == 0 {
			if dfs(node, nil) {
				issues = append(issues, ImportIssue{
					Type:        "circular",
					Files:       cyclePath,
					Description: "Ciclo de importação circular detectado: " + strings.Join(cyclePath, " -> "),
				})
				// Reinicia a detecção para outros nós se houver mais
				state = make(map[string]int)
			}
		}
	}

	res := ValidationResult{
		Valid:  len(issues) == 0,
		Issues: issues,
	}

	data, _ := json.MarshalIndent(res, "", "  ")
	return tools.Result{
		Success: true,
		Data:    string(data),
	}, nil
}
