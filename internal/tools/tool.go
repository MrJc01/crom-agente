package tools

import (
	"context"
	"encoding/json"
)

// Result representa o resultado da execução de uma ferramenta
type Result struct {
	Success bool   `json:"success"`
	Data    string `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Tool é a interface que toda ferramenta nativa ou dinâmica deve implementar
type Tool interface {
	// ID retorna o identificador único da ferramenta (ex: "read_file", "write_file")
	ID() string

	// Description retorna a descrição legível para injeção no prompt do LLM
	Description() string

	// ParametersSchema retorna o JSON Schema dos parâmetros aceitos
	ParametersSchema() json.RawMessage

	// RequiresApproval indica se a execução deve ser aprovada pelo usuário (HITL)
	RequiresApproval() bool

	// Execute executa a ferramenta com os argumentos fornecidos
	Execute(ctx context.Context, args json.RawMessage) (Result, error)
}
