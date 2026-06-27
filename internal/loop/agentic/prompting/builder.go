package prompting

import (
	"fmt"
	"sort"
	"strings"

	"github.com/crom/crom-agente/internal/config"
	"github.com/crom/crom-agente/internal/tools"
)

// BuildToolsInstructions monta dinamicamente o bloco de instruções para todas as ferramentas e subagentes registrados.
func BuildToolsInstructions(pm *config.PromptManager, registeredTools []tools.Tool) string {
	if len(registeredTools) == 0 {
		return ""
	}

	// Criar uma cópia e ordenar por ID para garantir determinismo no prompt
	sortedTools := make([]tools.Tool, len(registeredTools))
	copy(sortedTools, registeredTools)
	sort.Slice(sortedTools, func(i, j int) bool {
		return sortedTools[i].ID() < sortedTools[j].ID()
	})

	var sb strings.Builder
	sb.WriteString("\n### DIRETRIZES DE FERRAMENTAS E SUBAGENTES DISPONÍVEIS:\n")

	for _, t := range sortedTools {
		toolID := t.ID()
		promptKey := fmt.Sprintf("tool_instruction_%s", toolID)

		var instruction string
		if pm != nil {
			if p, ok := pm.GetPrompt(promptKey); ok && p.Enabled {
				instruction = p.Content
			}
		}

		if instruction == "" {
			// Fallback para a descrição interna da ferramenta
			instruction = t.Description()
		}

		sb.WriteString(fmt.Sprintf("- **%s**: %s\n", toolID, instruction))
	}

	sb.WriteString("\n⚠️ **FORMATO OBRIGATÓRIO DE FERRAMENTAS:** NUNCA escreva código Python/JS como `write_file(path=...)` para acionar ferramentas no texto. Você DEVE usar EXCLUSIVAMENTE as chamadas estruturadas JSON (Tool Calling) nativas. Chamadas em texto/código serão ignoradas.\n")

	return sb.String()
}

