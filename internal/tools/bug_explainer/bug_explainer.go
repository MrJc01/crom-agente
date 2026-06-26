package bug_explainer

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
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
		panic("falha ao carregar metadados de bug_explainer: " + err.Error())
	}
}

// BugExplainerTool sintetiza falhas de teste e erros usando LLM
type BugExplainerTool struct {
	workspaceRoot string
	llmProvider   llm.Provider
}

// NewBugExplainerTool cria a ferramenta
func NewBugExplainerTool(workspaceRoot string, llmProvider llm.Provider) *BugExplainerTool {
	return &BugExplainerTool{
		workspaceRoot: workspaceRoot,
		llmProvider:   llmProvider,
	}
}

func (t *BugExplainerTool) ID() string { return metadata.ID }

func (t *BugExplainerTool) Description() string { return metadata.Description }

func (t *BugExplainerTool) RequiresApproval() bool { return false }

func (t *BugExplainerTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"error_log": {
				"type": "string",
				"description": "Texto completo dos logs de erro ou falha do teste"
			}
		},
		"required": ["error_log"]
	}`)
}

func (t *BugExplainerTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		ErrorLog string `json:"error_log"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Result{Success: false, Error: "argumentos inválidos"}, nil
	}

	// Classificação inicial de gravidade (Task 38)
	severity := "Moderado (Falha de Lógica / Assert)"
	lowerLog := strings.ToLower(input.ErrorLog)
	if strings.Contains(lowerLog, "panic") || strings.Contains(lowerLog, "segmentation fault") || strings.Contains(lowerLog, "build failed") || strings.Contains(lowerLog, "timeout") {
		severity = "Crítico (Travamento do sistema / Compilação / OOM / Panic)"
	} else if strings.Contains(lowerLog, "warning") || strings.Contains(lowerLog, "deprecated") || strings.Contains(lowerLog, "lint") {
		severity = "Leve (Lint / Avisos)"
	}

	// Se temos LLMProvider, solicitamos a explicação inteligente
	if t.llmProvider != nil {
		systemMsg := `Você é o Sintetizador de Diagnósticos do CROM-Agente.
Sua tarefa é analisar os logs de erro fornecidos, filtrar ruídos desnecessários (como stack traces longos do runtime ou prints repetitivos) e gerar um relatório estruturado em markdown contendo:
1. Gravidade da Falha.
2. Causa provável do erro.
3. Localização exata no código (arquivo, linha, método) - extraia isso do erro.
4. Sugestão clara de como corrigir o problema no código.
Retorne apenas o relatório estruturado em markdown, sem qualquer outra introdução ou conclusão.`

		userMsg := fmt.Sprintf("Gravidade preliminar: %s\nLogs de erro:\n%s", severity, input.ErrorLog)

		resp, err := t.llmProvider.SendMessages(ctx, []llm.Message{
			{Role: "system", Content: systemMsg},
			{Role: "user", Content: userMsg},
		}, llm.RequestOptions{})

		if err == nil && resp != nil && resp.Message.Content != "" {
			return tools.Result{
				Success: true,
				Data:    resp.Message.Content,
			}, nil
		}
	}

	// Fallback analítico estático
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("### 🐛 Diagnóstico Estático de Falha\n\n"))
	sb.WriteString(fmt.Sprintf("- **Gravidade:** %s\n", severity))

	// Tenta extrair arquivo/linha
	fileLineRegex := regexp.MustCompile(`([a-zA-Z0-9_/.-]+\.go):(\d+)`)
	matches := fileLineRegex.FindStringSubmatch(input.ErrorLog)
	if len(matches) > 2 {
		sb.WriteString(fmt.Sprintf("- **Localização Provável:** `%s` na linha `%s`\n", matches[1], matches[2]))
	} else {
		sb.WriteString("- **Localização Provável:** Indeterminada pelo log básico.\n")
	}

	sb.WriteString("\n**Resumo do Log:**\n")
	lines := strings.Split(input.ErrorLog, "\n")
	count := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && (strings.Contains(strings.ToLower(trimmed), "fail") || strings.Contains(strings.ToLower(trimmed), "panic") || strings.Contains(strings.ToLower(trimmed), "error") || strings.Contains(strings.ToLower(trimmed), "assert")) {
			sb.WriteString(fmt.Sprintf("> %s\n", trimmed))
			count++
			if count >= 5 {
				break
			}
		}
	}

	return tools.Result{
		Success: true,
		Data:    sb.String(),
	}, nil
}
