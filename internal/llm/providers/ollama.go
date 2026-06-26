package providers

import (
	"context"
	"strings"

	"github.com/crom/crom-agente/internal/llm"
)

// OllamaProvider is a wrapper around OpenAIProvider that points to the local Ollama instance.
// Ollama has native support for the OpenAI Chat Completions API format.
type OllamaProvider struct {
	*OpenAIProvider
}

// NewOllamaProvider creates a new provider connecting to local Ollama.
func NewOllamaProvider(endpointURL string, model string) *OllamaProvider {
	if endpointURL == "" {
		endpointURL = "http://localhost:11434"
	}
	
	// Ensure the endpoint points to the v1 chat completions API if it doesn't already
	if !strings.HasSuffix(endpointURL, "/v1/chat/completions") {
		if strings.HasSuffix(endpointURL, "/") {
			endpointURL += "v1/chat/completions"
		} else {
			endpointURL += "/v1/chat/completions"
		}
	}
	
	base := NewOpenAIProvider("ollama", model)
	base.URL = endpointURL
	
	return &OllamaProvider{
		OpenAIProvider: base,
	}
}

func (o *OllamaProvider) Name() string {
	return "ollama"
}

func sanitizeMessagesForDeepSeek(messages []llm.Message) []llm.Message {
	sanitized := make([]llm.Message, len(messages))
	for i, msg := range messages {
		sMsg := msg
		
		// If assistant message has tool calls, append/prepend tool call info into text
		if sMsg.Role == "assistant" && len(sMsg.ToolCalls) > 0 {
			var builder strings.Builder
			builder.WriteString(sMsg.Content)
			if sMsg.Content != "" {
				builder.WriteString("\n")
			}
			builder.WriteString("[Chamando ferramentas:")
			for _, tc := range sMsg.ToolCalls {
				builder.WriteString(" " + tc.Function.Name)
			}
			builder.WriteString("]")
			sMsg.Content = builder.String()
			sMsg.ToolCalls = nil // Remove tool calls so API doesn't complain about them
		}
		
		// If tool message, convert role to user, and sanitize content
		if sMsg.Role == "tool" {
			sMsg.Role = "user"
			sMsg.Content = "[Retorno da ferramenta " + sMsg.Name + ": " + sMsg.Content + "]"
			sMsg.Name = ""
			sMsg.ToolCallID = ""
		}
		
		sanitized[i] = sMsg
	}
	return sanitized
}

func (o *OllamaProvider) SendMessages(ctx context.Context, messages []llm.Message, opts llm.RequestOptions) (*llm.Response, error) {
	if strings.Contains(strings.ToLower(o.model), "deepseek") {
		messages = sanitizeMessagesForDeepSeek(messages)
		opts.Tools = nil
	}
	return o.OpenAIProvider.SendMessages(ctx, messages, opts)
}

func (o *OllamaProvider) StreamMessages(ctx context.Context, messages []llm.Message, opts llm.RequestOptions, chunkChan chan<- string) (*llm.Response, error) {
	if strings.Contains(strings.ToLower(o.model), "deepseek") {
		messages = sanitizeMessagesForDeepSeek(messages)
		opts.Tools = nil
	}
	return o.OpenAIProvider.StreamMessages(ctx, messages, opts, chunkChan)
}
