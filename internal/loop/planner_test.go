package loop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
