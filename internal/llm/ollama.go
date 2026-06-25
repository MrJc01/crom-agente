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

type OllamaProvider struct {
	endpoint string
	model    string
}

func NewOllamaProvider(endpoint, model string) *OllamaProvider {
	if endpoint == "" {
		endpoint = "http://localhost:11434"
	}
	if model == "" {
		model = "llama3"
	}
	return &OllamaProvider{
		endpoint: endpoint,
		model:    model,
	}
}

func (p *OllamaProvider) Name() string {
	return "ollama"
}

func (p *OllamaProvider) SendMessages(ctx context.Context, messages []Message, opts RequestOptions) (*Response, error) {
	url := p.endpoint + "/api/chat"

	type ollamaFunctionCall struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}

	type ollamaToolCall struct {
		ID       string             `json:"id"`
		Type     string             `json:"type"`
		Function ollamaFunctionCall `json:"function"`
	}

	type ollamaChatMessage struct {
		Role      string           `json:"role"`
		Content   string           `json:"content,omitempty"`
		ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
	}

	// Detecta se é o modelo deepseek para contornar bug de template no Ollama
	isDeepSeek := strings.Contains(strings.ToLower(p.model), "deepseek")

	// Layer 2: Extrai o contexto escrito da mídia antes do envio
	injectedMessages := ExtractAndInjectMediaContext(ctx, messages, p.Name(), "", p.endpoint)

	reqMessages := make([]ollamaChatMessage, len(injectedMessages))
	for i, m := range injectedMessages {
		var tcs []ollamaToolCall
		content := m.Content
		if strings.HasPrefix(content, "image:base64:") {
			// Substitui o payload da imagem nativa pelo indicador textual se o modelo não for de visão,
			// mas como já injetamos a descrição do Layer 2 no Content, podemos omitir o base64 bruto.
			content = "[Imagem: Captura de tela processada]"
		}

		if isDeepSeek {
			if m.Role == "assistant" && len(m.ToolCalls) > 0 {
				var calls []string
				for _, tc := range m.ToolCalls {
					calls = append(calls, fmt.Sprintf("%s(%s)", tc.Function.Name, tc.Function.Arguments))
				}
				content += fmt.Sprintf("\n[Chamando ferramentas: %s]", strings.Join(calls, ", "))
			} else if m.Role == "tool" {
				reqMessages[i] = ollamaChatMessage{
					Role:    "user",
					Content: fmt.Sprintf("[Retorno da ferramenta %s: %s]", m.Name, m.Content),
				}
				continue
			}
		} else {
			for _, tc := range m.ToolCalls {
				argsBytes := []byte(tc.Function.Arguments)
				if len(argsBytes) == 0 {
					argsBytes = []byte("{}")
				}
				tcs = append(tcs, ollamaToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: ollamaFunctionCall{
						Name:      tc.Function.Name,
						Arguments: json.RawMessage(argsBytes),
					},
				})
			}
		}

		reqMessages[i] = ollamaChatMessage{
			Role:      m.Role,
			Content:   content,
			ToolCalls: tcs,
		}
	}

	type ollamaRequest struct {
		Model    string              `json:"model"`
		Messages []ollamaChatMessage `json:"messages"`
		Tools    []ToolDefinition    `json:"tools,omitempty"`
		Stream   bool                `json:"stream"`
	}

	reqTools := opts.Tools
	if isDeepSeek {
		reqTools = nil
	}

	reqBody := ollamaRequest{
		Model:    p.model,
		Messages: reqMessages,
		Tools:    reqTools,
		Stream:   false,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("ollama: erro ao serializar request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("ollama: erro ao criar request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama: falha na requisição HTTP: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ollama: erro ao ler response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama: status HTTP inválido (%d): %s", resp.StatusCode, string(bodyBytes))
	}

	type ollamaResponse struct {
		Message struct {
			Role      string           `json:"role"`
			Content   string           `json:"content"`
			ToolCalls []ollamaToolCall `json:"tool_calls"`
		} `json:"message"`
		Done            bool `json:"done"`
		PromptEvalCount int  `json:"prompt_eval_count"`
		EvalCount       int  `json:"eval_count"`
	}

	var apiResp ollamaResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("ollama: erro ao parsear response: %w", err)
	}

	var respTcs []ToolCall
	for _, tc := range apiResp.Message.ToolCalls {
		respTcs = append(respTcs, ToolCall{
			ID:   tc.ID,
			Type: tc.Type,
			Function: FunctionCall{
				Name:      tc.Function.Name,
				Arguments: string(tc.Function.Arguments),
			},
		})
	}

	return &Response{
		Message: Message{
			Role:      apiResp.Message.Role,
			Content:   apiResp.Message.Content,
			ToolCalls: respTcs,
		},
		Usage: Usage{
			PromptTokens:     apiResp.PromptEvalCount,
			CompletionTokens: apiResp.EvalCount,
			TotalTokens:      apiResp.PromptEvalCount + apiResp.EvalCount,
		},
	}, nil
}
