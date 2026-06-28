package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/crom/crom-agente/internal/llm"
)

var (
	capabilitiesMu sync.RWMutex
	toolUseCache   = make(map[string]bool)
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

func (p *OpenAIProvider) SupportsSystemPrompt() bool {
	return true
}

func (p *OpenAIProvider) Capabilities() llm.ModelCapabilities {
	caps := llm.GetCapabilities(p.model)
	capabilitiesMu.RLock()
	supported, cached := toolUseCache[p.URL+"|"+p.model]
	capabilitiesMu.RUnlock()
	if cached && !supported {
		caps.ToolUse = false
	}
	return caps
}

type openAIChatMessage struct {
	Role       string         `json:"role"`
	Content    interface{}    `json:"content,omitempty"`
	ToolCalls  []llm.ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	Name       string         `json:"name,omitempty"`
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

func (p *OpenAIProvider) SendMessages(ctx context.Context, messages []llm.Message, opts llm.RequestOptions) (*llm.Response, error) {
	url := p.URL

	// Verify cached tool use support
	capabilitiesMu.RLock()
	supported, cached := toolUseCache[p.URL+"|"+p.model]
	capabilitiesMu.RUnlock()

	var isToolUseDisabled bool
	if cached && !supported {
		isToolUseDisabled = true
		opts.Tools = nil
		opts.ToolChoice = ""
	}

	// Layer 2: Extrai o contexto escrito da mídia antes do envio
	injectedMessages := llm.ExtractAndInjectMediaContext(ctx, messages, p.Name(), p.apiKey, p.URL)

	reqMessages := make([]openAIChatMessage, len(injectedMessages))
	for i, m := range injectedMessages {
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

	if isToolUseDisabled {
		reqMessages = sanitizeMessagesForTextOnly(reqMessages)
	}

	type openAIRequest struct {
		Model       string               `json:"model"`
		Messages    []openAIChatMessage  `json:"messages"`
		Tools       []llm.ToolDefinition `json:"tools,omitempty"`
		ToolChoice  interface{}          `json:"tool_choice,omitempty"`
		Temperature *float64             `json:"temperature,omitempty"`
		MaxTokens   *int                 `json:"max_tokens,omitempty"`
		Stream      bool                 `json:"stream,omitempty"`
	}

	reqBody := openAIRequest{
		Model:       p.model,
		Messages:    reqMessages,
		Temperature: opts.Temperature,
		MaxTokens:   opts.MaxTokens,
	}

	if len(opts.Tools) > 0 && !isToolUseDisabled {
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
	maxRetries := 3

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
				time.Sleep(time.Duration(attempt*2) * time.Second)
				continue
			}
			return nil, fmt.Errorf("openai: falha na requisição HTTP após retries: %w", err)
		}

		bodyBytes, err = io.ReadAll(resp.Body)
		if err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("openai: erro ao ler response body: %w", err)
		}

		// Se for Rate Limit (429), tenta respeitar o Retry-After ou faz backoff exponencial antes de tentar novamente
		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			if attempt < maxRetries {
				retryAfterStr := resp.Header.Get("Retry-After")
				sleepDuration := time.Duration(attempt*5) * time.Second
				if retryAfterStr != "" {
					if seconds, errConv := strconv.Atoi(retryAfterStr); errConv == nil {
						sleepDuration = time.Duration(seconds) * time.Second
					}
				}
				time.Sleep(sleepDuration)
				continue
			}
			break
		}

		// Se for erro de servidor (5xx), tenta de novo com delay menor
		if resp.StatusCode >= 500 {
			resp.Body.Close()
			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt*2) * time.Second)
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

		// Detecta erros relacionados à falta de suporte de visão/multimodal
		isVisionError := strings.Contains(bodyStr, "support") || strings.Contains(bodyStr, "vision") ||
			strings.Contains(bodyStr, "image") || strings.Contains(bodyStr, "multimodal") ||
			strings.Contains(bodyStr, "endpoint") || strings.Contains(bodyStr, "404")

		if isVisionError {
			log.Printf("[OpenAIProvider] Detectado erro de compatibilidade de visão (%d). Fazendo fallback para texto puro com Layer 2...", resp.StatusCode)

			// Remove payloads nativos de visão, retendo a descrição textual da imagem já injetada
			for idx := range reqMessages {
				reqMessages[idx].Content = llm.StripMultimodalPayloads(reqMessages[idx].Content)
			}
			reqBody.Messages = reqMessages

			// Se também falhar devido a suporte de ferramentas, remove-as
			if len(opts.Tools) > 0 && (strings.Contains(bodyStr, "tool") || strings.Contains(bodyStr, "parameter")) {
				capabilitiesMu.Lock()
				toolUseCache[p.URL+"|"+p.model] = false
				capabilitiesMu.Unlock()
				isToolUseDisabled = true

				reqBody.Tools = nil
				reqBody.ToolChoice = nil
				reqBody.Messages = sanitizeMessagesForTextOnly(reqMessages)
			}

			jsonData, err = json.Marshal(reqBody)
			if err != nil {
				return nil, fmt.Errorf("openai: erro ao serializar request de retry de visão: %w", err)
			}

			req, err = http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
			if err != nil {
				return nil, fmt.Errorf("openai: erro ao criar request de retry de visão: %w", err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+p.apiKey)

			respRetry, err := client.Do(req)
			if err != nil {
				return nil, fmt.Errorf("openai: falha na requisição de retry de visão: %w", err)
			}
			defer respRetry.Body.Close()

			bodyBytes, err = io.ReadAll(respRetry.Body)
			if err != nil {
				return nil, fmt.Errorf("openai: erro ao ler body de retry de visão: %w", err)
			}

			if respRetry.StatusCode != http.StatusOK {
				return nil, fmt.Errorf("openai: retry de visão failed (%d): %s", respRetry.StatusCode, string(bodyBytes))
			}
		} else if len(opts.Tools) > 0 && (strings.Contains(bodyStr, "tool") || strings.Contains(bodyStr, "support") || strings.Contains(bodyStr, "parameter") || strings.Contains(bodyStr, "endpoint")) {
			capabilitiesMu.Lock()
			toolUseCache[p.URL+"|"+p.model] = false
			capabilitiesMu.Unlock()
			isToolUseDisabled = true

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

	// Cache successful tool use capability
	if len(opts.Tools) > 0 && !isToolUseDisabled {
		capabilitiesMu.Lock()
		toolUseCache[p.URL+"|"+p.model] = true
		capabilitiesMu.Unlock()
	}

	type openAIResponse struct {
		Choices []struct {
			Message llm.Message `json:"message"`
		} `json:"choices"`
		Usage llm.Usage `json:"usage"`
	}

	var apiResp openAIResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("openai: erro ao parsear response: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("openai: resposta vazia do modelo")
	}

	return &llm.Response{
		Message:         apiResp.Choices[0].Message,
		Usage:           apiResp.Usage,
		ToolUseDisabled: isToolUseDisabled,
	}, nil
}

type openAIStreamResponse struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content,omitempty"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id,omitempty"`
				Type     string `json:"type,omitempty"`
				Function struct {
					Name      string `json:"name,omitempty"`
					Arguments string `json:"arguments,omitempty"`
				} `json:"function,omitempty"`
			} `json:"tool_calls,omitempty"`
		} `json:"delta"`
	} `json:"choices"`
}

func (p *OpenAIProvider) StreamMessages(ctx context.Context, messages []llm.Message, opts llm.RequestOptions, chunkChan chan<- string) (*llm.Response, error) {
	defer close(chunkChan)

	url := p.URL

	capabilitiesMu.RLock()
	supported, cached := toolUseCache[p.URL+"|"+p.model]
	capabilitiesMu.RUnlock()

	var isToolUseDisabled bool
	if cached && !supported {
		isToolUseDisabled = true
		opts.Tools = nil
		opts.ToolChoice = ""
	}

	injectedMessages := llm.ExtractAndInjectMediaContext(ctx, messages, p.Name(), p.apiKey, p.URL)

	reqMessages := make([]openAIChatMessage, len(injectedMessages))
	for i, m := range injectedMessages {
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

	if isToolUseDisabled {
		reqMessages = sanitizeMessagesForTextOnly(reqMessages)
	}

	type openAIRequest struct {
		Model       string               `json:"model"`
		Messages    []openAIChatMessage  `json:"messages"`
		Tools       []llm.ToolDefinition `json:"tools,omitempty"`
		ToolChoice  interface{}          `json:"tool_choice,omitempty"`
		Temperature *float64             `json:"temperature,omitempty"`
		MaxTokens   *int                 `json:"max_tokens,omitempty"`
		Stream      bool                 `json:"stream,omitempty"`
	}

	reqBody := openAIRequest{
		Model:       p.model,
		Messages:    reqMessages,
		Temperature: opts.Temperature,
		MaxTokens:   opts.MaxTokens,
		Stream:      true,
	}

	if len(opts.Tools) > 0 && !isToolUseDisabled {
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
	maxRetries := 3

	for attempt := 1; attempt <= maxRetries; attempt++ {
		var err error
		req, err = http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
		if err != nil {
			return nil, fmt.Errorf("openai: erro ao criar request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
		req.Header.Set("Accept", "text/event-stream")

		resp, err = client.Do(req)
		if err != nil {
			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt*2) * time.Second)
				continue
			}
			return nil, fmt.Errorf("openai: falha na requisição HTTP após retries: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt*5) * time.Second)
				continue
			}
			break
		}

		if resp.StatusCode >= 500 {
			resp.Body.Close()
			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt*2) * time.Second)
				continue
			}
			break
		}

		break
	}

	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		bodyStr := strings.ToLower(string(bodyBytes))

		if len(opts.Tools) > 0 && (strings.Contains(bodyStr, "tool") || strings.Contains(bodyStr, "support") || strings.Contains(bodyStr, "parameter") || strings.Contains(bodyStr, "endpoint")) {
			capabilitiesMu.Lock()
			toolUseCache[p.URL+"|"+p.model] = false
			capabilitiesMu.Unlock()
			isToolUseDisabled = true

			reqBody.Tools = nil
			reqBody.ToolChoice = nil
			reqBody.Messages = sanitizeMessagesForTextOnly(reqMessages)

			jsonData, err = json.Marshal(reqBody)
			if err != nil {
				return nil, fmt.Errorf("openai: erro ao serializar request de retry stream: %w", err)
			}
			req, err = http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
			if err != nil {
				return nil, fmt.Errorf("openai: erro ao criar request de retry stream: %w", err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+p.apiKey)
			req.Header.Set("Accept", "text/event-stream")

			respRetry, err := client.Do(req)
			if err != nil {
				return nil, fmt.Errorf("openai: falha na requisição de retry stream: %w", err)
			}
			defer respRetry.Body.Close()

			if respRetry.StatusCode != http.StatusOK {
				retryBytes, _ := io.ReadAll(respRetry.Body)
				return nil, fmt.Errorf("openai: retry stream failed (%d): %s", respRetry.StatusCode, string(retryBytes))
			}
			resp = respRetry
		} else {
			return nil, fmt.Errorf("openai: status HTTP inválido (%d) (stream): %s", resp.StatusCode, string(bodyBytes))
		}
	}

	if len(opts.Tools) > 0 && !isToolUseDisabled {
		capabilitiesMu.Lock()
		toolUseCache[p.URL+"|"+p.model] = true
		capabilitiesMu.Unlock()
	}

	// Parsing the stream
	scanner := bufio.NewScanner(resp.Body)
	
	var fullContent strings.Builder
	var toolCallsMap = make(map[int]*llm.ToolCall)
	
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == "data: [DONE]" {
			break
		}
		if strings.HasPrefix(line, "data: ") {
			jsonStr := strings.TrimPrefix(line, "data: ")
			var chunk openAIStreamResponse
			if err := json.Unmarshal([]byte(jsonStr), &chunk); err != nil {
				continue // ignore parse errors on chunks
			}
			
			if len(chunk.Choices) > 0 {
				delta := chunk.Choices[0].Delta
				
				if delta.Content != "" {
					fullContent.WriteString(delta.Content)
					chunkChan <- delta.Content
				}
				
				for _, tc := range delta.ToolCalls {
					if toolCallsMap[tc.Index] == nil {
						toolCallsMap[tc.Index] = &llm.ToolCall{
							ID:   tc.ID,
							Type: tc.Type,
							Function: llm.FunctionCall{
								Name:      tc.Function.Name,
								Arguments: tc.Function.Arguments,
							},
						}
					} else {
						// Append to existing
						toolCallsMap[tc.Index].Function.Arguments += tc.Function.Arguments
					}
				}
			}
		}
	}
	
	if err := scanner.Err(); err != nil && err != io.EOF {
		return nil, fmt.Errorf("openai: erro ao ler stream: %w", err)
	}
	
	var finalToolCalls []llm.ToolCall
	for i := 0; i < len(toolCallsMap); i++ {
		if tc, exists := toolCallsMap[i]; exists {
			finalToolCalls = append(finalToolCalls, *tc)
		}
	}

	return &llm.Response{
		Message: llm.Message{
			Role:      "assistant",
			Content:   fullContent.String(),
			ToolCalls: finalToolCalls,
		},
		Usage: llm.Usage{TotalTokens: 0}, // Stream APIs don't always provide usage without specific flags
		ToolUseDisabled: isToolUseDisabled,
	}, nil
}
