package llm

import (
	"context"
	"fmt"
)

// MockResponse define uma resposta programável para o provider mock
type MockResponse struct {
	Response *Response
	Err      error
}

// MockProvider é um provedor de LLM para testes automatizados offline
// Ele responde com respostas pré-programadas em sequência
type MockProvider struct {
	responses []MockResponse
	callIndex int
	CallLog   [][]Message // Registra todas as chamadas recebidas para inspeção nos testes
}

// NewMockProvider cria um provider mock com respostas pré-programadas
func NewMockProvider(responses ...MockResponse) *MockProvider {
	if len(responses) == 0 {
		responses = []MockResponse{
			MockTextResponse("Tarefa concluida com sucesso.", 10),
		}
	}
	return &MockProvider{
		responses: responses,
		CallLog:   make([][]Message, 0),
	}
}

// SendMessages retorna a próxima resposta pré-programada na fila
func (m *MockProvider) SendMessages(ctx context.Context, messages []Message, opts RequestOptions) (*Response, error) {
	// Registra a chamada
	msgCopy := make([]Message, len(messages))
	copy(msgCopy, messages)
	m.CallLog = append(m.CallLog, msgCopy)

	if m.callIndex >= len(m.responses) {
		return nil, fmt.Errorf("mock provider: sem respostas restantes (chamada #%d)", m.callIndex+1)
	}

	resp := m.responses[m.callIndex]
	m.callIndex++
	return resp.Response, resp.Err
}

// Name retorna o nome do provedor mock
func (m *MockProvider) Name() string {
	return "mock"
}

// TotalCalls retorna o número total de chamadas feitas ao provider
func (m *MockProvider) TotalCalls() int {
	return len(m.CallLog)
}

// --- Funções auxiliares para construir respostas mock facilmente ---

// MockTextResponse cria uma resposta mock com apenas texto (sem tool calls)
func MockTextResponse(content string, tokens int) MockResponse {
	return MockResponse{
		Response: &Response{
			Message: Message{
				Role:    "assistant",
				Content: content,
			},
			Usage: Usage{TotalTokens: tokens},
		},
	}
}

// MockToolCallResponse cria uma resposta mock com uma chamada de ferramenta
func MockToolCallResponse(toolName string, toolArgs string, tokens int) MockResponse {
	return MockResponse{
		Response: &Response{
			Message: Message{
				Role: "assistant",
				ToolCalls: []ToolCall{
					{
						ID:   fmt.Sprintf("call_%s", toolName),
						Type: "function",
						Function: FunctionCall{
							Name:      toolName,
							Arguments: toolArgs,
						},
					},
				},
			},
			Usage: Usage{TotalTokens: tokens},
		},
	}
}

// MockEmptyResponse cria uma resposta mock completamente vazia (sem texto nem tool calls)
func MockEmptyResponse() MockResponse {
	return MockResponse{
		Response: &Response{
			Message: Message{Role: "assistant"},
			Usage:   Usage{TotalTokens: 5},
		},
	}
}

// MockErrorResponse cria uma resposta mock que retorna um erro
func MockErrorResponse(errMsg string) MockResponse {
	return MockResponse{
		Err: fmt.Errorf("%s", errMsg),
	}
}
