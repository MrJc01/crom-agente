package providers

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/crom/crom-agente/internal/llm"
)

// RetryProvider é um wrapper que implementa tolerância a falhas (Exponential Backoff)
type RetryProvider struct {
	underlying llm.Provider
	maxRetries int
}

func NewRetryProvider(p llm.Provider, retries int) *RetryProvider {
	if retries <= 0 {
		retries = 3
	}
	return &RetryProvider{
		underlying: p,
		maxRetries: retries,
	}
}

func (r *RetryProvider) Name() string {
	return r.underlying.Name()
}

func (r *RetryProvider) SupportsSystemPrompt() bool {
	return r.underlying.SupportsSystemPrompt()
}

func (r *RetryProvider) SendMessages(ctx context.Context, messages []llm.Message, opts llm.RequestOptions) (*llm.Response, error) {
	var lastErr error
	backoff := 2 * time.Second

	for i := 0; i < r.maxRetries; i++ {
		resp, err := r.underlying.SendMessages(ctx, messages, opts)
		if err == nil {
			return resp, nil
		}

		// Erros irrecuperáveis que não devem tentar novamente
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "invalid api key") ||
			strings.Contains(errStr, "unauthorized") ||
			strings.Contains(errStr, "context canceled") {
			return nil, err
		}

		lastErr = err
		slog.Warn("Falha na chamada LLM, tentando novamente...",
			"provider", r.Name(),
			"tentativa", i+1,
			"max_tentativas", r.maxRetries,
			"erro", err)

		// Aguarda o backoff ou o cancelamento do context
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
			// backoff exponencial: 2s, 4s, 8s...
			backoff *= 2
		}
	}

	return nil, fmt.Errorf("todas as %d tentativas falharam: %w", r.maxRetries, lastErr)
}
