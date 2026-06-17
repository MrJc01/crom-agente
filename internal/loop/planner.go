package loop

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/crom/crom-agente/internal/state"
)

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
