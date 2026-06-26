package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/crom/crom-agente/internal/llm"
)

type AnthropicProvider struct {
	apiKey string
	model  string
}

func NewAnthropicProvider(apiKey, model string) *AnthropicProvider {
	if model == "" {
		model = "claude-3-5-sonnet-20241022"
	}
	return &AnthropicProvider{
		apiKey: apiKey,
		model:  model,
	}
}

func (p *AnthropicProvider) Name() string {
	return "anthropic"
}

func (p *AnthropicProvider) SupportsSystemPrompt() bool {
	return true
}

func (p *AnthropicProvider) Capabilities() llm.ModelCapabilities {
	return llm.GetCapabilities(p.model)
}

// Structs para API Anthropic Messages

type anthropicContent struct {
	Type      string                 `json:"type"`
	Text      string                 `json:"text,omitempty"`
	ID        string                 `json:"id,omitempty"`
	Name      string                 `json:"name,omitempty"`
	Input     map[string]interface{} `json:"input,omitempty"`
	ToolUseID string                 `json:"tool_use_id,omitempty"`
	Content   string                 `json:"content,omitempty"`
}

type anthropicMessage struct {
	Role    string             `json:"role"`
	Content []anthropicContent `json:"content"`
}

type anthropicTool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"input_schema"`
}

type anthropicRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	System      string             `json:"system,omitempty"`
	Messages    []anthropicMessage `json:"messages"`
	Tools       []anthropicTool    `json:"tools,omitempty"`
	Temperature *float64           `json:"temperature,omitempty"`
}

func (p *AnthropicProvider) SendMessages(ctx context.Context, messages []llm.Message, opts llm.RequestOptions) (*llm.Response, error) {
	url := "https://api.anthropic.com/v1/messages"

	var systemPrompt string
	anthropicMsgs := make([]anthropicMessage, 0)

	for _, m := range messages {
		if m.Role == "system" {
			if systemPrompt != "" {
				systemPrompt += "\n"
			}
			systemPrompt += m.Content
			continue
		}

		if m.Role == "tool" {
			// Resposta de ferramenta na Anthropic tem role = user e tipo = tool_result
			block := anthropicContent{
				Type:      "tool_result",
				ToolUseID: m.ToolCallID,
				Content:   m.Content,
			}
			anthropicMsgs = append(anthropicMsgs, anthropicMessage{
				Role:    "user",
				Content: []anthropicContent{block},
			})
			continue
		}

		if m.Role == "assistant" {
			blocks := make([]anthropicContent, 0)
			if m.Content != "" {
				blocks = append(blocks, anthropicContent{
					Type: "text",
					Text: m.Content,
				})
			}
			for _, tc := range m.ToolCalls {
				var inputArgs map[string]interface{}
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &inputArgs)

				blocks = append(blocks, anthropicContent{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Function.Name,
					Input: inputArgs,
				})
			}
			anthropicMsgs = append(anthropicMsgs, anthropicMessage{
				Role:    "assistant",
				Content: blocks,
			})
			continue
		}

		if m.Role == "user" {
			anthropicMsgs = append(anthropicMsgs, anthropicMessage{
				Role: "user",
				Content: []anthropicContent{
					{
						Type: "text",
						Text: m.Content,
					},
				},
			})
		}
	}

	reqBody := anthropicRequest{
		Model:       p.model,
		MaxTokens:   4000,
		System:      systemPrompt,
		Messages:    anthropicMsgs,
		Temperature: opts.Temperature,
	}

	if len(opts.Tools) > 0 {
		reqBody.Tools = make([]anthropicTool, len(opts.Tools))
		for i, t := range opts.Tools {
			reqBody.Tools[i] = anthropicTool{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				InputSchema: t.Function.Parameters,
			}
		}
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("anthropic: erro ao serializar request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("anthropic: erro ao criar request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic: falha na requisição HTTP: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("anthropic: erro ao ler response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic: status HTTP inválido (%d): %s", resp.StatusCode, string(bodyBytes))
	}

	type anthropicUsage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	}

	type anthropicResponse struct {
		Content []struct {
			Type  string                 `json:"type"`
			Text  string                 `json:"text,omitempty"`
			ID    string                 `json:"id,omitempty"`
			Name  string                 `json:"name,omitempty"`
			Input map[string]interface{} `json:"input,omitempty"`
		} `json:"content"`
		Usage anthropicUsage `json:"usage"`
	}

	var apiResp anthropicResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("anthropic: erro ao parsear response: %w", err)
	}

	// Converter resposta da Anthropic de volta para llm.Message
	var resContent string
	toolCalls := make([]llm.ToolCall, 0)

	for _, block := range apiResp.Content {
		if block.Type == "text" {
			resContent += block.Text
		} else if block.Type == "tool_use" {
			argBytes, _ := json.Marshal(block.Input)
			toolCalls = append(toolCalls, llm.ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: llm.FunctionCall{
					Name:      block.Name,
					Arguments: string(argBytes),
				},
			})
		}
	}

	return &llm.Response{
		Message: llm.Message{
			Role:      "assistant",
			Content:   resContent,
			ToolCalls: toolCalls,
		},
		Usage: llm.Usage{
			PromptTokens:     apiResp.Usage.InputTokens,
			CompletionTokens: apiResp.Usage.OutputTokens,
			TotalTokens:      apiResp.Usage.InputTokens + apiResp.Usage.OutputTokens,
		},
	}, nil
}

func (p *AnthropicProvider) StreamMessages(ctx context.Context, messages []llm.Message, opts llm.RequestOptions, chunkChan chan<- string) (*llm.Response, error) {
	defer close(chunkChan)
	// Fallback to non-streaming for now
	return p.SendMessages(ctx, messages, opts)
}
