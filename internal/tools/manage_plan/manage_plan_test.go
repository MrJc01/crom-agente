package manage_plan

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crom/crom-agente/internal/state"
)

func TestManagePlan_Get(t *testing.T) {
	ws := t.TempDir()
	sm := state.NewStateManager(ws)
	_ = sm.LoadState()

	tool := NewManagePlanTool(ws, sm)

	// Plano vazio inicialmente
	resp, err := tool.Execute(context.Background(), json.RawMessage(`{"action": "get"}`))
	if err != nil {
		t.Fatalf("falha ao executar get: %v", err)
	}

	if !resp.Success {
		t.Fatalf("get falhou: %s", resp.Data)
	}

	var data planResponse
	if err := json.Unmarshal([]byte(resp.Data), &data); err != nil {
		t.Fatalf("erro ao unmarshal resposta: %v", err)
	}

	if len(data.Plan) != 0 || data.Stats.Total != 0 {
		t.Errorf("esperava plano vazio, obteve %d tarefas", len(data.Plan))
	}
}

func TestManagePlan_Create(t *testing.T) {
	ws := t.TempDir()
	sm := state.NewStateManager(ws)
	_ = sm.LoadState()

	tool := NewManagePlanTool(ws, sm)

	args := `{
		"action": "create",
		"items": [
			{"title": "Task One", "status": "pending"},
			{"title": "Task Two", "status": "in_progress"},
			{"title": "Task Three", "status": "completed"}
		]
	}`

	resp, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("falha ao executar create: %v", err)
	}

	if !resp.Success {
		t.Fatalf("create falhou: %s", resp.Data)
	}

	var data planResponse
	if err := json.Unmarshal([]byte(resp.Data), &data); err != nil {
		t.Fatalf("erro ao unmarshal resposta: %v", err)
	}

	if len(data.Plan) != 3 {
		t.Errorf("esperava 3 tarefas, obteve %d", len(data.Plan))
	}

	if data.Stats.Total != 3 || data.Stats.Pending != 1 || data.Stats.InProgress != 1 || data.Stats.Completed != 1 {
		t.Errorf("estatísticas incorretas: %+v", data.Stats)
	}

	// Verifica se persistiu fisicamente em plan.md no workspace
	planMdPath := filepath.Join(ws, "plan.md")
	content, err := os.ReadFile(planMdPath)
	if err != nil {
		t.Fatalf("não conseguiu ler plan.md: %v", err)
	}

	strContent := string(content)
	if !strings.Contains(strContent, "- [ ] Task One") ||
		!strings.Contains(strContent, "- [/] Task Two") ||
		!strings.Contains(strContent, "- [x] Task Three") {
		t.Errorf("plan.md com conteúdo inválido:\n%s", strContent)
	}
}

func TestManagePlan_Update(t *testing.T) {
	ws := t.TempDir()
	sm := state.NewStateManager(ws)
	_ = sm.LoadState()

	tool := NewManagePlanTool(ws, sm)

	// Cria primeiro
	createArgs := `{
		"action": "create",
		"items": [
			{"title": "Task One", "status": "pending"},
			{"title": "Task Two", "status": "pending"}
		]
	}`
	_, _ = tool.Execute(context.Background(), json.RawMessage(createArgs))

	// Atualiza
	updateArgs := `{
		"action": "update",
		"items": [
			{"title": "Task One", "status": "completed"},
			{"title": "Task Three", "status": "in_progress"}
		]
	}`

	resp, err := tool.Execute(context.Background(), json.RawMessage(updateArgs))
	if err != nil {
		t.Fatalf("falha ao executar update: %v", err)
	}

	if !resp.Success {
		t.Fatalf("update falhou: %s", resp.Data)
	}

	var data planResponse
	if err := json.Unmarshal([]byte(resp.Data), &data); err != nil {
		t.Fatalf("erro ao unmarshal resposta: %v", err)
	}

	if len(data.Plan) != 3 {
		t.Errorf("esperava 3 tarefas no total (2 originais + 1 nova), obteve %d", len(data.Plan))
	}

	// Verifica status atualizados
	for _, item := range data.Plan {
		switch item.Title {
		case "Task One":
			if item.Status != "completed" {
				t.Errorf("Task One deveria estar completed, mas está %q", item.Status)
			}
		case "Task Two":
			if item.Status != "pending" {
				t.Errorf("Task Two deveria estar pending, mas está %q", item.Status)
			}
		case "Task Three":
			if item.Status != "in_progress" {
				t.Errorf("Task Three deveria estar in_progress, mas está %q", item.Status)
			}
		}
	}
}

func TestManagePlan_UpdateWithNormalization(t *testing.T) {
	ws := t.TempDir()
	sm := state.NewStateManager(ws)
	_ = sm.LoadState()

	tool := NewManagePlanTool(ws, sm)

	// Cria primeiro
	createArgs := `{
		"action": "create",
		"items": [
			{"title": "Criar arquivo principal main.go", "status": "pending"}
		]
	}`
	_, _ = tool.Execute(context.Background(), json.RawMessage(createArgs))

	// Atualiza usando título com stop-words diferentes mas mesmo significado normalizado
	updateArgs := `{
		"action": "update",
		"items": [
			{"title": "Fazer o arquivo principal main.go", "status": "completed"}
		]
	}`

	resp, err := tool.Execute(context.Background(), json.RawMessage(updateArgs))
	if err != nil {
		t.Fatalf("falha no update: %v", err)
	}

	var data planResponse
	if err := json.Unmarshal([]byte(resp.Data), &data); err != nil {
		t.Fatalf("erro no unmarshal: %v", err)
	}

	if len(data.Plan) != 1 {
		t.Fatalf("deveria ter de-duplicado por match normalizado e mantido apenas 1 tarefa, obteve %d: %+v", len(data.Plan), data.Plan)
	}

	if data.Plan[0].Status != "completed" {
		t.Errorf("a tarefa não foi atualizada com sucesso pelo título normalizado, status: %q", data.Plan[0].Status)
	}
}

func TestManagePlan_InvalidAction(t *testing.T) {
	ws := t.TempDir()
	sm := state.NewStateManager(ws)
	_ = sm.LoadState()

	tool := NewManagePlanTool(ws, sm)

	resp, err := tool.Execute(context.Background(), json.RawMessage(`{"action": "invalid_action_name"}`))
	if err != nil {
		t.Fatalf("falha ao executar: %v", err)
	}

	if resp.Success {
		t.Errorf("esperava Success=false para ação inválida, obteve true")
	}

	var data planResponse
	if err := json.Unmarshal([]byte(resp.Data), &data); err != nil {
		t.Fatalf("erro ao parsear resposta de erro: %v", err)
	}

	if data.Success {
		t.Errorf("campo success no JSON deveria ser false")
	}

	if !strings.Contains(data.Message, "ação desconhecida") {
		t.Errorf("mensagem de erro inválida: %q", data.Message)
	}
}
