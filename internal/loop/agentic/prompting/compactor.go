package prompting

import (
	"context"
	"fmt"
	"log"

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

// CompactMessages aplica compactação inteligente usando o LLM para resumir o meio da conversa
func CompactMessages(ctx context.Context, provider llm.Provider, maxMsgs int, handler MessageHandler, messages []llm.Message) []llm.Message {
	if maxMsgs <= 0 {
		maxMsgs = 40
	}

	if len(messages) <= maxMsgs {
		return messages
	}

	keepRecent := 15
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

	middleEnd := len(messages) - keepRecent

	var toSummarize string
	for i := middleStart; i < middleEnd; i++ {
		content := truncateStr(messages[i].Content, 500)
		toSummarize += fmt.Sprintf("[%s]: %s\n", messages[i].Role, content)
		if len(messages[i].ToolCalls) > 0 {
			toSummarize += fmt.Sprintf("[%s] executou %d chamadas de ferramenta.\n", messages[i].Role, len(messages[i].ToolCalls))
		}
	}

	summaryPrompt := i18n.Get("system.compactor_summary_prompt", toSummarize)

	resp, err := provider.SendMessages(ctx, []llm.Message{
		{Role: "system", Content: i18n.Get("system.compactor_system_role")},
		{Role: "user", Content: summaryPrompt},
	}, llm.RequestOptions{})

	var compacted []llm.Message
	if err == nil && resp.Message.Content != "" {
		if handler != nil {
			handler.OnMessage("system", i18n.Get("system.compactor_optimization_log"))
		}
		compacted = append(compacted, messages[:middleStart]...)
		
		// Preserve any system messages from the middle block
		for i := middleStart; i < middleEnd; i++ {
			if messages[i].Role == "system" {
				compacted = append(compacted, messages[i])
			}
		}

		compacted = append(compacted, llm.Message{
			Role:    "system",
			Content: i18n.Get("system.compactor_history_summary", resp.Message.Content),
		})
		compacted = append(compacted, messages[middleEnd:]...)
	} else {
		// Fallback
		keepFromEnd := maxMsgs - 1
		compacted = append(compacted, messages[0])
		compacted = append(compacted, messages[len(messages)-keepFromEnd:]...)
	}

	log.Printf("[AgenticLoop] Compactou histórico de %d para %d mensagens", len(messages), len(compacted))
	return compacted
}
