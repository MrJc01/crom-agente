package tooling

import (
	"strings"

	"github.com/crom/crom-agente/internal/llm"
	"github.com/crom/crom-agente/internal/tools"
)

// BuildRequestOptions constrói as definições de ferramentas para enviar ao LLM
func BuildRequestOptions(availableTools map[string]tools.Tool, intent string) llm.RequestOptions {
	if len(availableTools) == 0 {
		return llm.RequestOptions{}
	}

	defs := make([]llm.ToolDefinition, 0, len(availableTools))
	intentLower := strings.ToLower(intent)

	for _, t := range availableTools {
		// Tool Pruning Rudimentar: se temos muitas ferramentas, podemos podar ferramentas super específicas
		// se a intenção atual claramente não envolve seus domínios (ex: mcp)
		if strings.HasPrefix(t.ID(), "mcp_") && !strings.Contains(intentLower, "mcp") && !strings.Contains(intentLower, "external") {
			// Skip MCP tools se não parecerem relevantes
			continue
		}

		defs = append(defs, llm.ToolDefinition{
			Type: "function",
			Function: llm.ToolFunctionSchema{
				Name:        t.ID(),
				Description: t.Description(),
				Parameters:  t.ParametersSchema(),
			},
		})
	}

	return llm.RequestOptions{
		Tools:      defs,
		ToolChoice: "auto",
	}
}
