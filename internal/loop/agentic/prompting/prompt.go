package prompting

import (
	"context"
	"fmt"
	"strings"

	"github.com/crom/crom-agente/internal/i18n"
	"github.com/crom/crom-agente/internal/llm"
)

// OptimizePrompt executa uma chamada de LLM para refinar e enriquecer o prompt do usuário antes do loop ReAct
func OptimizePrompt(ctx context.Context, provider llm.Provider, rawPrompt string) (string, error) {
	// Se o prompt for muito curto ou for um comando simples/TUI slash command, não otimiza
	if len(rawPrompt) < 5 || strings.HasPrefix(rawPrompt, "/") {
		return rawPrompt, nil
	}

	messages := []llm.Message{
		{Role: "system", Content: i18n.Get("system.optimizer_system_prompt")},
		{Role: "user", Content: i18n.Get("system.optimizer_user_prompt", rawPrompt)},
	}

	resp, err := provider.SendMessages(ctx, messages, llm.RequestOptions{})
	if err != nil {
		return "", err
	}

	optimized := strings.TrimSpace(resp.Message.Content)
	if optimized == "" {
		return "", fmt.Errorf("%s", i18n.Get("errors.optimizer_blank_response"))
	}

	return optimized, nil
}
