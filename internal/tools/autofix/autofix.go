package autofix

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/crom/crom-agente/internal/llm"
	"github.com/crom/crom-agente/internal/tools"
)

//go:embed metadata.json
var metadataJSON []byte

var metadata tools.ToolMetadata

func init() {
	var err error
	metadata, err = tools.ParseMetadata(metadataJSON)
	if err != nil {
		panic("falha ao carregar metadados de autofix: " + err.Error())
	}
}

// AutofixTool automatiza o loop de correção de bugs em arquivos de código
type AutofixTool struct {
	workspaceRoot string
	jail          bool
	llmProvider   llm.Provider
}

// NewAutofixTool cria a ferramenta autofix
func NewAutofixTool(workspaceRoot string, jail bool, llmProvider llm.Provider) *AutofixTool {
	return &AutofixTool{
		workspaceRoot: workspaceRoot,
		jail:          jail,
		llmProvider:   llmProvider,
	}
}

func (t *AutofixTool) ID() string { return metadata.ID }

func (t *AutofixTool) Description() string { return metadata.Description }

func (t *AutofixTool) RequiresApproval() bool { return true }

func (t *AutofixTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Caminho do arquivo de código-fonte que contém o erro"
			},
			"error_message": {
				"type": "string",
				"description": "A mensagem de erro de compilação ou falha do teste obtida"
			}
		},
		"required": ["path", "error_message"]
	}`)
}

func (t *AutofixTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		Path         string `json:"path"`
		ErrorMessage string `json:"error_message"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Result{Success: false, Error: "argumentos inválidos"}, nil
	}

	targetFile, err := tools.ValidatePath(t.workspaceRoot, input.Path, t.jail)
	if err != nil {
		return tools.Result{Success: false, Error: err.Error()}, nil
	}

	// 1. Lê o arquivo original para fazer backup em memória e obter conteúdo atual
	origBytes, err := os.ReadFile(targetFile)
	if err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("falha ao ler arquivo original: %v", err)}, nil
	}

	if t.llmProvider == nil {
		return tools.Result{Success: false, Error: "LLMProvider não está disponível para esta ferramenta"}, nil
	}

	// 2. Solicita ao LLM a correção
	systemMsg := `Você é o Especialista de Autofix do CROM-Agente.
Sua tarefa é analisar o código-fonte fornecido e o log de erro, identificar a correção necessária e retornar o CONTEÚDO COMPLETO E CORRIGIDO do arquivo.
Não insira blocos de explicação ou formatação markdown (sem tags de código como ` + "```" + `go). Retorne APENAS o código puro corrigido.`

	userMsg := fmt.Sprintf("--- CÓDIGO ATUAL ---\n%s\n\n--- LOG DE ERRO ---\n%s", string(origBytes), input.ErrorMessage)

	resp, err := t.llmProvider.SendMessages(ctx, []llm.Message{
		{Role: "system", Content: systemMsg},
		{Role: "user", Content: userMsg},
	}, llm.RequestOptions{})

	if err != nil || resp == nil || resp.Message.Content == "" {
		return tools.Result{Success: false, Error: fmt.Sprintf("falha na geração de correção pela LLM: %v", err)}, nil
	}

	fixedCode := resp.Message.Content
	fixedCode = cleanMarkdownCodeBlock(fixedCode)

	// 3. Aplica a correção temporariamente
	err = os.WriteFile(targetFile, []byte(fixedCode), 0644)
	if err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("falha ao escrever arquivo corrigido: %v", err)}, nil
	}

	// 4. Executa verificação rápida (compilação/testes no diretório do arquivo)
	dir := filepath.Dir(targetFile)
	cmd := exec.CommandContext(ctx, "go", "test", "-tags", "headless", "./...")
	cmd.Dir = dir
	testOut, errTest := cmd.CombinedOutput()

	if errTest != nil {
		// Restaura backup original (Rollback)
		_ = os.WriteFile(targetFile, origBytes, 0644)
		return tools.Result{
			Success: false,
			Error:   fmt.Sprintf("a correção gerada falhou nos testes de validação: %v. Modificação revertida.", errTest),
			Data:    string(testOut),
		}, nil
	}

	return tools.Result{
		Success: true,
		Data:    fmt.Sprintf("✓ Autofix aplicado com sucesso! Testes passaram no pacote %s.\nLogs:\n%s", dir, string(testOut)),
	}, nil
}

func cleanMarkdownCodeBlock(code string) string {
	code = strings.TrimSpace(code)
	if strings.HasPrefix(code, "```") {
		lines := strings.Split(code, "\n")
		if len(lines) > 2 {
			return strings.Join(lines[1:len(lines)-1], "\n")
		}
	}
	return code
}
