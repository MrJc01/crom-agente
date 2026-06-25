package code_explainer

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
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
		panic("falha ao carregar metadados de code_explainer: " + err.Error())
	}
}

// CodeExplainerTool extrai a AST de um arquivo e gera uma descrição detalhada do funcionamento
type CodeExplainerTool struct {
	workspaceRoot string
	jail          bool
}

// NewCodeExplainerTool cria uma instância do explicador de código
func NewCodeExplainerTool(workspaceRoot string, jail bool) *CodeExplainerTool {
	return &CodeExplainerTool{
		workspaceRoot: workspaceRoot,
		jail:          jail,
	}
}

func (t *CodeExplainerTool) ID() string { return metadata.ID }

func (t *CodeExplainerTool) Description() string {
	return metadata.Description
}

func (t *CodeExplainerTool) RequiresApproval() bool { return false }

func (t *CodeExplainerTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Caminho do arquivo Go para analisar e explicar"
			},
			"function_name": {
				"type": "string",
				"description": "Nome de função específica para explicar (opcional)"
			},
			"detail_level": {
				"type": "string",
				"description": "Nível de detalhe: summary (resumo), detailed (completo)",
				"enum": ["summary", "detailed"]
			}
		},
		"required": ["path"]
	}`)
}

func (t *CodeExplainerTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		Path         string `json:"path"`
		FunctionName string `json:"function_name"`
		DetailLevel  string `json:"detail_level"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Result{Success: false, Error: "argumentos inválidos"}, nil
	}

	if input.DetailLevel == "" {
		input.DetailLevel = "detailed"
	}

	targetFile, err := tools.ValidatePath(t.workspaceRoot, input.Path, t.jail)
	if err != nil {
		return tools.Result{Success: false, Error: err.Error()}, nil
	}

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, targetFile, nil, parser.ParseComments)
	if err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("erro ao parsear: %s", err.Error())}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# 📖 Análise de Código: `%s`\n\n", input.Path))

	// Informações do pacote
	sb.WriteString(fmt.Sprintf("**Pacote:** `%s`\n\n", node.Name.Name))

	// Imports
	if len(node.Imports) > 0 {
		sb.WriteString("## 📦 Dependências\n\n")
		for _, imp := range node.Imports {
			importPath := imp.Path.Value
			category := categorizeImport(importPath)
			alias := ""
			if imp.Name != nil {
				alias = fmt.Sprintf(" (alias: `%s`)", imp.Name.Name)
			}
			sb.WriteString(fmt.Sprintf("- %s `%s`%s\n", category, importPath, alias))
		}
		sb.WriteString("\n")
	}

	// Tipos e Structs
	var types []string
	var interfaces []string
	var constants []string

	for _, decl := range node.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}

		switch genDecl.Tok {
		case token.TYPE:
			for _, spec := range genDecl.Specs {
				typeSpec := spec.(*ast.TypeSpec)
				if _, ok := typeSpec.Type.(*ast.StructType); ok {
					types = append(types, typeSpec.Name.Name)
				}
				if _, ok := typeSpec.Type.(*ast.InterfaceType); ok {
					interfaces = append(interfaces, typeSpec.Name.Name)
				}
			}
		case token.CONST:
			for _, spec := range genDecl.Specs {
				valSpec, ok := spec.(*ast.ValueSpec)
				if ok {
					for _, name := range valSpec.Names {
						constants = append(constants, name.Name)
					}
				}
			}
		}
	}

	if len(types) > 0 {
		sb.WriteString("## 🏗️ Tipos (Structs)\n\n")
		for _, typeName := range types {
			sb.WriteString(fmt.Sprintf("- `%s`", typeName))
			if input.DetailLevel == "detailed" {
				desc := describeType(node, typeName)
				if desc != "" {
					sb.WriteString(fmt.Sprintf(": %s", desc))
				}
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	if len(interfaces) > 0 {
		sb.WriteString("## 🔌 Interfaces\n\n")
		for _, ifName := range interfaces {
			sb.WriteString(fmt.Sprintf("- `%s`\n", ifName))
		}
		sb.WriteString("\n")
	}

	if len(constants) > 0 && input.DetailLevel == "detailed" {
		sb.WriteString("## 🔢 Constantes\n\n")
		for _, c := range constants {
			sb.WriteString(fmt.Sprintf("- `%s`\n", c))
		}
		sb.WriteString("\n")
	}

	// Funções
	sb.WriteString("## ⚙️ Funções\n\n")

	funcCount := 0
	for _, decl := range node.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		// Filtrar por nome se especificado
		if input.FunctionName != "" && funcDecl.Name.Name != input.FunctionName {
			continue
		}

		funcCount++
		explainFunction(&sb, fset, funcDecl, input.DetailLevel)
	}

	if funcCount == 0 {
		if input.FunctionName != "" {
			return tools.Result{Success: false, Error: fmt.Sprintf("função '%s' não encontrada", input.FunctionName)}, nil
		}
		sb.WriteString("_Nenhuma função encontrada neste arquivo._\n")
	}

	// Métricas
	if input.DetailLevel == "detailed" {
		sb.WriteString("## 📊 Métricas\n\n")
		sb.WriteString(fmt.Sprintf("| Métrica | Valor |\n"))
		sb.WriteString(fmt.Sprintf("|---------|-------|\n"))
		sb.WriteString(fmt.Sprintf("| Structs | %d |\n", len(types)))
		sb.WriteString(fmt.Sprintf("| Interfaces | %d |\n", len(interfaces)))
		sb.WriteString(fmt.Sprintf("| Funções | %d |\n", funcCount))
		sb.WriteString(fmt.Sprintf("| Imports | %d |\n", len(node.Imports)))
		sb.WriteString(fmt.Sprintf("| Constantes | %d |\n", len(constants)))
	}

	return tools.Result{Success: true, Data: sb.String()}, nil
}

func explainFunction(sb *strings.Builder, fset *token.FileSet, funcDecl *ast.FuncDecl, level string) {
	// Nome e assinatura
	name := funcDecl.Name.Name
	visibility := "🔒 privada"
	if funcDecl.Name.IsExported() {
		visibility = "🌐 pública"
	}

	receiver := ""
	if funcDecl.Recv != nil && len(funcDecl.Recv.List) > 0 {
		receiver = fmt.Sprintf(" (método de `%s`)", tools.ExprToString(funcDecl.Recv.List[0].Type))
	}

	sb.WriteString(fmt.Sprintf("### `%s`%s — %s\n\n", name, receiver, visibility))

	// Doc existente
	if funcDecl.Doc != nil {
		sb.WriteString(fmt.Sprintf("📝 **Doc:** %s\n\n", strings.TrimSpace(funcDecl.Doc.Text())))
	}

	// Parâmetros
	if funcDecl.Type.Params != nil && len(funcDecl.Type.Params.List) > 0 {
		sb.WriteString("**Parâmetros:**\n")
		for _, p := range funcDecl.Type.Params.List {
			typeName := tools.ExprToString(p.Type)
			for _, pName := range p.Names {
				sb.WriteString(fmt.Sprintf("- `%s` (`%s`)\n", pName.Name, typeName))
			}
			if len(p.Names) == 0 {
				sb.WriteString(fmt.Sprintf("- `%s`\n", typeName))
			}
		}
		sb.WriteString("\n")
	}

	// Retornos
	if funcDecl.Type.Results != nil && len(funcDecl.Type.Results.List) > 0 {
		sb.WriteString("**Retornos:**\n")
		for _, r := range funcDecl.Type.Results.List {
			sb.WriteString(fmt.Sprintf("- `%s`\n", tools.ExprToString(r.Type)))
		}
		sb.WriteString("\n")
	}

	// Análise detalhada do corpo
	if level == "detailed" && funcDecl.Body != nil {
		sb.WriteString("**Comportamento:**\n")
		analyzeBody(sb, funcDecl.Body)
		sb.WriteString("\n")
	}

	sb.WriteString("---\n\n")
}

func analyzeBody(sb *strings.Builder, body *ast.BlockStmt) {
	if body == nil {
		return
	}

	errorChecks := 0
	goroutines := 0
	defers := 0
	loops := 0
	switches := 0

	ast.Inspect(body, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.IfStmt:
			errorChecks++
		case *ast.GoStmt:
			goroutines++
		case *ast.DeferStmt:
			defers++
		case *ast.ForStmt, *ast.RangeStmt:
			loops++
		case *ast.SwitchStmt, *ast.TypeSwitchStmt:
			switches++
		}
		return true
	})

	if errorChecks > 0 {
		sb.WriteString(fmt.Sprintf("- 🛡️ %d verificações condicionais (if/error checks)\n", errorChecks))
	}
	if goroutines > 0 {
		sb.WriteString(fmt.Sprintf("- 🚀 %d goroutines lançadas (concorrência)\n", goroutines))
	}
	if defers > 0 {
		sb.WriteString(fmt.Sprintf("- ⏳ %d defer statements (limpeza/finalização)\n", defers))
	}
	if loops > 0 {
		sb.WriteString(fmt.Sprintf("- 🔁 %d loops (for/range)\n", loops))
	}
	if switches > 0 {
		sb.WriteString(fmt.Sprintf("- 🔀 %d switches (ramificação múltipla)\n", switches))
	}

	totalComplexity := errorChecks + goroutines + defers + loops + switches
	if totalComplexity == 0 {
		sb.WriteString("- ✅ Função simples e linear\n")
	} else if totalComplexity > 10 {
		sb.WriteString("- ⚠️ **Função de alta complexidade** — considere refatoração\n")
	}
}

func categorizeImport(importPath string) string {
	importPath = strings.Trim(importPath, "\"")
	if !strings.Contains(importPath, ".") {
		return "📦 stdlib"
	}
	if strings.Contains(importPath, "github.com") || strings.Contains(importPath, "golang.org") {
		return "🔗 externo"
	}
	return "📁 local"
}

func describeType(node *ast.File, typeName string) string {
	for _, decl := range node.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}

		for _, spec := range genDecl.Specs {
			typeSpec := spec.(*ast.TypeSpec)
			if typeSpec.Name.Name != typeName {
				continue
			}

			st, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}

			fieldCount := 0
			if st.Fields != nil {
				fieldCount = len(st.Fields.List)
			}

			desc := fmt.Sprintf("%d campos", fieldCount)
			if genDecl.Doc != nil {
				desc += " — " + strings.TrimSpace(genDecl.Doc.Text())
			}
			return desc
		}
	}
	return ""
}
