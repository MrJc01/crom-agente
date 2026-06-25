package llm

import "strings"

// ModelCapabilities representa os atributos e recursos suportados por um modelo de LLM.
type ModelCapabilities struct {
	ToolUse          bool
	Vision           bool
	MaxContext       int
	StreamingSupport bool
}

// CapabilityProvider define a interface para consultar capacidades.
type CapabilityProvider interface {
	Capabilities() ModelCapabilities
}

// KnownCapabilities é a tabela estática com as capacidades de modelos conhecidos.
var KnownCapabilities = map[string]ModelCapabilities{
	"gpt-4o": {
		ToolUse:          true,
		Vision:           true,
		MaxContext:       128000,
		StreamingSupport: true,
	},
	"gpt-4o-mini": {
		ToolUse:          true,
		Vision:           true,
		MaxContext:       128000,
		StreamingSupport: true,
	},
	"claude-3-5-sonnet": {
		ToolUse:          true,
		Vision:           true,
		MaxContext:       200000,
		StreamingSupport: true,
	},
	"claude-3-opus": {
		ToolUse:          true,
		Vision:           true,
		MaxContext:       200000,
		StreamingSupport: true,
	},
	"gemini-1.5-pro": {
		ToolUse:          true,
		Vision:           true,
		MaxContext:       2000000,
		StreamingSupport: true,
	},
	"gemini-1.5-flash": {
		ToolUse:          true,
		Vision:           true,
		MaxContext:       1000000,
		StreamingSupport: true,
	},
	"llama-3-8b": {
		ToolUse:          true,
		Vision:           false,
		MaxContext:       8192,
		StreamingSupport: true,
	},
	"llama-3.2-3b": {
		ToolUse:          false,
		Vision:           false,
		MaxContext:       128000,
		StreamingSupport: true,
	},
	// Mapeamentos OpenRouter comuns
	"meta-llama/llama-3.2-3b-instruct": {
		ToolUse:          false,
		Vision:           false,
		MaxContext:       128000,
		StreamingSupport: true,
	},
	"meta-llama/llama-3-8b-instruct": {
		ToolUse:          true,
		Vision:           false,
		MaxContext:       8192,
		StreamingSupport: true,
	},
}

// GetCapabilities busca as capacidades para um determinado modelo.
// Se o modelo não estiver mapeado na tabela estática, retorna um fallback dinâmico (por padrão Assume ToolUse=true).
func GetCapabilities(model string) ModelCapabilities {
	modelLower := strings.ToLower(model)

	// Busca exata na tabela
	if caps, ok := KnownCapabilities[model]; ok {
		return caps
	}

	// Busca por correspondência parcial (ex: "llama-3.2-3b" dentro de caminhos do OpenRouter)
	for key, caps := range KnownCapabilities {
		if strings.Contains(modelLower, strings.ToLower(key)) {
			return caps
		}
	}

	// Fallback padrão se não catalogado
	return ModelCapabilities{
		ToolUse:          true,
		Vision:           false,
		MaxContext:       8192,
		StreamingSupport: true,
	}
}
