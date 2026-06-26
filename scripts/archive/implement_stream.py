import sys
import re

def implement():
    with open("internal/llm/providers/openai.go", "r") as f:
        content = f.read()

    # Update openAIRequest
    old_req = """	type openAIRequest struct {
		Model       string               `json:"model"`
		Messages    []openAIChatMessage  `json:"messages"`
		Tools       []llm.ToolDefinition `json:"tools,omitempty"`
		ToolChoice  interface{}          `json:"tool_choice,omitempty"`
		Temperature *float64             `json:"temperature,omitempty"`
		MaxTokens   *int                 `json:"max_tokens,omitempty"`
	}"""
    new_req = """	type openAIRequest struct {
		Model       string               `json:"model"`
		Messages    []openAIChatMessage  `json:"messages"`
		Tools       []llm.ToolDefinition `json:"tools,omitempty"`
		ToolChoice  interface{}          `json:"tool_choice,omitempty"`
		Temperature *float64             `json:"temperature,omitempty"`
		MaxTokens   *int                 `json:"max_tokens,omitempty"`
		Stream      bool                 `json:"stream,omitempty"`
	}"""
    content = content.replace(old_req, new_req)

    # Now append StreamMessages
    stream_func = """
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
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("openai: erro ao criar request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai: falha na requisição HTTP: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai: status HTTP inválido (%d): %s", resp.StatusCode, string(bodyBytes))
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
"""
    if "func (p *OpenAIProvider) StreamMessages" not in content:
        content += stream_func
    
    # ensure "bufio" is imported
    if '"bufio"' not in content:
        content = re.sub(r'import \(', 'import (\n\t"bufio"', content, count=1)
        
    with open("internal/llm/providers/openai.go", "w") as f:
        f.write(content)

implement()
