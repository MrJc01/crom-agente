package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// TreeTool gera a árvore de arquivos e diretórios do workspace
type TreeTool struct {
	workspaceRoot string
	jail          bool
}

// NewTreeTool cria a ferramenta tree
func NewTreeTool(workspaceRoot string, jail bool) *TreeTool {
	return &TreeTool{
		workspaceRoot: workspaceRoot,
		jail:          jail,
	}
}

// ID retorna o identificador da ferramenta
func (t *TreeTool) ID() string {
	return "tree"
}

// Description retorna a descrição da ferramenta
func (t *TreeTool) Description() string {
	return "Mapeia a árvore de arquivos e subdiretórios a partir de uma pasta do workspace, ocultando pastas de controle (ex: .git, node_modules)."
}

// ParametersSchema define os parâmetros aceitos via JSON
func (t *TreeTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Subdiretório para início da varredura (opcional, padrão raiz do workspace)"
			},
			"max_depth": {
				"type": "integer",
				"description": "Profundidade limite para recursão (opcional, padrão 3, máximo 6)"
			}
		}
	}`)
}

// RequiresApproval indica que esta ferramenta pode rodar sem confirmação
func (t *TreeTool) RequiresApproval() bool {
	return false
}

// Execute gera a árvore de diretórios
func (t *TreeTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var input struct {
		Path     string `json:"path"`
		MaxDepth int    `json:"max_depth"`
	}

	// Parsing dos argumentos (opcionais)
	if len(args) > 0 && string(args) != "null" && string(args) != "{}" {
		_ = json.Unmarshal(args, &input)
	}

	if input.MaxDepth <= 0 {
		input.MaxDepth = 3 // Padrão
	}
	if input.MaxDepth > 6 {
		input.MaxDepth = 6 // Limite rígido para evitar estourar tokens
	}

	startDir, err := ValidatePath(t.workspaceRoot, input.Path, t.jail)
	if err != nil {
		return Result{Success: false, Error: err.Error()}, nil
	}

	startDirAbs, err := filepath.Abs(startDir)
	if err != nil {
		return Result{Success: false, Error: err.Error()}, nil
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf(". (%s)\n", filepath.Base(startDirAbs)))

	// Lógica recursiva controlando o recuo e depth
	err = t.walkTree(startDirAbs, startDirAbs, 1, input.MaxDepth, &builder)
	if err != nil {
		return Result{Success: false, Error: err.Error()}, nil
	}

	return Result{Success: true, Data: builder.String()}, nil
}

func (t *TreeTool) walkTree(rootDir, currentDir string, currentDepth, maxDepth int, builder *strings.Builder) error {
	if currentDepth > maxDepth {
		return nil
	}

	files, err := filepath.Glob(filepath.Join(currentDir, "*"))
	if err != nil {
		return err
	}

	// Tratar arquivos ocultos manualmente ou via varredura se necessário
	// Mas o filepath.Glob("*") normalmente ignora ocultos em Unix a menos que especificado.
	// Vamos forçar leitura de diretório para incluir tudo exceto os ignorados explícitos.
	entries, err := filepath.Glob(filepath.Join(currentDir, ".*"))
	if err == nil {
		files = append(files, entries...)
	}

	for _, file := range files {
		base := filepath.Base(file)

		// Pular marcadores de referência e pastas ignoradas
		if base == "." || base == ".." {
			continue
		}

		// Filtros de exclusão padrão
		if base == ".git" || base == "node_modules" || base == "build" || base == "dist" || base == "bin" || base == ".crom" || base == "tmp" || base == "obj" {
			continue
		}

		info, err := os.Stat(file)
		if err != nil {
			continue
		}

		indent := strings.Repeat("  ", currentDepth)
		if info.IsDir() {
			builder.WriteString(fmt.Sprintf("%s📁 %s/\n", indent, base))
			err = t.walkTree(rootDir, file, currentDepth+1, maxDepth, builder)
			if err != nil {
				return err
			}
		} else {
			builder.WriteString(fmt.Sprintf("%s📄 %s\n", indent, base))
		}
	}

	return nil
}
