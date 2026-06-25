package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/crom/crom-agente/internal/llm"
)

type GeminiProvider struct {
	apiKey string
	model  string
}

func NewGeminiProvider(apiKey, model string) *GeminiProvider {
	if model == "" {
		model = "gemini-2.5-pro"
	}
	return &GeminiProvider{
		apiKey: apiKey,
		model:  model,
	}
}

func (p *GeminiProvider) Name() string {
	return "gemini"
}

func (p *GeminiProvider) SupportsSystemPrompt() bool {
	return true
}

func parseGeminiMultimodalContent(text string) interface{} {
	if !strings.Contains(text, "image:base64:") {
		return text
	}

	var parts []interface{}
	lines := strings.Split(text, "\n")
	var currentText []string

	flushText := func() {
		if len(currentText) > 0 {
			joined := strings.Join(currentText, "\n")
			if strings.TrimSpace(joined) != "" {
				parts = append(parts, map[string]interface{}{
					"type": "text",
					"text": joined,
				})
			}
			currentText = nil
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "image:base64:") {
			flushText()
			b64 := strings.TrimPrefix(trimmed, "image:base64:")
			parts = append(parts, map[string]interface{}{
				"type": "image_url",
				"image_url": map[string]interface{}{
					"url": "data:image/png;base64," + b64,
				},
			})
		} else {
			currentText = append(currentText, line)
		}
	}
	flushText()

	if len(parts) == 0 {
		return text
	}
	return parts
}

func (p *GeminiProvider) SendMessages(ctx context.Context, messages []llm.Message, opts llm.RequestOptions) (*llm.Response, error) {
	// Endpoint oficial do Google com compatibilidade OpenAI
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/chat/completions?key=%s", p.apiKey)

	type geminiChatMessage struct {
		Role       string         `json:"role"`
		Content    interface{}    `json:"content,omitempty"`
		ToolCalls  []llm.ToolCall `json:"tool_calls,omitempty"`
		ToolCallID string         `json:"tool_call_id,omitempty"`
		Name       string         `json:"name,omitempty"`
	}

	// Layer 2: Extrai o contexto escrito da mídia antes do envio
	// Gera a URL completa com a chave para uso do MediaExtractor se ele precisar fazer VLM
	completeURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/chat/completions?key=%s", p.apiKey)
	injectedMessages := llm.ExtractAndInjectMediaContext(ctx, messages, p.Name(), p.apiKey, completeURL)

	reqMessages := make([]geminiChatMessage, len(injectedMessages))
	for i, m := range injectedMessages {
		role := m.Role
		var content interface{} = m.Content
		if m.Role == "user" {
			content = parseGeminiMultimodalContent(m.Content)
		}
		reqMessages[i] = geminiChatMessage{
			Role:       role,
			Content:    content,
			ToolCalls:  m.ToolCalls,
			ToolCallID: m.ToolCallID,
			Name:       m.Name,
		}
	}

	type geminiRequest struct {
		Model      string               `json:"model"`
		Messages   []geminiChatMessage  `json:"messages"`
		Tools      []llm.ToolDefinition `json:"tools,omitempty"`
		ToolChoice interface{}          `json:"tool_choice,omitempty"`
	}

	reqBody := geminiRequest{
		Model:    p.model,
		Messages: reqMessages,
	}

	if len(opts.Tools) > 0 {
		reqBody.Tools = opts.Tools
		if opts.ToolChoice != "" {
			reqBody.ToolChoice = opts.ToolChoice
		} else {
			reqBody.ToolChoice = "auto"
		}
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("gemini: erro ao serializar request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("gemini: erro ao criar request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini: falha na requisição HTTP: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gemini: erro ao ler response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		bodyStr := string(bodyBytes)

		// Detecta erros relacionados à falta de suporte de visão/multimodal
		isVisionError := strings.Contains(bodyStr, "support") || strings.Contains(bodyStr, "vision") ||
			strings.Contains(bodyStr, "image") || strings.Contains(bodyStr, "multimodal") ||
			strings.Contains(bodyStr, "endpoint") || strings.Contains(bodyStr, "404")

		if isVisionError {
			// Remove payloads nativos de visão, retendo a descrição textual da imagem já injetada
			for idx := range reqMessages {
				reqMessages[idx].Content = llm.StripMultimodalPayloads(reqMessages[idx].Content)
			}
			reqBody.Messages = reqMessages

			jsonData, err = json.Marshal(reqBody)
			if err != nil {
				return nil, fmt.Errorf("gemini: erro ao serializar request de retry de visão: %w", err)
			}

			req, err = http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
			if err != nil {
				return nil, fmt.Errorf("gemini: erro ao criar request de retry de visão: %w", err)
			}
			req.Header.Set("Content-Type", "application/json")

			respRetry, err := client.Do(req)
			if err != nil {
				return nil, fmt.Errorf("gemini: falha na requisição de retry de visão: %w", err)
			}
			defer respRetry.Body.Close()

			bodyBytes, err = io.ReadAll(respRetry.Body)
			if err != nil {
				return nil, fmt.Errorf("gemini: erro ao ler body de retry de visão: %w", err)
			}

			if respRetry.StatusCode != http.StatusOK {
				return nil, fmt.Errorf("gemini: retry de visão failed (%d): %s", respRetry.StatusCode, string(bodyBytes))
			}
		} else {
			return nil, fmt.Errorf("gemini: status HTTP inválido (%d): %s", resp.StatusCode, bodyStr)
		}
	}

	type geminiResponse struct {
		Choices []struct {
			Message llm.Message `json:"message"`
		} `json:"choices"`
		Usage llm.Usage `json:"usage"`
	}

	var apiResp geminiResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("gemini: erro ao parsear response: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("gemini: resposta vazia do modelo")
	}

	return &llm.Response{
		Message: apiResp.Choices[0].Message,
		Usage:   apiResp.Usage,
	}, nil
}
