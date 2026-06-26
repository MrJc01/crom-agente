package prompting

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/crom/crom-agente/internal/i18n"
	"github.com/crom/crom-agente/internal/llm"
)

// MessageHandler define a interface mínima necessária para reportar logs
type MessageHandler interface {
	OnMessage(role, msg string)
}

// truncateStr helper para não estourar o buffer local
func truncateStr(s string, max int) string {
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}

// Regex patterns para extração determinística
var (
	funcSigRegex   = regexp.MustCompile(`(?m)^(?:func |def |class |function |export (?:default )?(?:function |class ))([^\n{(]+)`)
	stackRegex     = regexp.MustCompile(`(?m)^\s+at .+:\d+|^\s*File ".+", line \d+|^\s+[a-zA-Z0-9_/.-]+\.(?:go|py|js|ts):\d+`)
	errorLineRegex = regexp.MustCompile(`(?im)(?:error|panic|exception|traceback|fatal|fail(?:ed|ure)?)[:\s].{5,120}`)
	filePathRegex  = regexp.MustCompile(`(?:[\w./\\-]+\.(?:go|py|js|ts|jsx|tsx|java|rs))(?::\d+)?`)
)

// CompactMessages aplica compactação inteligente usando heurísticas determinísticas (sem custo LLM)
// O LLM é usado como fallback apenas se useLLMFallback for true
func CompactMessages(ctx context.Context, provider llm.Provider, maxMsgs int, handler MessageHandler, messages []llm.Message) []llm.Message {
	if maxMsgs <= 0 {
		maxMsgs = 40
	}

	currentTokens := llm.CountTokens(messages, "", provider.Name(), "default")
	if len(messages) <= maxMsgs && currentTokens <= 50000 {
		return messages
	}

	keepRecent := 10 // equivale a 5 turnos completos de iteração
	if maxMsgs < 20 {
		keepRecent = 5
	}

	middleStart := 0
	for i, m := range messages {
		if m.Role == "user" {
			middleStart = i + 1
			break
		}
	}

	if middleStart == 0 || middleStart >= len(messages)-keepRecent {
		// Fallback para truncamento simples
		keepFromEnd := maxMsgs - 1
		compacted := make([]llm.Message, 0, maxMsgs)
		compacted = append(compacted, messages[0])
		compacted = append(compacted, messages[len(messages)-keepFromEnd:]...)
		return compacted
	}

	var compacted []llm.Message
	// Sliding Window Dinâmica por Tokens (Task 73)
	for {
		middleEnd := len(messages) - keepRecent
		if middleEnd <= middleStart {
			middleEnd = middleStart + 1
		}

		summary := extractDeterministicSummary(messages[middleStart:middleEnd])

		compacted = make([]llm.Message, 0)
		compacted = append(compacted, messages[:middleStart]...)
		compacted = append(compacted, llm.Message{
			Role:    "system",
			Content: i18n.Get("system.compactor_history_summary", summary),
		})
		if middleEnd < len(messages) {
			compacted = append(compacted, messages[middleEnd:]...)
		}

		compactedTokens := llm.CountTokens(compacted, "", provider.Name(), "default")
		if compactedTokens <= 50000 || keepRecent <= 2 {
			break
		}
		
		// Se ainda está acima do limite de tokens, tentamos compactar mais turnos recentes
		keepRecent -= 2
	}

	if handler != nil {
		handler.OnMessage("system", i18n.Get("system.compactor_optimization_log"))
	}

	log.Printf("[AgenticLoop] Compactou histórico de %d para %d mensagens (determinístico, limit=50k tokens)", len(messages), len(compacted))
	return compacted
}

// extractDeterministicSummary extrai um resumo estruturado sem usar LLM
func extractDeterministicSummary(messages []llm.Message) string {
	var sb strings.Builder

	// 1. Timeline de ações realizadas
	var timeline []string
	toolCallCount := make(map[string]int)
	var errors []string
	var funcSignatures []string
	filesModified := make(map[string]bool)

	for _, msg := range messages {
		content := msg.Content

		// Contar tool calls
		if len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				toolCallCount[tc.Function.Name]++

				// Extrair arquivos de argumentos de escrita
				if tc.Function.Name == "write_file" || tc.Function.Name == "edit_file" {
					if paths := filePathRegex.FindAllString(tc.Function.Arguments, 3); len(paths) > 0 {
						for _, p := range paths {
							filesModified[p] = true
						}
					}
				}
			}
		}

		// Extrair erros/stack traces do conteúdo
		if msg.Role == "tool" || (msg.Role == "assistant" && strings.Contains(strings.ToLower(content), "error")) {
			if errMatches := errorLineRegex.FindAllString(content, 3); len(errMatches) > 0 {
				for _, e := range errMatches {
					trimmed := strings.TrimSpace(e)
					if len(trimmed) > 10 && len(errors) < 5 {
						errors = append(errors, truncateStr(trimmed, 120))
					}
				}
			}
		}

		// Extrair assinaturas de funções mencionadas
		if msg.Role == "assistant" {
			if sigs := funcSigRegex.FindAllString(content, 5); len(sigs) > 0 {
				for _, sig := range sigs {
					trimmed := strings.TrimSpace(sig)
					if len(trimmed) > 5 && len(funcSignatures) < 8 {
						funcSignatures = append(funcSignatures, truncateStr(trimmed, 80))
					}
				}
			}

			// Construir timeline de ações do assistente
			firstLine := firstNonEmptyLine(content)
			if firstLine != "" && len(timeline) < 10 {
				timeline = append(timeline, truncateStr(firstLine, 100))
			}
		}
	}

	// Montar o resumo estruturado
	sb.WriteString("=== RESUMO DO HISTÓRICO COMPACTADO ===\n\n")

	// Timeline
	if len(timeline) > 0 {
		sb.WriteString("[TIMELINE DE AÇÕES]\n")
		for i, action := range timeline {
			sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, action))
		}
		sb.WriteString("\n")
	}

	// Tool calls resumidos
	if len(toolCallCount) > 0 {
		sb.WriteString("[FERRAMENTAS EXECUTADAS]\n")
		for name, count := range toolCallCount {
			sb.WriteString(fmt.Sprintf("  - %s: %dx\n", name, count))
		}
		sb.WriteString("\n")
	}

	// Arquivos modificados
	if len(filesModified) > 0 {
		sb.WriteString("[ARQUIVOS MODIFICADOS]\n")
		for path := range filesModified {
			sb.WriteString(fmt.Sprintf("  - %s\n", path))
		}
		sb.WriteString("\n")
	}

	// Erros encontrados
	if len(errors) > 0 {
		sb.WriteString("[ERROS DETECTADOS]\n")
		for _, e := range errors {
			sb.WriteString(fmt.Sprintf("  ❌ %s\n", e))
		}
		sb.WriteString("\n")
	}

	// Assinaturas de funções
	if len(funcSignatures) > 0 {
		sb.WriteString("[FUNÇÕES/CLASSES RELEVANTES]\n")
		for _, sig := range funcSignatures {
			sb.WriteString(fmt.Sprintf("  - %s\n", sig))
		}
	}

	return sb.String()
}

// firstNonEmptyLine retorna a primeira linha não-vazia de um texto
func firstNonEmptyLine(s string) string {
	lines := strings.SplitN(s, "\n", 5)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) > 3 {
			return trimmed
		}
	}
	return ""
}

