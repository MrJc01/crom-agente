package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
)

// ComplexityReducerTool calcula a complexidade ciclomática de funções e sinaliza as que excedem o limiar
type ComplexityReducerTool struct {
	workspaceRoot string
	jail          bool
}

// NewComplexityReducerTool cria uma instância do redutor de complexidade
func NewComplexityReducerTool(workspaceRoot string, jail bool) *ComplexityReducerTool {
	return &ComplexityReducerTool{
		workspaceRoot: workspaceRoot,
		jail:          jail,
	}
}

func (t *ComplexityReducerTool) ID() string             { return "complexity_reducer" }
func (t *ComplexityReducerTool) Description() string     { return "Calcula a complexidade ciclomática de funções Go e sinaliza aquelas que excedem o limiar configurável (default: 15)." }
func (t *ComplexityReducerTool) RequiresApproval() bool  { return false }

func (t *ComplexityReducerTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Caminho do arquivo ou diretório Go para analisar"
			},
			"threshold": {
				"type": "integer",
				"description": "Limiar de complexidade ciclomática (default: 15)"
			},
			"show_all": {
				"type": "boolean",
				"description": "Se true, mostra todas as funções, não apenas as que excedem o limiar"
			}
		},
		"required": ["path"]
	}`)
}

// FuncComplexity armazena a complexidade de uma função
type FuncComplexity struct {
	File       string
	Name       string
	Receiver   string
	LineNumber int
	Complexity int
	Branches   ComplexityBreakdown
}

// ComplexityBreakdown detalha os tipos de ramificação encontrados
type ComplexityBreakdown struct {
	IfStmts     int
	ForLoops    int
	RangeLoops  int
	SwitchCases int
	SelectCases int
	BoolOps     int // && e || em condições
	GoStmts     int
}

func (t *ComplexityReducerTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var input struct {
		Path      string `json:"path"`
		Threshold int    `json:"threshold"`
		ShowAll   bool   `json:"show_all"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return Result{Success: false, Error: "argumentos inválidos"}, nil
	}

	if input.Threshold <= 0 {
		input.Threshold = 15
	}

	targetPath, err := ValidatePath(t.workspaceRoot, input.Path, t.jail)
	if err != nil {
		return Result{Success: false, Error: err.Error()}, nil
	}

	var results []FuncComplexity

	// Parsear arquivo(s)
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, targetPath, nil, parser.ParseComments)
	if err != nil {
		return Result{Success: false, Error: fmt.Sprintf("erro ao parsear: %s", err.Error())}, nil
	}

	for _, decl := range node.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		fc := calculateComplexity(fset, funcDecl, input.Path)
		results = append(results, fc)
	}

	// Ordenar por complexidade decrescente
	sort.Slice(results, func(i, j int) bool {
		return results[i].Complexity > results[j].Complexity
	})

	// Gerar relatório
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# 📊 Relatório de Complexidade Ciclomática\n\n"))
	sb.WriteString(fmt.Sprintf("**Arquivo:** `%s`\n", input.Path))
	sb.WriteString(fmt.Sprintf("**Limiar:** %d\n\n", input.Threshold))

	// Estatísticas gerais
	totalFuncs := len(results)
	exceeding := 0
	for _, r := range results {
		if r.Complexity > input.Threshold {
			exceeding++
		}
	}

	sb.WriteString(fmt.Sprintf("| Métrica | Valor |\n"))
	sb.WriteString(fmt.Sprintf("|---------|-------|\n"))
	sb.WriteString(fmt.Sprintf("| Total de funções | %d |\n", totalFuncs))
	sb.WriteString(fmt.Sprintf("| Excedem limiar | %d |\n", exceeding))

	if totalFuncs > 0 {
		maxComplexity := results[0].Complexity
		avgComplexity := 0
		for _, r := range results {
			avgComplexity += r.Complexity
		}
		avgComplexity /= totalFuncs
		sb.WriteString(fmt.Sprintf("| Complexidade máxima | %d |\n", maxComplexity))
		sb.WriteString(fmt.Sprintf("| Complexidade média | %d |\n", avgComplexity))
	}
	sb.WriteString("\n")

	// Funções com alta complexidade
	if exceeding > 0 {
		sb.WriteString("## ⚠️ Funções com Alta Complexidade\n\n")
		for _, r := range results {
			if r.Complexity <= input.Threshold {
				continue
			}

			funcName := r.Name
			if r.Receiver != "" {
				funcName = fmt.Sprintf("(%s).%s", r.Receiver, r.Name)
			}

			sb.WriteString(fmt.Sprintf("### 🔴 `%s` — Complexidade: **%d** (linha %d)\n\n", funcName, r.Complexity, r.LineNumber))
			sb.WriteString("**Breakdown:**\n")
			writeBreakdown(&sb, r.Branches)
			sb.WriteString("\n**Sugestões de refatoração:**\n")
			writeSuggestions(&sb, r)
			sb.WriteString("\n---\n\n")
		}
	}

	// Todas as funções (se solicitado)
	if input.ShowAll {
		sb.WriteString("## 📋 Todas as Funções\n\n")
		sb.WriteString("| Função | Complexidade | Status |\n")
		sb.WriteString("|--------|-------------|--------|\n")

		for _, r := range results {
			funcName := r.Name
			if r.Receiver != "" {
				funcName = fmt.Sprintf("(%s).%s", r.Receiver, r.Name)
			}

			status := "✅"
			if r.Complexity > input.Threshold {
				status = "🔴"
			} else if r.Complexity > input.Threshold/2 {
				status = "🟡"
			}

			sb.WriteString(fmt.Sprintf("| `%s` | %d | %s |\n", funcName, r.Complexity, status))
		}
		sb.WriteString("\n")
	}

	return Result{Success: true, Data: sb.String()}, nil
}

// calculateComplexity calcula a complexidade ciclomática de uma função Go
func calculateComplexity(fset *token.FileSet, funcDecl *ast.FuncDecl, file string) FuncComplexity {
	fc := FuncComplexity{
		File:       file,
		Name:       funcDecl.Name.Name,
		LineNumber: fset.Position(funcDecl.Pos()).Line,
		Complexity: 1, // Base complexity é 1
	}

	if funcDecl.Recv != nil && len(funcDecl.Recv.List) > 0 {
		fc.Receiver = exprToString(funcDecl.Recv.List[0].Type)
	}

	if funcDecl.Body == nil {
		return fc
	}

	ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.IfStmt:
			fc.Complexity++
			fc.Branches.IfStmts++
			// Contar operadores booleanos na condição
			fc.Branches.BoolOps += countBoolOps(node.Cond)
			fc.Complexity += countBoolOps(node.Cond)

		case *ast.ForStmt:
			fc.Complexity++
			fc.Branches.ForLoops++

		case *ast.RangeStmt:
			fc.Complexity++
			fc.Branches.RangeLoops++

		case *ast.CaseClause:
			if node.List != nil { // Ignora caso default
				fc.Complexity++
				fc.Branches.SwitchCases++
			}

		case *ast.CommClause:
			if node.Comm != nil { // Ignora caso default
				fc.Complexity++
				fc.Branches.SelectCases++
			}

		case *ast.GoStmt:
			fc.Branches.GoStmts++
		}
		return true
	})

	return fc
}

// countBoolOps conta operadores && e || em uma expressão condicional
func countBoolOps(expr ast.Expr) int {
	count := 0
	ast.Inspect(expr, func(n ast.Node) bool {
		if binExpr, ok := n.(*ast.BinaryExpr); ok {
			if binExpr.Op.String() == "&&" || binExpr.Op.String() == "||" {
				count++
			}
		}
		return true
	})
	return count
}

func writeBreakdown(sb *strings.Builder, b ComplexityBreakdown) {
	if b.IfStmts > 0 {
		sb.WriteString(fmt.Sprintf("- `if` statements: %d\n", b.IfStmts))
	}
	if b.ForLoops > 0 {
		sb.WriteString(fmt.Sprintf("- `for` loops: %d\n", b.ForLoops))
	}
	if b.RangeLoops > 0 {
		sb.WriteString(fmt.Sprintf("- `range` loops: %d\n", b.RangeLoops))
	}
	if b.SwitchCases > 0 {
		sb.WriteString(fmt.Sprintf("- `switch/case` branches: %d\n", b.SwitchCases))
	}
	if b.SelectCases > 0 {
		sb.WriteString(fmt.Sprintf("- `select/case` branches: %d\n", b.SelectCases))
	}
	if b.BoolOps > 0 {
		sb.WriteString(fmt.Sprintf("- Boolean operators (`&&`/`||`): %d\n", b.BoolOps))
	}
	if b.GoStmts > 0 {
		sb.WriteString(fmt.Sprintf("- Goroutines (`go`): %d\n", b.GoStmts))
	}
}

func writeSuggestions(sb *strings.Builder, fc FuncComplexity) {
	if fc.Branches.IfStmts > 5 {
		sb.WriteString("- 🔧 **Extrair condicionais**: Muitas verificações if/else. Considere usar early returns, tabelas de dispatch ou padrão Strategy.\n")
	}

	if fc.Branches.SwitchCases > 5 {
		sb.WriteString("- 🔧 **Switch extenso**: Considere substituir por um mapa de funções (dispatch table) ou polimorfismo.\n")
	}

	if fc.Branches.ForLoops+fc.Branches.RangeLoops > 3 {
		sb.WriteString("- 🔧 **Múltiplos loops**: Considere extrair loops internos em funções auxiliares nomeadas.\n")
	}

	if fc.Branches.BoolOps > 3 {
		sb.WriteString("- 🔧 **Condições complexas**: Considere extrair condições booleanas em funções predicadas bem nomeadas.\n")
	}

	if fc.Branches.GoStmts > 2 {
		sb.WriteString("- 🔧 **Muitas goroutines**: Considere usar worker pool ou pipeline pattern para controlar concorrência.\n")
	}

	if fc.Complexity > 25 {
		sb.WriteString("- 🚨 **Refatoração urgente**: Complexidade extrema. Quebre esta função em sub-funções menores e mais testáveis.\n")
	}
}
