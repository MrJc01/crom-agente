package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
)

// DocGeneratorTool gera documentação estruturada (GoDoc, JSDoc) para funções sem documentação
type DocGeneratorTool struct {
	workspaceRoot string
	jail          bool
}

// NewDocGeneratorTool cria uma instância do gerador de documentação
func NewDocGeneratorTool(workspaceRoot string, jail bool) *DocGeneratorTool {
	return &DocGeneratorTool{
		workspaceRoot: workspaceRoot,
		jail:          jail,
	}
}

func (t *DocGeneratorTool) ID() string             { return "doc_generator" }
func (t *DocGeneratorTool) Description() string     { return "Analisa funções sem documentação em um arquivo Go e gera comentários GoDoc estruturados baseados no corpo da função." }
func (t *DocGeneratorTool) RequiresApproval() bool  { return true }

func (t *DocGeneratorTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Caminho do arquivo Go para gerar documentação"
			},
			"function_name": {
				"type": "string",
				"description": "Nome da função específica (opcional, se vazio gera para todas sem doc)"
			},
			"apply": {
				"type": "boolean",
				"description": "Se true, aplica os comentários no arquivo. Se false, retorna preview."
			}
		},
		"required": ["path"]
	}`)
}

// UndocumentedFunc representa uma função sem documentação GoDoc
type UndocumentedFunc struct {
	Name       string
	Receiver   string // vazio se for função livre
	Params     []string
	Returns    []string
	LineNumber int
	Body       string
}

func (t *DocGeneratorTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var input struct {
		Path         string `json:"path"`
		FunctionName string `json:"function_name"`
		Apply        bool   `json:"apply"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return Result{Success: false, Error: "argumentos inválidos"}, nil
	}

	targetFile, err := ValidatePath(t.workspaceRoot, input.Path, t.jail)
	if err != nil {
		return Result{Success: false, Error: err.Error()}, nil
	}

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, targetFile, nil, parser.ParseComments)
	if err != nil {
		return Result{Success: false, Error: fmt.Sprintf("erro ao parsear arquivo: %s", err.Error())}, nil
	}

	// Ler conteúdo original do arquivo para inserção de comentários
	data, err := os.ReadFile(targetFile)
	if err != nil {
		return Result{Success: false, Error: fmt.Sprintf("erro ao ler arquivo: %s", err.Error())}, nil
	}
	lines := strings.Split(string(data), "\n")

	// Encontrar funções sem documentação
	var undocumented []UndocumentedFunc

	for _, decl := range node.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		// Verificar se já possui documentação
		if funcDecl.Doc != nil && len(funcDecl.Doc.List) > 0 {
			continue
		}

		// Filtrar por nome se especificado
		if input.FunctionName != "" && funcDecl.Name.Name != input.FunctionName {
			continue
		}

		// Pular funções não exportadas pequenas (helpers privados)
		if !funcDecl.Name.IsExported() && input.FunctionName == "" {
			continue
		}

		uf := UndocumentedFunc{
			Name:       funcDecl.Name.Name,
			LineNumber: fset.Position(funcDecl.Pos()).Line,
		}

		// Extrair receiver
		if funcDecl.Recv != nil && len(funcDecl.Recv.List) > 0 {
			uf.Receiver = exprToString(funcDecl.Recv.List[0].Type)
		}

		// Extrair parâmetros
		if funcDecl.Type.Params != nil {
			for _, p := range funcDecl.Type.Params.List {
				typeName := exprToString(p.Type)
				for _, name := range p.Names {
					uf.Params = append(uf.Params, fmt.Sprintf("%s %s", name.Name, typeName))
				}
				if len(p.Names) == 0 {
					uf.Params = append(uf.Params, typeName)
				}
			}
		}

		// Extrair retornos
		if funcDecl.Type.Results != nil {
			for _, r := range funcDecl.Type.Results.List {
				uf.Returns = append(uf.Returns, exprToString(r.Type))
			}
		}

		undocumented = append(undocumented, uf)
	}

	if len(undocumented) == 0 {
		return Result{Success: true, Data: "Todas as funções exportadas já possuem documentação GoDoc."}, nil
	}

	// Gerar comentários GoDoc para cada função
	type docEntry struct {
		LineNumber int
		Comment    string
	}
	var docs []docEntry

	for _, uf := range undocumented {
		comment := generateGoDoc(uf)
		docs = append(docs, docEntry{LineNumber: uf.LineNumber, Comment: comment})
	}

	if !input.Apply {
		// Preview mode: retornar os comentários gerados
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("📝 %d funções sem documentação encontradas:\n\n", len(docs)))
		for _, d := range docs {
			sb.WriteString(fmt.Sprintf("--- Linha %d ---\n%s\n\n", d.LineNumber, d.Comment))
		}
		return Result{Success: true, Data: sb.String()}, nil
	}

	// Apply mode: inserir os comentários no arquivo
	// Processar de baixo para cima para não invalidar números de linha
	for i := len(docs) - 1; i >= 0; i-- {
		d := docs[i]
		lineIdx := d.LineNumber - 1 // 0-indexed
		if lineIdx < 0 || lineIdx >= len(lines) {
			continue
		}

		// Inserir o comentário antes da linha da função
		commentLines := strings.Split(d.Comment, "\n")
		newLines := make([]string, 0, len(lines)+len(commentLines))
		newLines = append(newLines, lines[:lineIdx]...)
		newLines = append(newLines, commentLines...)
		newLines = append(newLines, lines[lineIdx:]...)
		lines = newLines
	}

	newContent := strings.Join(lines, "\n")
	if err := os.WriteFile(targetFile, []byte(newContent), 0644); err != nil {
		return Result{Success: false, Error: fmt.Sprintf("erro ao gravar arquivo: %s", err.Error())}, nil
	}

	return Result{Success: true, Data: fmt.Sprintf("✅ Documentação GoDoc inserida para %d funções em %s", len(docs), input.Path)}, nil
}

// generateGoDoc gera um comentário GoDoc baseado na assinatura e corpo da função
func generateGoDoc(uf UndocumentedFunc) string {
	var sb strings.Builder

	// Construir descrição baseada no nome e receiver
	description := describeFunction(uf)
	sb.WriteString(fmt.Sprintf("// %s %s", uf.Name, description))

	// Documentar parâmetros se houver muitos
	if len(uf.Params) > 2 {
		sb.WriteString("\n//")
		sb.WriteString("\n// Parâmetros:")
		for _, p := range uf.Params {
			parts := strings.SplitN(p, " ", 2)
			if len(parts) == 2 {
				sb.WriteString(fmt.Sprintf("\n//   - %s: %s", parts[0], parts[1]))
			}
		}
	}

	// Documentar retornos se houver
	if len(uf.Returns) > 0 {
		sb.WriteString("\n//")
		if len(uf.Returns) == 1 {
			sb.WriteString(fmt.Sprintf("\n// Retorna %s.", describeReturnType(uf.Returns[0])))
		} else {
			sb.WriteString("\n// Retorna:")
			for _, r := range uf.Returns {
				sb.WriteString(fmt.Sprintf("\n//   - %s", describeReturnType(r)))
			}
		}
	}

	return sb.String()
}

// describeFunction gera uma descrição textual baseada no nome da função
func describeFunction(uf UndocumentedFunc) string {
	name := uf.Name

	// Detectar padrões comuns de nomeação Go
	if strings.HasPrefix(name, "New") {
		typeName := strings.TrimPrefix(name, "New")
		if uf.Receiver == "" {
			return fmt.Sprintf("cria uma nova instância de %s.", typeName)
		}
	}

	if strings.HasPrefix(name, "Get") {
		field := strings.TrimPrefix(name, "Get")
		return fmt.Sprintf("retorna o valor de %s.", field)
	}

	if strings.HasPrefix(name, "Set") {
		field := strings.TrimPrefix(name, "Set")
		return fmt.Sprintf("define o valor de %s.", field)
	}

	if strings.HasPrefix(name, "Is") || strings.HasPrefix(name, "Has") || strings.HasPrefix(name, "Can") {
		return fmt.Sprintf("verifica a condição de %s.", name)
	}

	if strings.HasPrefix(name, "Handle") {
		return fmt.Sprintf("processa e responde ao evento de %s.", strings.TrimPrefix(name, "Handle"))
	}

	if strings.HasPrefix(name, "Parse") {
		return fmt.Sprintf("faz o parsing de %s.", strings.TrimPrefix(name, "Parse"))
	}

	if strings.HasPrefix(name, "Load") {
		return fmt.Sprintf("carrega %s.", strings.TrimPrefix(name, "Load"))
	}

	if strings.HasPrefix(name, "Save") {
		return fmt.Sprintf("persiste %s.", strings.TrimPrefix(name, "Save"))
	}

	if strings.HasPrefix(name, "Delete") || strings.HasPrefix(name, "Remove") {
		return fmt.Sprintf("remove %s.", strings.TrimPrefix(strings.TrimPrefix(name, "Delete"), "Remove"))
	}

	if strings.HasPrefix(name, "Validate") {
		return fmt.Sprintf("valida %s.", strings.TrimPrefix(name, "Validate"))
	}

	if uf.Receiver != "" {
		return fmt.Sprintf("executa a operação de %s no receptor %s.", name, uf.Receiver)
	}

	return fmt.Sprintf("executa a operação de %s.", name)
}

// describeReturnType gera descrição amigável para um tipo de retorno
func describeReturnType(t string) string {
	if t == "error" {
		return "um error se a operação falhar"
	}
	if t == "bool" {
		return "true em caso de sucesso"
	}
	if t == "string" {
		return "uma string com o resultado"
	}
	if strings.HasPrefix(t, "[]") {
		return fmt.Sprintf("uma lista de %s", t[2:])
	}
	if strings.HasPrefix(t, "*") {
		return fmt.Sprintf("um ponteiro para %s (ou nil)", t[1:])
	}
	return t
}
