package manage_plan

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/crom/crom-agente/internal/loop"
	"github.com/crom/crom-agente/internal/state"
	"github.com/crom/crom-agente/internal/tools"
)

//go:embed metadata.json
var metadataJSON []byte

var metadata tools.ToolMetadata

func init() {
	var err error
	metadata, err = tools.ParseMetadata(metadataJSON)
	if err != nil {
		panic("falha ao carregar metadados de manage_plan: " + err.Error())
	}
}

// ManagePlanTool permite gerenciar o plano de trabalho estruturado do agente
type ManagePlanTool struct {
	workspaceRoot string
	stateManager  *state.StateManager
}

// NewManagePlanTool cria a ferramenta manage_plan
func NewManagePlanTool(workspaceRoot string, sm *state.StateManager) *ManagePlanTool {
	return &ManagePlanTool{
		workspaceRoot: workspaceRoot,
		stateManager:  sm,
	}
}

// ID retorna o identificador da ferramenta
func (t *ManagePlanTool) ID() string {
	return metadata.ID
}

// Description retorna a descrição da ferramenta
func (t *ManagePlanTool) Description() string {
	return metadata.Description
}

// ParametersSchema define os parâmetros aceitos via JSON
func (t *ManagePlanTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {
				"type": "string",
				"enum": ["create", "update", "get"],
				"description": "A ação a ser executada: 'create' para definir um novo plano (substitui o existente), 'update' para atualizar status de tarefas existentes, 'get' para consultar o plano atual."
			},
			"items": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"title": {
							"type": "string",
							"description": "Título da tarefa."
						},
						"status": {
							"type": "string",
							"enum": ["pending", "in_progress", "completed"],
							"description": "Status da tarefa."
						}
					},
					"required": ["title", "status"]
				},
				"description": "Lista de tarefas. Obrigatório para 'create' e 'update'."
			}
		},
		"required": ["action"]
	}`)
}

// RequiresApproval indica se a ferramenta necessita de aprovação
func (t *ManagePlanTool) RequiresApproval() bool {
	return false
}

// planResponse é o formato padronizado de resposta JSON da ferramenta
type planResponse struct {
	Success bool             `json:"success"`
	Action  string           `json:"action"`
	Message string           `json:"message"`
	Plan    []state.TaskItem `json:"plan"`
	Stats   planStats        `json:"stats"`
}

type planStats struct {
	Total      int `json:"total"`
	Pending    int `json:"pending"`
	InProgress int `json:"in_progress"`
	Completed  int `json:"completed"`
}

func computeStats(plan []state.TaskItem) planStats {
	s := planStats{Total: len(plan)}
	for _, item := range plan {
		switch item.Status {
		case "pending":
			s.Pending++
		case "in_progress":
			s.InProgress++
		case "completed":
			s.Completed++
		}
	}
	return s
}

func formatResponse(resp planResponse) string {
	data, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"success": false, "error": "%s"}`, err.Error())
	}
	return string(data)
}

// Execute executa a chamada
func (t *ManagePlanTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		Action string `json:"action"`
		Items  []struct {
			Title  string `json:"title"`
			Status string `json:"status"`
		} `json:"items"`
	}

	if err := json.Unmarshal(args, &input); err != nil {
		resp := planResponse{
			Success: false,
			Action:  "unknown",
			Message: "argumentos JSON inválidos: " + err.Error(),
		}
		return tools.Result{Success: false, Data: formatResponse(resp)}, nil
	}

	if t.stateManager == nil {
		resp := planResponse{
			Success: false,
			Action:  input.Action,
			Message: "state manager não disponível",
		}
		return tools.Result{Success: false, Data: formatResponse(resp)}, nil
	}

	switch strings.ToLower(input.Action) {
	case "get":
		plan := t.stateManager.GetPlan()
		resp := planResponse{
			Success: true,
			Action:  "get",
			Message: fmt.Sprintf("Plano atual com %d tarefas.", len(plan)),
			Plan:    plan,
			Stats:   computeStats(plan),
		}
		return tools.Result{Success: true, Data: formatResponse(resp)}, nil

	case "create":
		if len(input.Items) == 0 {
			resp := planResponse{
				Success: false,
				Action:  "create",
				Message: "nenhuma tarefa fornecida para o plano",
			}
			return tools.Result{Success: false, Data: formatResponse(resp)}, nil
		}

		var plan []state.TaskItem
		for _, item := range input.Items {
			status := item.Status
			if status == "" {
				status = "pending"
			}
			plan = append(plan, state.TaskItem{
				Title:  item.Title,
				Status: status,
			})
		}

		_ = t.stateManager.SetPlan(plan)
		_ = loop.WritePlanToFile(t.stateManager, plan)
		_ = loop.WriteTaskMdToSession(t.stateManager, plan)

		resp := planResponse{
			Success: true,
			Action:  "create",
			Message: fmt.Sprintf("Plano criado com %d tarefas.", len(plan)),
			Plan:    plan,
			Stats:   computeStats(plan),
		}
		return tools.Result{Success: true, Data: formatResponse(resp)}, nil

	case "update":
		if len(input.Items) == 0 {
			resp := planResponse{
				Success: false,
				Action:  "update",
				Message: "nenhuma tarefa fornecida para atualização",
			}
			return tools.Result{Success: false, Data: formatResponse(resp)}, nil
		}

		currentPlan := t.stateManager.GetPlan()
		if len(currentPlan) == 0 {
			resp := planResponse{
				Success: false,
				Action:  "update",
				Message: "não há plano existente para atualizar; use 'create' primeiro",
			}
			return tools.Result{Success: false, Data: formatResponse(resp)}, nil
		}

		// Mapeia por título exato e normalizado
		exactMap := make(map[string]int)
		normMap := make(map[string]int)
		for idx, item := range currentPlan {
			exactMap[strings.ToLower(item.Title)] = idx
			normMap[loop.NormalizeTitle(item.Title)] = idx
		}

		updatedCount := 0
		addedCount := 0
		for _, item := range input.Items {
			exactKey := strings.ToLower(item.Title)
			normKey := loop.NormalizeTitle(item.Title)

			idx := -1
			if i, exists := exactMap[exactKey]; exists {
				idx = i
			} else if i, exists := normMap[normKey]; exists {
				idx = i
			}

			status := item.Status
			if status == "" {
				status = "pending"
			}

			if idx >= 0 {
				// Atualiza via manage_plan permite regressão (é explícito)
				if currentPlan[idx].Status != status {
					currentPlan[idx].Status = status
					updatedCount++
				}
			} else {
				currentPlan = append(currentPlan, state.TaskItem{
					Title:  item.Title,
					Status: status,
				})
				newIdx := len(currentPlan) - 1
				exactMap[exactKey] = newIdx
				normMap[normKey] = newIdx
				addedCount++
			}
		}

		_ = t.stateManager.SetPlan(currentPlan)
		_ = loop.WritePlanToFile(t.stateManager, currentPlan)
		_ = loop.WriteTaskMdToSession(t.stateManager, currentPlan)

		resp := planResponse{
			Success: true,
			Action:  "update",
			Message: fmt.Sprintf("Plano atualizado: %d atualizadas, %d adicionadas.", updatedCount, addedCount),
			Plan:    currentPlan,
			Stats:   computeStats(currentPlan),
		}
		return tools.Result{Success: true, Data: formatResponse(resp)}, nil

	default:
		resp := planResponse{
			Success: false,
			Action:  input.Action,
			Message: fmt.Sprintf("ação desconhecida: '%s'. Use 'create', 'update' ou 'get'.", input.Action),
		}
		return tools.Result{Success: false, Data: formatResponse(resp)}, nil
	}
}
