package core

import (
	"context"
	"fmt"

	"github.com/crom/crom-agente/internal/i18n"
	"github.com/crom/crom-agente/internal/llm"
)

// CompressHistory utiliza o provedor LLM para sumarizar de forma concisa o historico de execucao do subagente
func CompressHistory(ctx context.Context, provider llm.Provider, prompt string, result string, priorSummary string) (string, error) {
	if provider == nil {
		// Fallback se nao houver LLM configurado: concatena informacoes basicas
		return fmt.Sprintf("%s\n- Tarefa: %s\n- Resultado: %s", priorSummary, prompt, result), nil
	}

	sysRole := i18n.Get("system.summarizer_role")
	if sysRole == "" {
		sysRole = "Resumir histórico técnico do agente. Seja extremamente direto."
	}

	userPromptTemplate := i18n.Get("system.summarizer_prompt")
	if userPromptTemplate == "" {
		userPromptTemplate = "Resuma o progresso.\nTarefa: %s\nResultado: %s\nHistórico anterior: %s"
	}

	messages := []llm.Message{
		{
			Role:    "system",
			Content: sysRole,
		},
		{
			Role:    "user",
			Content: fmt.Sprintf(userPromptTemplate, prompt, result, priorSummary),
		},
	}

	// Limita os tokens de saída para forçar concisão extrema
	resp, err := provider.SendMessages(ctx, messages, llm.RequestOptions{})
	if err != nil {
		return "", fmt.Errorf("falha ao enviar mensagens de compressao: %w", err)
	}

	if resp == nil || resp.Message.Content == "" {
		return fmt.Sprintf("%s\n- Tarefa: %s\n- Resultado: %s", priorSummary, prompt, result), nil
	}

	return resp.Message.Content, nil
}
