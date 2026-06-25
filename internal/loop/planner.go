package loop

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/crom/crom-agente/internal/llm"
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

// statusWeight retorna o peso ordinal de um status para determinar regressões.
// completed (3) > in_progress (2) > pending (1) > desconhecido (0)
func statusWeight(status string) int {
	switch status {
	case "completed":
		return 3
	case "in_progress":
		return 2
	case "pending":
		return 1
	default:
		return 0
	}
}

// normalizeStopWords define artigos, preposições e verbos auxiliares comuns em pt-BR
// usados para normalização de títulos de tarefas na de-duplicação.
var normalizeStopWords = map[string]bool{
	"o": true, "a": true, "os": true, "as": true,
	"um": true, "uma": true, "uns": true, "umas": true,
	"de": true, "do": true, "da": true, "dos": true, "das": true,
	"em": true, "no": true, "na": true, "nos": true, "nas": true,
	"por": true, "para": true, "com": true, "sem": true,
	"e": true, "ou": true,
	"criar": true, "fazer": true, "escrever": true, "implementar": true, "adicionar": true,
	"create": true, "make": true, "write": true, "implement": true, "add": true,
	"the": true, "an": true,
}

// NormalizeTitle normaliza um título de tarefa removendo pontuações,
// stop words e comparando em lowercase para evitar duplicatas.
func NormalizeTitle(title string) string {
	lower := strings.ToLower(strings.TrimSpace(title))
	// Remove pontuações
	var cleaned []rune
	for _, r := range lower {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) || r == '.' || r == '-' || r == '_' || r == '/' {
			cleaned = append(cleaned, r)
		}
	}
	words := strings.Fields(string(cleaned))
	var filtered []string
	for _, w := range words {
		if !normalizeStopWords[w] {
			filtered = append(filtered, w)
		}
	}
	if len(filtered) == 0 {
		return lower
	}
	return strings.Join(filtered, " ")
}

// UpdatePlannerFromMessage extrai checklists de plano da mensagem e salva no StateManager se houver modificações.
// Aplica proteção contra regressão de status (Fase 17) e normalização de títulos para de-duplicação (Fase 18).
func UpdatePlannerFromMessage(sm *state.StateManager, message string) {
	UpdatePlannerFromMessageWithConfig(sm, message, false)
}

// UpdatePlannerFromMessageWithConfig é a versão completa que aceita configuração de proteção de cache.
// Se disableCacheProtection for true, permite regressão de status (ex: de completed para pending).
func UpdatePlannerFromMessageWithConfig(sm *state.StateManager, message string, disableCacheProtection bool) {
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

	// Mapeia plano atual com chave exata e normalizada
	exactMap := make(map[string]int)
	normMap := make(map[string]int)
	for idx, item := range currentPlan {
		exactMap[strings.ToLower(item.Title)] = idx
		normMap[NormalizeTitle(item.Title)] = idx
	}

	// Mescla
	for _, newItem := range newPlan {
		exactKey := strings.ToLower(newItem.Title)
		normKey := NormalizeTitle(newItem.Title)

		// Busca: primeiro por match exato, depois por match normalizado
		idx := -1
		if i, exists := exactMap[exactKey]; exists {
			idx = i
		} else if i, exists := normMap[normKey]; exists {
			idx = i
		}

		if idx >= 0 {
			// Proteção contra regressão de status (Plan Cache Guard)
			if !disableCacheProtection && statusWeight(newItem.Status) < statusWeight(currentPlan[idx].Status) {
				// Ignora regressão: não permite que um status mais avançado volte
				continue
			}
			if currentPlan[idx].Status != newItem.Status {
				currentPlan[idx].Status = newItem.Status
			}
		} else {
			// Verifica se é duplicata normalizada de algum item existente antes de adicionar
			currentPlan = append(currentPlan, newItem)
			// Atualiza os mapas
			newIdx := len(currentPlan) - 1
			exactMap[exactKey] = newIdx
			normMap[normKey] = newIdx
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

// --- Fase 14: ParseExpectedFiles ---

// filePathRegex captura caminhos de arquivos mencionados em seções de Proposed Changes
var filePathRegex = regexp.MustCompile(`(?:(?:\[(?:NEW|MODIFY|DELETE)\])|(?:file:///))\s*(?:\[.*?\]\()?([\w/._-]+(?:\.\w+)+)\)?`)

// ParseExpectedFiles examina o histórico de mensagens do assistente e extrai
// caminhos de arquivos que foram mencionados em seções [Proposed Changes], [NEW], [MODIFY]
// ou em links file:///. Isso é usado para verificar se os arquivos foram realmente criados no disco.
func ParseExpectedFiles(messages []llm.Message) []string {
	seen := make(map[string]bool)
	var files []string

	for _, msg := range messages {
		if msg.Role != "assistant" {
			continue
		}

		// Busca por padrões de arquivo em seções [NEW] e [MODIFY]
		matches := filePathRegex.FindAllStringSubmatch(msg.Content, -1)
		for _, match := range matches {
			if len(match) >= 2 {
				path := strings.TrimSpace(match[1])
				// Ignora se parecer com um diretório ou for muito curto
				if path == "" || !strings.Contains(path, ".") {
					continue
				}
				if !seen[path] {
					seen[path] = true
					files = append(files, path)
				}
			}
		}

		// Busca adicional por links file:/// explícitos
		scanner := bufio.NewScanner(strings.NewReader(msg.Content))
		for scanner.Scan() {
			line := scanner.Text()
			if idx := strings.Index(line, "file:///"); idx >= 0 {
				rest := line[idx+len("file:///"):]
				// Extrai até o próximo parêntese, espaço ou final da linha
				end := len(rest)
				for i, ch := range rest {
					if ch == ')' || ch == ' ' || ch == '\t' || ch == '"' || ch == '\'' {
						end = i
						break
					}
				}
				path := "/" + rest[:end]
				if strings.Contains(path, ".") && !seen[path] {
					seen[path] = true
					files = append(files, path)
				}
			}
		}
	}

	return files
}

// VerifyExpectedFiles verifica se os arquivos esperados existem no disco.
// Retorna os caminhos dos arquivos que estão faltando.
func VerifyExpectedFiles(expectedFiles []string, workspaceDir string) []string {
	var missing []string
	for _, f := range expectedFiles {
		// Se o caminho já for absoluto, verifica diretamente
		checkPath := f
		if !filepath.IsAbs(f) {
			checkPath = filepath.Join(workspaceDir, f)
		}
		if _, err := os.Stat(checkPath); os.IsNotExist(err) {
			missing = append(missing, f)
		}
	}
	return missing
}
