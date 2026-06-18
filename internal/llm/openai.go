package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type OpenAIProvider struct {
	apiKey string
	model  string
	URL    string
}

func NewOpenAIProvider(apiKey, model string) *OpenAIProvider {
	if model == "" {
		model = "gpt-4o"
	}
	return &OpenAIProvider{
		apiKey: apiKey,
		model:  model,
		URL:    "https://api.openai.com/v1/chat/completions",
	}
}

func (p *OpenAIProvider) Name() string {
	return "openai"
}

type openAIChatMessage struct {
	Role       string      `json:"role"`
	Content    interface{} `json:"content,omitempty"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
	Name       string      `json:"name,omitempty"`
}

func sanitizeMessagesForTextOnly(messages []openAIChatMessage) []openAIChatMessage {
	sanitized := make([]openAIChatMessage, len(messages))
	for i, m := range messages {
		var content interface{} = m.Content
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			var calls []string
			for _, tc := range m.ToolCalls {
				calls = append(calls, fmt.Sprintf("%s(%s)", tc.Function.Name, tc.Function.Arguments))
			}
			contentStr, ok := content.(string)
			if ok {
				content = contentStr + fmt.Sprintf("\n[Chamando ferramentas: %s]", strings.Join(calls, ", "))
			}
		} else if m.Role == "tool" {
			contentStr, _ := content.(string)
			sanitized[i] = openAIChatMessage{
				Role:    "user",
				Content: fmt.Sprintf("[Retorno da ferramenta %s: %s]", m.Name, contentStr),
			}
			continue
		}
		sanitized[i] = openAIChatMessage{
			Role:    m.Role,
			Content: content,
		}
	}
	return sanitized
}

func parseMultimodalContent(text string) interface{} {
	if !strings.Contains(text, "image:base64:") && !strings.Contains(text, "audio:base64:") {
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
		} else if strings.HasPrefix(trimmed, "audio:base64:") {
			flushText()
			audioContent := strings.TrimPrefix(trimmed, "audio:base64:")
			audioParts := strings.SplitN(audioContent, ":", 2)
			if len(audioParts) == 2 {
				format := audioParts[0]
				b64 := audioParts[1]
				parts = append(parts, map[string]interface{}{
					"type": "input_audio",
					"input_audio": map[string]interface{}{
						"data":   b64,
						"format": format,
					},
				})
			}
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

func (p *OpenAIProvider) SendMessages(ctx context.Context, messages []Message, opts RequestOptions) (*Response, error) {
	url := p.URL

	reqMessages := make([]openAIChatMessage, len(messages))
	for i, m := range messages {
		var content interface{} = m.Content
		if m.Role == "user" {
			content = parseMultimodalContent(m.Content)
		}
		reqMessages[i] = openAIChatMessage{
			Role:       m.Role,
			Content:    content,
			ToolCalls:  m.ToolCalls,
			ToolCallID: m.ToolCallID,
			Name:       m.Name,
		}
	}


	type openAIRequest struct {
		Model      string              `json:"model"`
		Messages   []openAIChatMessage `json:"messages"`
		Tools      []ToolDefinition    `json:"tools,omitempty"`
		ToolChoice interface{}         `json:"tool_choice,omitempty"`
	}

	reqBody := openAIRequest{
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
		return nil, fmt.Errorf("openai: erro ao serializar request: %w", err)
	}

	client := &http.Client{}
	var resp *http.Response
	var req *http.Request
	var bodyBytes []byte
	maxRetries := 5
	
	for attempt := 1; attempt <= maxRetries; attempt++ {
		var err error
		req, err = http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
		if err != nil {
			return nil, fmt.Errorf("openai: erro ao criar request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+p.apiKey)

		resp, err = client.Do(req)
		if err != nil {
			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt*5) * time.Second)
				continue
			}
			return nil, fmt.Errorf("openai: falha na requisição HTTP após retries: %w", err)
		}

		bodyBytes, err = io.ReadAll(resp.Body)
		if err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("openai: erro ao ler response body: %w", err)
		}

		// Se for Rate Limit (429) ou erro de servidor (5xx), tenta de novo
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			resp.Body.Close()
			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt*5) * time.Second)
				continue
			}
			break // sai do loop e processa o erro
		}
		
		// Se deu certo ou é um erro de cliente (400, 401, 404), quebra o loop
		break
	}
	
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}

	if resp.StatusCode != http.StatusOK {
		bodyStr := string(bodyBytes)
		// Se falhar devido a suporte de ferramentas, tenta novamente em modo texto puro
		if len(opts.Tools) > 0 && (strings.Contains(bodyStr, "tool") || strings.Contains(bodyStr, "support") || strings.Contains(bodyStr, "parameter")) {
			reqBody.Tools = nil
			reqBody.ToolChoice = nil
			reqBody.Messages = sanitizeMessagesForTextOnly(reqMessages)

			jsonData, err = json.Marshal(reqBody)
			if err != nil {
				return nil, fmt.Errorf("openai: erro ao serializar request de retry: %w", err)
			}
			req, err = http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
			if err != nil {
				return nil, fmt.Errorf("openai: erro ao criar request de retry: %w", err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+p.apiKey)

			respRetry, err := client.Do(req)
			if err != nil {
				return nil, fmt.Errorf("openai: falha na requisição de retry: %w", err)
			}
			defer respRetry.Body.Close()

			bodyBytes, err = io.ReadAll(respRetry.Body)
			if err != nil {
				return nil, fmt.Errorf("openai: erro ao ler body de retry: %w", err)
			}

			if respRetry.StatusCode != http.StatusOK {
				return nil, fmt.Errorf("openai: retry failed (%d): %s", respRetry.StatusCode, string(bodyBytes))
			}
		} else {
			return nil, fmt.Errorf("openai: status HTTP inválido (%d): %s", resp.StatusCode, bodyStr)
		}
	}

	type openAIResponse struct {
		Choices []struct {
			Message Message `json:"message"`
		} `json:"choices"`
		Usage Usage `json:"usage"`
	}

	var apiResp openAIResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("openai: erro ao parsear response: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("openai: resposta vazia do modelo")
	}

	return &Response{
		Message: apiResp.Choices[0].Message,
		Usage:   apiResp.Usage,
	}, nil
}
