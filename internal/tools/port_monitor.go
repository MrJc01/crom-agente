package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// PortMonitorTool verifica se uma porta local está ativa
type PortMonitorTool struct {
	workspaceRoot string
}

// NewPortMonitorTool cria a ferramenta port_monitor
func NewPortMonitorTool(workspaceRoot string) *PortMonitorTool {
	return &PortMonitorTool{
		workspaceRoot: workspaceRoot,
	}
}

// ID retorna o identificador da ferramenta
func (t *PortMonitorTool) ID() string {
	return "port_monitor"
}

// Description retorna a descrição da ferramenta
func (t *PortMonitorTool) Description() string {
	return "Verifica se uma porta TCP específica está ativa e aceitando conexões no localhost."
}

// ParametersSchema define os parâmetros aceitos via JSON
func (t *PortMonitorTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"port": {
				"type": "integer",
				"description": "Número da porta local a ser verificada (ex: 8080)"
			},
			"timeout_ms": {
				"type": "integer",
				"description": "Tempo limite de conexão em milissegundos (opcional, padrão 1000)"
			}
		},
		"required": ["port"]
	}`)
}

// RequiresApproval indica que esta ferramenta pode rodar livremente
func (t *PortMonitorTool) RequiresApproval() bool {
	return false
}

// Execute verifica a porta usando net.DialTimeout
func (t *PortMonitorTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var input struct {
		Port      int `json:"port"`
		TimeoutMs int `json:"timeout_ms"`
	}

	if err := json.Unmarshal(args, &input); err != nil {
		return Result{Success: false, Error: "argumentos inválidos de JSON"}, nil
	}

	if input.Port <= 0 || input.Port > 65535 {
		return Result{Success: false, Error: fmt.Sprintf("porta inválida: %d", input.Port)}, nil
	}

	if input.TimeoutMs <= 0 {
		input.TimeoutMs = 1000 // Padrão 1 segundo
	}

	address := fmt.Sprintf("127.0.0.1:%d", input.Port)
	dialer := net.Dialer{Timeout: time.Duration(input.TimeoutMs) * time.Millisecond}

	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return Result{
			Success: false,
			Error:   fmt.Sprintf("porta %d fechada ou inacessível: %s", input.Port, err.Error()),
		}, nil
	}
	conn.Close()

	return Result{
		Success: true,
		Data:    fmt.Sprintf("porta %d está aberta e aceitando conexões", input.Port),
	}, nil
}
