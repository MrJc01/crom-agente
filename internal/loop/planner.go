package loop

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/crom/crom-agente/internal/state"
)

// ExecutionPhase representa a fase atual do motor de planejamento
type ExecutionPhase string

const (
	// PhasePlanning é a fase de planejamento: o agente analisa o pedido e gera um plano detalhado
	PhasePlanning ExecutionPhase = "planning"
	// PhaseExecution é a fase de execução: o agente executa as tarefas do plano gerado
	PhaseExecution ExecutionPhase = "execution"
)

// WritePlanToFile salva o plano estruturado como um arquivo markdown no workspace
func WritePlanToFile(sm *state.StateManager, plan []state.TaskItem) error {
	if sm == nil {
		return nil
	}

	filePath := sm.FilePath()
	dir := filepath.Dir(filePath)
	if filepath.Base(dir) == "sessions" {
		dir = filepath.Dir(dir)
	}

	planPath := filepath.Join(dir, "plan.md")

	var sb strings.Builder
	sb.WriteString("# Plano de Trabalho Cromia\n\n")
	sb.WriteString("Este arquivo é atualizado dinamicamente pelo agente Cromia durante a execução das tarefas.\n\n")

	for _, item := range plan {
		box := "[ ]"
		switch item.Status {
		case "in_progress":
			box = "[/]"
		case "completed":
			box = "[x]"
		}
		sb.WriteString(fmt.Sprintf("- %s %s\n", box, item.Title))
	}

	return os.WriteFile(planPath, []byte(sb.String()), 0644)
}

// WriteTaskMdToSession salva o task.md de sessão dentro da pasta da sessão (.crom/sessions/<id>/task.md)
// Isso espelha o comportamento do Antigravity IDE e permite rastrear o progresso interno de sessões longas.
func WriteTaskMdToSession(sm *state.StateManager, plan []state.TaskItem) error {
	if sm == nil {
		return nil
	}

	sessionDir := filepath.Dir(sm.FilePath())

	var sb strings.Builder
	sb.WriteString("# Checklist de Tarefas da Sessão\n\n")
	sb.WriteString("Gerado automaticamente pelo agente Cromia. Atualizado a cada iteração.\n\n")

	pendingCount := 0
	inProgressCount := 0
	completedCount := 0

	for _, item := range plan {
		box := "[ ]"
		switch item.Status {
		case "in_progress":
			box = "[/]"
			inProgressCount++
		case "completed":
			box = "[x]"
			completedCount++
		default:
			pendingCount++
		}
		sb.WriteString(fmt.Sprintf("- %s %s\n", box, item.Title))
	}

	sb.WriteString(fmt.Sprintf("\n---\n**Progresso**: %d concluídas / %d em andamento / %d pendentes\n",
		completedCount, inProgressCount, pendingCount))

	taskMdPath := filepath.Join(sessionDir, "task.md")
	return os.WriteFile(taskMdPath, []byte(sb.String()), 0644)
}

// HasPendingTasks verifica se o plano ainda contém itens pendentes ou em andamento
// Retorna true se o agente NÃO deve encerrar a sessão ainda.
func HasPendingTasks(plan []state.TaskItem) bool {
	for _, item := range plan {
		if item.Status == "pending" || item.Status == "in_progress" {
			return true
		}
	}
	return false
}

// GeneratePendingTasksWarning gera uma mensagem de correção para injetar no contexto
// quando o agente tenta encerrar com tarefas ainda abertas.
func GeneratePendingTasksWarning(plan []state.TaskItem) string {
	var pending []string
	var inProgress []string

	for _, item := range plan {
		switch item.Status {
		case "pending":
			pending = append(pending, fmt.Sprintf("  - [ ] %s", item.Title))
		case "in_progress":
			inProgress = append(inProgress, fmt.Sprintf("  - [/] %s", item.Title))
		}
	}

	var msg strings.Builder
	msg.WriteString("⚠️ [TASK_INCOMPLETE_WARNING] Você está tentando encerrar a sessão, mas ainda existem tarefas não concluídas no plano:\n")
	if len(inProgress) > 0 {
		msg.WriteString("\n**Em andamento:**\n")
		msg.WriteString(strings.Join(inProgress, "\n"))
		msg.WriteString("\n")
	}
	if len(pending) > 0 {
		msg.WriteString("\n**Pendentes:**\n")
		msg.WriteString(strings.Join(pending, "\n"))
		msg.WriteString("\n")
	}
	msg.WriteString("\n**Você DEVE continuar executando** até que todos os itens estejam marcados como `[x]`. " +
		"Retome imediatamente a próxima tarefa pendente.")
	return msg.String()
}

// ParsePlan extrai tarefas de checklists em markdown (ex: - [ ] Tarefa) contidos na string
func ParsePlan(content string) []state.TaskItem {
	var items []state.TaskItem
	scanner := bufio.NewScanner(strings.NewReader(content))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Detecta padrões de checkbox
		var status string
		var title string

		if strings.HasPrefix(line, "- [ ]") || strings.HasPrefix(line, "* [ ]") {
			status = "pending"
			title = strings.TrimSpace(line[5:])
		} else if strings.HasPrefix(line, "- [/]") || strings.HasPrefix(line, "* [/]") {
			status = "in_progress"
			title = strings.TrimSpace(line[5:])
		} else if strings.HasPrefix(line, "- [x]") || strings.HasPrefix(line, "* [x]") ||
			strings.HasPrefix(line, "- [X]") || strings.HasPrefix(line, "* [X]") {
			status = "completed"
			title = strings.TrimSpace(line[5:])
		}

		if status != "" && title != "" {
			items = append(items, state.TaskItem{
				Title:  title,
				Status: status,
			})
		}
	}

	return items
}

// UpdatePlannerFromMessage extrai checklists de plano da mensagem e salva no StateManager se houver modificações
func UpdatePlannerFromMessage(sm *state.StateManager, message string) {
	if sm == nil {
		return
	}

	newPlan := ParsePlan(message)
	if len(newPlan) == 0 {
		return
	}

	// Se já existir um plano, mescla atualizações de status mantendo novos itens
	currentPlan := sm.GetPlan()
	if len(currentPlan) == 0 {
		_ = sm.SetPlan(newPlan)
		_ = WritePlanToFile(sm, newPlan)
		_ = WriteTaskMdToSession(sm, newPlan)
		return
	}

	// Mapeia plano atual
	planMap := make(map[string]int)
	for idx, item := range currentPlan {
		planMap[strings.ToLower(item.Title)] = idx
	}

	// Mescla
	for _, newItem := range newPlan {
		key := strings.ToLower(newItem.Title)
		if idx, exists := planMap[key]; exists {
			// Atualiza o status se mudou
			if currentPlan[idx].Status != newItem.Status {
				currentPlan[idx].Status = newItem.Status
			}
		} else {
			// Adiciona novo item do plano
			currentPlan = append(currentPlan, newItem)
		}
	}

	_ = sm.SetPlan(currentPlan)
	_ = WritePlanToFile(sm, currentPlan)
	_ = WriteTaskMdToSession(sm, currentPlan)
}

// SyncPlanToContext gera uma representação em texto do plano estruturado para ser injetada no loop
func SyncPlanToContext(sm *state.StateManager) string {
	if sm == nil {
		return ""
	}

	plan := sm.GetPlan()
	if len(plan) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n📋 [PLANO DE TRABALHO ATUAL]:\n")
	for _, item := range plan {
		box := "[ ]"
		switch item.Status {
		case "in_progress":
			box = "[/]"
		case "completed":
			box = "[x]"
		}
		sb.WriteString(fmt.Sprintf("- %s %s\n", box, item.Title))
	}
	return sb.String()
}

// GetCurrentPhase determina a fase atual do motor com base no plano existente.
// Se não houver plano definido ainda, estamos na fase de Planejamento.
// Se houver plano e algum item em andamento ou concluído, estamos na fase de Execução.
func GetCurrentPhase(sm *state.StateManager) ExecutionPhase {
	if sm == nil {
		return PhasePlanning
	}
	plan := sm.GetPlan()
	if len(plan) == 0 {
		return PhasePlanning
	}
	for _, item := range plan {
		if item.Status == "in_progress" || item.Status == "completed" {
			return PhaseExecution
		}
	}
	return PhasePlanning
}
