package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// ScheduleTimerTool permite que o agente agende um timer para chamar a si mesmo no futuro
type ScheduleTimerTool struct {
	workspaceRoot string
	scheduleFunc  func(task string, durationSeconds int)
}

// NewScheduleTimerTool cria a ferramenta schedule_timer
func NewScheduleTimerTool(workspaceRoot string, scheduleFunc func(task string, durationSeconds int)) *ScheduleTimerTool {
	return &ScheduleTimerTool{
		workspaceRoot: workspaceRoot,
		scheduleFunc:  scheduleFunc,
	}
}

// ID retorna o identificador da ferramenta
func (t *ScheduleTimerTool) ID() string {
	return "schedule_timer"
}

// Description retorna a descrição da ferramenta
func (t *ScheduleTimerTool) Description() string {
	return "Agenda a execução de uma nova tarefa (ou a continuidade da tarefa atual) para este agente após um intervalo de tempo em segundos. Útil para verificar resultados de processos em background ou servidores locais após uma espera."
}

// ParametersSchema define os parâmetros aceitos via JSON
func (t *ScheduleTimerTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"duration_seconds": {
				"type": "integer",
				"description": "O tempo de espera em segundos antes de acionar o agente novamente"
			},
			"task": {
				"type": "string",
				"description": "O prompt ou instrução a ser executado quando o timer expirar (ex: 'Verifique se o servidor local na porta 8000 já responde a requisições')"
			}
		},
		"required": ["duration_seconds", "task"]
	}`)
}

// RequiresApproval indica se a ferramenta necessita de aprovação do usuário (HITL)
func (t *ScheduleTimerTool) RequiresApproval() bool {
	return true // Requer confirmação para agendar o auto-despertar
}

// Execute agenda o timer chamando a função injetada
func (t *ScheduleTimerTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var input struct {
		DurationSeconds int    `json:"duration_seconds"`
		Task            string `json:"task"`
	}

	if err := json.Unmarshal(args, &input); err != nil {
		return Result{Success: false, Error: "argumentos inválidos de JSON"}, nil
	}

	if input.DurationSeconds <= 0 {
		return Result{Success: false, Error: "a duração em segundos deve ser maior que zero"}, nil
	}

	if input.Task == "" {
		return Result{Success: false, Error: "a tarefa/prompt não pode estar vazia"}, nil
	}

	if t.scheduleFunc == nil {
		return Result{Success: false, Error: "serviço de agendamento não configurado no daemon"}, nil
	}

	// Executa a função de agendamento em background
	t.scheduleFunc(input.Task, input.DurationSeconds)

	return Result{
		Success: true,
		Data:    fmt.Sprintf("Timer agendado com sucesso para rodar em %d segundos com o prompt: %q", input.DurationSeconds, input.Task),
	}, nil
}
