package loop

import "time"

// AgentEvent é a struct unificada de evento que o agente emite em tempo real.
// Todos os consumidores (CLI, Daemon API, SDK) recebem esta struct serializada em JSON.
type AgentEvent struct {
	Timestamp time.Time              `json:"timestamp"`
	Event     string                 `json:"event"`               // "thinking", "tool_call", "tool_result", "message", "error", "finished"
	Iteration int                    `json:"iteration,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// AgentError é um erro tipado emitido pelo agente para tratamento programático.
type AgentError struct {
	Code    string                 `json:"code"`              // Ex: "ERR_LLM_RATE_LIMIT"
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// Códigos de erro padronizados para tratamento programático em qualquer cliente.
const (
	ErrLLMRateLimit     = "ERR_LLM_RATE_LIMIT"
	ErrLLMAuth          = "ERR_LLM_AUTHENTICATION"
	ErrLLMEmptyResponse = "ERR_LLM_EMPTY_RESPONSE"
	ErrToolNotFound     = "ERR_TOOL_NOT_FOUND"
	ErrToolExecution    = "ERR_TOOL_EXECUTION"
	ErrToolTimeout      = "ERR_TOOL_TIMEOUT"
	ErrPermissionDenied = "ERR_PERMISSION_DENIED"
	ErrSandboxViolation = "ERR_SANDBOX_VIOLATION"
	ErrMaxIterations    = "ERR_MAX_ITERATIONS"
	ErrConsecutiveFails = "ERR_CONSECUTIVE_FAILURES"
	ErrContextCanceled  = "ERR_CONTEXT_CANCELED"
)
