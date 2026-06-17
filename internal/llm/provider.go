package llm

import "context"

// Message representa uma mensagem no histórico de conversação do agente
type Message struct {
	Role       string     `json:"role"`                  // "system", "user", "assistant", "tool"
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`         // Nome da ferramenta (quando role=tool)
}

// ToolCall representa uma chamada de ferramenta solicitada pelo LLM
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"` // "function"
	Function FunctionCall `json:"function"`
}

// FunctionCall contém o nome e os argumentos da função chamada
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string dos argumentos
}

// ToolDefinition descreve uma ferramenta disponível para o LLM
type ToolDefinition struct {
	Type     string             `json:"type"` // "function"
	Function ToolFunctionSchema `json:"function"`
}

// ToolFunctionSchema descreve o schema de uma ferramenta
type ToolFunctionSchema struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"`
}

// Usage contém informações de consumo de tokens da resposta
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Response representa a resposta completa de uma chamada ao LLM
type Response struct {
	Message Message `json:"message"`
	Usage   Usage   `json:"usage"`
}

// RequestOptions contém opções para a chamada ao LLM
type RequestOptions struct {
	Tools      []ToolDefinition `json:"tools,omitempty"`
	ToolChoice string           `json:"tool_choice,omitempty"` // "auto", "none"
}

// Provider é a interface de abstração para qualquer provedor de LLM
// Implementações concretas: OpenAI, Gemini, Anthropic, Ollama, Mock
type Provider interface {
	// SendMessages envia mensagens ao LLM e retorna a resposta
	SendMessages(ctx context.Context, messages []Message, opts RequestOptions) (*Response, error)

	// Name retorna o nome do provedor (para logs)
	Name() string
}
