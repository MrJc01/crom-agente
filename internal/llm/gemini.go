package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type GeminiProvider struct {
	apiKey string
	model  string
}

func NewGeminiProvider(apiKey, model string) *GeminiProvider {
	if model == "" {
		model = "gemini-1.5-pro"
	}
	return &GeminiProvider{
		apiKey: apiKey,
		model:  model,
	}
}

func (p *GeminiProvider) Name() string {
	return "gemini"
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

func (p *GeminiProvider) SendMessages(ctx context.Context, messages []Message, opts RequestOptions) (*Response, error) {
	// Endpoint oficial do Google com compatibilidade OpenAI
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/chat/completions?key=%s", p.apiKey)

	type geminiChatMessage struct {
		Role       string      `json:"role"`
		Content    interface{} `json:"content,omitempty"`
		ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
		ToolCallID string      `json:"tool_call_id,omitempty"`
		Name       string      `json:"name,omitempty"`
	}

	reqMessages := make([]geminiChatMessage, len(messages))
	for i, m := range messages {
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
		Model      string              `json:"model"`
		Messages   []geminiChatMessage `json:"messages"`
		Tools      []ToolDefinition    `json:"tools,omitempty"`
		ToolChoice interface{}         `json:"tool_choice,omitempty"`
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
		return nil, fmt.Errorf("gemini: status HTTP inválido (%d): %s", resp.StatusCode, string(bodyBytes))
	}

	type geminiResponse struct {
		Choices []struct {
			Message Message `json:"message"`
		} `json:"choices"`
		Usage Usage `json:"usage"`
	}

	var apiResp geminiResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("gemini: erro ao parsear response: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("gemini: resposta vazia do modelo")
	}

	return &Response{
		Message: apiResp.Choices[0].Message,
		Usage:   apiResp.Usage,
	}, nil
}
