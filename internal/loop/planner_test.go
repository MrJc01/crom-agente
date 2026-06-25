package loop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crom/crom-agente/internal/llm"
	"github.com/crom/crom-agente/internal/state"
)

func TestParsePlan(t *testing.T) {
	content := `
	Aqui está o meu plano:
	- [ ] Primeira tarefa a fazer
	- [/] Tarefa que está em progresso
	* [x] Tarefa concluída com asterisco
	- [X] Outra concluída com X maiúsculo
	E algumas observações avulsas sem checkbox.
	`

	tasks := ParsePlan(content)
	if len(tasks) != 4 {
		t.Fatalf("esperava 4 tarefas extraídas, obteve %d", len(tasks))
	}

	if tasks[0].Title != "Primeira tarefa a fazer" || tasks[0].Status != "pending" {
		t.Fatalf("tarefa 0 inválida: %+v", tasks[0])
	}
	if tasks[1].Title != "Tarefa que está em progresso" || tasks[1].Status != "in_progress" {
		t.Fatalf("tarefa 1 inválida: %+v", tasks[1])
	}
	if tasks[2].Title != "Tarefa concluída com asterisco" || tasks[2].Status != "completed" {
		t.Fatalf("tarefa 2 inválida: %+v", tasks[2])
	}
	if tasks[3].Title != "Outra concluída com X maiúsculo" || tasks[3].Status != "completed" {
		t.Fatalf("tarefa 3 inválida: %+v", tasks[3])
	}
}

func TestUpdatePlannerAndSync(t *testing.T) {
	ws := t.TempDir()
	sm := state.NewStateManager(ws)
	_ = sm.LoadState()

	// 1. Mensagem inicial define plano
	msg1 := `Vamos iniciar o trabalho.
- [ ] Task 1
- [ ] Task 2`

	UpdatePlannerFromMessage(sm, msg1)
	plan := sm.GetPlan()
	if len(plan) != 2 {
		t.Fatalf("esperava 2 tarefas salvas, obteve %d", len(plan))
	}

	// 2. Mensagem atualiza progresso e adiciona nova task
	msg2 := `Atualizando:
- [x] Task 1
- [/] Task 2
- [ ] Task 3`

	UpdatePlannerFromMessage(sm, msg2)
	plan = sm.GetPlan()
	if len(plan) != 3 {
		t.Fatalf("esperava 3 tarefas salvas após atualização, obteve %d", len(plan))
	}

	if plan[0].Title != "Task 1" || plan[0].Status != "completed" {
		t.Fatalf("Task 1 não foi atualizada para concluída: %+v", plan[0])
	}
	if plan[1].Title != "Task 2" || plan[1].Status != "in_progress" {
		t.Fatalf("Task 2 não foi atualizada para em progresso: %+v", plan[1])
	}
	if plan[2].Title != "Task 3" || plan[2].Status != "pending" {
		t.Fatalf("Task 3 não foi adicionada: %+v", plan[2])
	}

	// 3. Sincroniza plano de volta no contexto
	ctxText := SyncPlanToContext(sm)
	if !strings.Contains(ctxText, "[PLANO DE TRABALHO ATUAL]") {
		t.Fatalf("contexto gerado inválido: %s", ctxText)
	}
	if !strings.Contains(ctxText, "- [x] Task 1") {
		t.Fatalf("contexto não contém status da Task 1: %s", ctxText)
	}
	if !strings.Contains(ctxText, "- [/] Task 2") {
		t.Fatalf("contexto não contém status da Task 2: %s", ctxText)
	}
	if !strings.Contains(ctxText, "- [ ] Task 3") {
		t.Fatalf("contexto não contém status da Task 3: %s", ctxText)
	}
}

func TestPlannerHelpers(t *testing.T) {
	ws := t.TempDir()
	sm := state.NewStateManager(ws)
	_ = sm.LoadState()

	// Initially plan is empty, phase should be planning
	if GetCurrentPhase(sm) != PhasePlanning {
		t.Errorf("expected PhasePlanning for empty plan")
	}

	plan := []state.TaskItem{
		{Title: "Task 1", Status: "pending"},
		{Title: "Task 2", Status: "in_progress"},
		{Title: "Task 3", Status: "completed"},
	}

	if !HasPendingTasks(plan) {
		t.Errorf("expected true for HasPendingTasks because of pending/in_progress tasks")
	}

	if HasPendingTasks([]state.TaskItem{{Title: "Task 3", Status: "completed"}}) {
		t.Errorf("expected false for HasPendingTasks when all tasks are completed")
	}

	warning := GeneratePendingTasksWarning(plan)
	if !strings.Contains(warning, "Task 1") || !strings.Contains(warning, "Task 2") || strings.Contains(warning, "Task 3") {
		t.Errorf("unexpected warning content: %s", warning)
	}

	_ = sm.SetPlan(plan)
	// Now phase should be Execution because we have an in_progress task
	if GetCurrentPhase(sm) != PhaseExecution {
		t.Errorf("expected PhaseExecution when tasks are in progress or completed")
	}

	err := WriteTaskMdToSession(sm, plan)
	if err != nil {
		t.Fatalf("failed to write task.md: %v", err)
	}

	taskMdPath := filepath.Join(filepath.Dir(sm.FilePath()), "task.md")
	data, err := os.ReadFile(taskMdPath)
	if err != nil {
		t.Fatalf("failed to read written task.md: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "- [ ] Task 1") || !strings.Contains(content, "- [/] Task 2") || !strings.Contains(content, "- [x] Task 3") {
		t.Errorf("task.md has invalid content: %s", content)
	}
}

func TestNormalizeTitle(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Criar o arquivo main.go", "arquivo main.go"},
		{"fazer testes de unidade", "testes unidade"},
		{"implementar a rota de login", "rota login"},
		{"Create new database connection", "new database connection"},
		{"Anotações e detalhes", "anotações detalhes"},
	}

	for _, tc := range tests {
		res := NormalizeTitle(tc.input)
		if res != tc.expected {
			t.Errorf("NormalizeTitle(%q) = %q; esperado %q", tc.input, res, tc.expected)
		}
	}
}

func TestUpdatePlannerRegressionGuard(t *testing.T) {
	ws := t.TempDir()
	sm := state.NewStateManager(ws)
	_ = sm.LoadState()

	// Define completed task
	plan1 := `
	- [x] Tarefa Importante
	`
	UpdatePlannerFromMessage(sm, plan1)
	
	// Tentativa de regredir para pending
	plan2 := `
	- [ ] Tarefa Importante
	`
	UpdatePlannerFromMessage(sm, plan2)

	plan := sm.GetPlan()
	if len(plan) != 1 || plan[0].Status != "completed" {
		t.Errorf("erro: tarefa regrediu status de completed para %q", plan[0].Status)
	}
}

func TestUpdatePlannerRegressionAllowed(t *testing.T) {
	ws := t.TempDir()
	sm := state.NewStateManager(ws)
	_ = sm.LoadState()

	plan1 := `
	- [x] Tarefa Importante
	`
	UpdatePlannerFromMessageWithConfig(sm, plan1, false)

	// Regressão permitida com disableCacheProtection = true
	plan2 := `
	- [ ] Tarefa Importante
	`
	UpdatePlannerFromMessageWithConfig(sm, plan2, true)

	plan := sm.GetPlan()
	if len(plan) != 1 || plan[0].Status != "pending" {
		t.Errorf("esperava status pending pós regressão permitida, obteve %q", plan[0].Status)
	}
}

func TestUpdatePlannerDeduplication(t *testing.T) {
	ws := t.TempDir()
	sm := state.NewStateManager(ws)
	_ = sm.LoadState()

	// Adiciona uma tarefa
	UpdatePlannerFromMessage(sm, "- [ ] Criar o arquivo principal main.go")

	// Adiciona tarefa similar que normaliza para o mesmo título ("arquivo principal main.go")
	UpdatePlannerFromMessage(sm, "- [ ] Fazer o arquivo principal main.go")

	plan := sm.GetPlan()
	if len(plan) != 1 {
		t.Errorf("esperava 1 tarefa por conta da de-duplicação, obteve %d: %+v", len(plan), plan)
	}
}

func TestParseExpectedFiles(t *testing.T) {
	fromLLM := []llm.Message{
		{
			Role:    "assistant",
			Content: `Para resolver isso vamos criar o arquivo [NEW] [main.go](file:///home/j/workspace/main.go)
			E também editar o arquivo [MODIFY] [utils/helper.py](file:///home/j/workspace/utils/helper.py)
			Ou simplesmente o link file:///home/j/workspace/config.json e deletar [DELETE] test.txt.
			`,
		},
	}

	files := ParseExpectedFiles(fromLLM)
	expected := []string{"main.go", "utils/helper.py", "/home/j/workspace/config.json"}
	
	hasFile := func(name string) bool {
		for _, f := range files {
			if strings.Contains(f, name) {
				return true
			}
		}
		return false
	}

	for _, exp := range expected {
		if !hasFile(exp) {
			t.Errorf("ParseExpectedFiles não extraiu o arquivo esperado: %s, extraídos: %v", exp, files)
		}
	}
}

func TestVerifyExpectedFiles(t *testing.T) {
	ws := t.TempDir()
	
	// Cria um arquivo existente
	existFile := filepath.Join(ws, "exists.go")
	_ = os.WriteFile(existFile, []byte("package main"), 0644)

	expected := []string{"exists.go", "missing.go"}
	missing := VerifyExpectedFiles(expected, ws)

	if len(missing) != 1 || missing[0] != "missing.go" {
		t.Errorf("VerifyExpectedFiles deveria retornar missing.go como ausente, obteve %v", missing)
	}
}
