package providers

import (
	"context"
	"fmt"

	"github.com/crom/crom-agente/internal/llm"
)

// MockResponse define uma resposta programável para o provider mock
type MockResponse struct {
	Response *llm.Response
	Err      error
}

// MockProvider é um provedor de LLM para testes automatizados offline
// Ele responde com respostas pré-programadas em sequência
type MockProvider struct {
	responses           []MockResponse
	callIndex           int
	CallLog             [][]llm.Message // Registra todas as chamadas recebidas para inspeção nos testes
	DisableSystemPrompt bool            // Para simular a falta de suporte a System Prompt nos testes
	MockCapabilities    *llm.ModelCapabilities
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
		CallLog:   make([][]llm.Message, 0),
	}
}

// SendMessages retorna a próxima resposta pré-programada na fila
func (m *MockProvider) SendMessages(ctx context.Context, messages []llm.Message, opts llm.RequestOptions) (*llm.Response, error) {
	// Registra a chamada
	msgCopy := make([]llm.Message, len(messages))
	copy(msgCopy, messages)
	m.CallLog = append(m.CallLog, msgCopy)

	if m.callIndex >= len(m.responses) {
		m.callIndex = len(m.responses) - 1
	}

	resp := m.responses[m.callIndex]
	m.callIndex++
	return resp.Response, resp.Err
}

// Name retorna o nome do provedor mock
func (m *MockProvider) Name() string {
	return "mock"
}

// SupportsSystemPrompt retorna se o mock suporta System Prompt (baseado no campo DisableSystemPrompt)
func (m *MockProvider) SupportsSystemPrompt() bool {
	return !m.DisableSystemPrompt
}

// Capabilities retorna as capacidades mockadas ou capacidades padrão
func (m *MockProvider) Capabilities() llm.ModelCapabilities {
	if m.MockCapabilities != nil {
		return *m.MockCapabilities
	}
	return llm.ModelCapabilities{
		ToolUse:          true,
		Vision:           true,
		MaxContext:       128000,
		StreamingSupport: true,
	}
}

// TotalCalls retorna o número total de chamadas feitas ao provider
func (m *MockProvider) TotalCalls() int {
	return len(m.CallLog)
}

// --- Funções auxiliares para construir respostas mock facilmente ---

// MockTextResponse cria uma resposta mock com apenas texto (sem tool calls)
func MockTextResponse(content string, tokens int) MockResponse {
	return MockResponse{
		Response: &llm.Response{
			Message: llm.Message{
				Role:    "assistant",
				Content: content,
			},
			Usage: llm.Usage{TotalTokens: tokens},
		},
	}
}

// MockToolCallResponse cria uma resposta mock com uma chamada de ferramenta
func MockToolCallResponse(toolName string, toolArgs string, tokens int) MockResponse {
	return MockResponse{
		Response: &llm.Response{
			Message: llm.Message{
				Role: "assistant",
				ToolCalls: []llm.ToolCall{
					{
						ID:   fmt.Sprintf("call_%s", toolName),
						Type: "function",
						Function: llm.FunctionCall{
							Name:      toolName,
							Arguments: toolArgs,
						},
					},
				},
			},
			Usage: llm.Usage{TotalTokens: tokens},
		},
	}
}

// MockEmptyResponse cria uma resposta mock completamente vazia (sem texto nem tool calls)
func MockEmptyResponse() MockResponse {
	return MockResponse{
		Response: &llm.Response{
			Message: llm.Message{Role: "assistant"},
			Usage:   llm.Usage{TotalTokens: 5},
		},
	}
}

// MockErrorResponse cria uma resposta mock que retorna um erro
func MockErrorResponse(errMsg string) MockResponse {
	return MockResponse{
		Err: fmt.Errorf("%s", errMsg),
	}
}
