package ask_user

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/crom/crom-agente/internal/tools"
)

//go:embed metadata.json
var metadataJSON []byte

var metadata tools.ToolMetadata

func init() {
	var err error
	metadata, err = tools.ParseMetadata(metadataJSON)
	if err != nil {
		panic("falha ao carregar metadados de ask_user: " + err.Error())
	}
}

// AskUserTool permite ao agente fazer perguntas estruturadas ao usuário e suspender a execução
type AskUserTool struct {
	workspaceRoot string
}

// NewAskUserTool cria a ferramenta ask_user
func NewAskUserTool(workspaceRoot string) *AskUserTool {
	return &AskUserTool{
		workspaceRoot: workspaceRoot,
	}
}

// ID retorna o identificador da ferramenta
func (t *AskUserTool) ID() string {
	return metadata.ID
}

// Description retorna a descrição da ferramenta
func (t *AskUserTool) Description() string {
	return metadata.Description
}

// ParametersSchema define os parâmetros aceitos via JSON
func (t *AskUserTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"question": {
				"type": "string",
				"description": "A pergunta ou pedido de esclarecimento a ser exibido para o usuário."
			},
			"options": {
				"type": "array",
				"items": {
					"type": "string"
				},
				"description": "Lista opcional de opções recomendadas/sugeridas pelo agente para escolha rápida."
			},
			"allow_custom": {
				"type": "boolean",
				"description": "Se verdadeiro (padrão), permite que o usuário escreva uma resposta personalizada mesmo que as opções estejam presentes.",
				"default": true
			}
		},
		"required": ["question"]
	}`)
}

// RequiresApproval indica se a ferramenta necessita de aprovação.
// Como o ask_user já é por definição uma ação de diálogo com o usuário que interrompe o fluxo cognitivo,
// RequiresApproval é false porque a suspensão é tratada na camada de orquestração.
func (t *AskUserTool) RequiresApproval() bool {
	return false
}

// Execute executa a chamada
func (t *AskUserTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var rawInput struct {
		Question    string      `json:"question"`
		Options     []string    `json:"options"`
		AllowCustom interface{} `json:"allow_custom"`
	}

	if err := json.Unmarshal(args, &rawInput); err != nil {
		return tools.Result{Success: false, Error: "argumentos inválidos de JSON"}, nil
	}

	allowCustom := true
	if rawInput.AllowCustom != nil {
		switch v := rawInput.AllowCustom.(type) {
		case bool:
			allowCustom = v
		case string:
			lower := strings.ToLower(strings.TrimSpace(v))
			allowCustom = (lower == "true" || lower == "yes" || lower == "1")
		}
	}
	_ = allowCustom

	if rawInput.Question == "" {
		return tools.Result{Success: false, Error: "a pergunta não pode estar vazia"}, nil
	}

	return tools.Result{
		Success: true,
		Data:    fmt.Sprintf("Pergunta enviada ao usuário: %q", rawInput.Question),
	}, nil
}
