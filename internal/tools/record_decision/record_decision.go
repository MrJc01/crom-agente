package record_decision

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/crom/crom-agente/internal/tools"
)

//go:embed metadata.json
var metadataJSON []byte

var metadata tools.ToolMetadata

func init() {
	var err error
	metadata, err = tools.ParseMetadata(metadataJSON)
	if err != nil {
		panic("falha ao carregar metadados de record_decision: " + err.Error())
	}
}

// RecordDecisionTool grava decisões no decisions.log compartilhado
type RecordDecisionTool struct {
	workspaceRoot string
}

// NewRecordDecisionTool cria a ferramenta record_decision
func NewRecordDecisionTool(workspaceRoot string) *RecordDecisionTool {
	return &RecordDecisionTool{
		workspaceRoot: workspaceRoot,
	}
}

// ID retorna o identificador da ferramenta
func (t *RecordDecisionTool) ID() string {
	return metadata.ID
}

// Description retorna a descrição da ferramenta
func (t *RecordDecisionTool) Description() string {
	return metadata.Description
}

// ParametersSchema define os parâmetros aceitos via JSON
func (t *RecordDecisionTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"decision": {
				"type": "string",
				"description": "Texto contendo a decisão de arquitetura, conclusão de teste ou constatação lógica importante a ser registrada."
			}
		},
		"required": ["decision"]
	}`)
}

// RequiresApproval indica que esta ferramenta não precisa de aprovação HITL (só grava log)
func (t *RecordDecisionTool) RequiresApproval() bool {
	return false
}

// Execute realiza a gravação no decisions.log
func (t *RecordDecisionTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		Decision string `json:"decision"`
	}

	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Result{Success: false, Error: "argumentos inválidos de JSON"}, nil
	}

	if input.Decision == "" {
		return tools.Result{Success: false, Error: "a decisão não pode ser vazia"}, nil
	}

	logDir := filepath.Join(t.workspaceRoot, ".crom")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return tools.Result{Success: false, Error: "falha ao criar diretório .crom: " + err.Error()}, nil
	}

	logPath := filepath.Join(logDir, "decisions.log")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return tools.Result{Success: false, Error: "falha ao abrir arquivo decisions.log: " + err.Error()}, nil
	}
	defer f.Close()

	timestamp := time.Now().Format(time.RFC3339)
	logEntry := fmt.Sprintf("[%s] %s\n", timestamp, input.Decision)

	if _, err := f.WriteString(logEntry); err != nil {
		return tools.Result{Success: false, Error: "falha ao escrever no log: " + err.Error()}, nil
	}

	return tools.Result{
		Success: true,
		Data:    fmt.Sprintf("decisão registrada com sucesso no log de raciocínio compartilhado: %q", input.Decision),
	}, nil
}
