package llm

import (
	"fmt"
)


// WrapProvider encapsulates an existing provider with standard enterprise decorators like Retry
func WrapProvider(p Provider) Provider {
	return NewRetryProvider(p, 3)
}

// NewProvider fabrica um LLM Provider com base no nome, modelo e função para obter variáveis de ambiente
func NewProvider(providerName, model string, getEnv func(string) string) (Provider, error) {
	p, err := buildBaseProvider(providerName, model, getEnv)
	if err != nil {
		return nil, err
	}
	return WrapProvider(p), nil
}

func buildBaseProvider(providerName, model string, getEnv func(string) string) (Provider, error) {
	switch providerName {
	case "openai":
		apiKey := getEnv("OPENAI_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY não está configurada no .env")
		}
		return NewOpenAIProvider(apiKey, model), nil
	case "gemini":
		apiKey := getEnv("GEMINI_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("GEMINI_API_KEY não está configurada no .env")
		}
		return NewGeminiProvider(apiKey, model), nil
	case "anthropic":
		apiKey := getEnv("ANTHROPIC_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("ANTHROPIC_API_KEY não está configurada no .env")
		}
		return NewAnthropicProvider(apiKey, model), nil
	case "ollama":
		endpoint := getEnv("OLLAMA_HOST")
		if endpoint == "" {
			endpoint = "http://localhost:11434"
		}
		return NewOllamaProvider(endpoint, model), nil
	case "openrouter":
		apiKey := getEnv("OPENROUTER_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("OPENROUTER_API_KEY nao esta configurada no .env")
		}
		p := NewOpenAIProvider(apiKey, model)
		p.URL = "https://openrouter.ai/api/v1/chat/completions"
		return p, nil
	case "cromia":
		apiKey := getEnv("CROMIA_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("CROMIA_API_KEY nao esta configurada no .env (Verifique o login no Desktop App)")
		}
		p := NewOpenAIProvider(apiKey, model)
		p.URL = "https://cloud.ia.crom.run/api/v1/chat/completions"
		return p, nil
	case "mock":
		return NewMockProvider(), nil
	default:
		return nil, fmt.Errorf("provedor de LLM desconhecido: %s", providerName)
	}
}
