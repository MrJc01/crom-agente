package refactor_auditor

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

	"github.com/crom/crom-agente/internal/tools"
)

//go:embed metadata.json
var metadataJSON []byte

var metadata tools.ToolMetadata

func init() {
	var err error
	metadata, err = tools.ParseMetadata(metadataJSON)
	if err != nil {
		panic("falha ao carregar metadados de refactor_auditor: " + err.Error())
	}
}

// RefactorAuditorTool audita modificações em busca de refatorações ineficientes/ruins
type RefactorAuditorTool struct {
	workspaceRoot string
	jail          bool
}

// NewRefactorAuditorTool cria a ferramenta de auditoria
func NewRefactorAuditorTool(workspaceRoot string, jail bool) *RefactorAuditorTool {
	return &RefactorAuditorTool{
		workspaceRoot: workspaceRoot,
		jail:          jail,
	}
}

func (t *RefactorAuditorTool) ID() string { return metadata.ID }

func (t *RefactorAuditorTool) Description() string { return metadata.Description }

func (t *RefactorAuditorTool) RequiresApproval() bool { return false }

func (t *RefactorAuditorTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Caminho do arquivo ou diretório modificado a ser auditado"
			}
		},
		"required": ["path"]
	}`)
}

func (t *RefactorAuditorTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Result{Success: false, Error: "argumentos inválidos"}, nil
	}

	targetPath, err := tools.ValidatePath(t.workspaceRoot, input.Path, t.jail)
	if err != nil {
		return tools.Result{Success: false, Error: err.Error()}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== 🔍 RELATÓRIO DE AUDITORIA DE REFATORAÇÃO: %s ===\n\n", input.Path))

	// 1. Verifica se há arquivos mortos ou temporários redundantes
	files, _ := os.ReadDir(t.workspaceRoot)
	deadFiles := 0
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".tmp") || strings.HasSuffix(f.Name(), ".bak") || strings.HasPrefix(f.Name(), "temp_") {
			sb.WriteString(fmt.Sprintf("⚠️  Arquivo temporário/morto encontrado no workspace: `%s`\n", f.Name()))
			deadFiles++
		}
	}
	if deadFiles == 0 {
		sb.WriteString("✓ Nenhum arquivo morto/temporário encontrado no workspace.\n")
	}

	// 2. Análise estática AST no arquivo alvo se for Go (Tamanho de funções e complexidade)
	info, err := os.Stat(targetPath)
	if err == nil && !info.IsDir() && filepath.Ext(targetPath) == ".go" {
		fset := token.NewFileSet()
		node, errParse := parser.ParseFile(fset, targetPath, nil, parser.ParseComments)
		if errParse == nil {
			sb.WriteString("\n--- 🏗️ Análise de Funções e Métodos ---\n")
			hasIssues := false
			for _, decl := range node.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}

				// Mede tamanho da função em linhas
				startLine := fset.Position(fn.Pos()).Line
				endLine := fset.Position(fn.End()).Line
				lines := endLine - startLine

				if lines > 80 {
					sb.WriteString(fmt.Sprintf("⚠️  Função muito longa: `%s` tem %d linhas (sugere-se dividir a função).\n", fn.Name.Name, lines))
					hasIssues = true
				}
			}
			if !hasIssues {
				sb.WriteString("✓ Todas as funções estão com tamanho adequado e bem estruturadas.\n")
			}
		}
	}

	// 3. Verifica duplicidades de código via git diff se disponível
	cmd := exec.CommandContext(ctx, "git", "diff", "HEAD")
	cmd.Dir = t.workspaceRoot
	diffOut, errDiff := cmd.CombinedOutput()
	if errDiff == nil && len(diffOut) > 0 {
		sb.WriteString("\n--- 📈 Análise de Diff / Duplicações ---\n")
		lines := strings.Split(string(diffOut), "\n")
		addedLines := make(map[string]int)
		duplicates := 0
		for _, line := range lines {
			if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
				trimmed := strings.TrimSpace(line[1:])
				if len(trimmed) > 15 {
					addedLines[trimmed]++
					if addedLines[trimmed] > 1 {
						sb.WriteString(fmt.Sprintf("⚠️  Linha redundante/duplicada adicionada: `%s`\n", trimmed))
						duplicates++
						if duplicates >= 3 {
							break
						}
					}
				}
			}
		}
		if duplicates == 0 {
			sb.WriteString("✓ Nenhuma linha duplicada óbvia detectada no diff atual.\n")
		}
	}

	sb.WriteString("\n=============================================\n")

	return tools.Result{
		Success: true,
		Data:    sb.String(),
	}, nil
}
