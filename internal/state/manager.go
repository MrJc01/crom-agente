package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/crom/crom-agente/internal/llm"
	"github.com/crom/crom-agente/internal/security"
)

const (
	// DefaultStateFileName é o nome padrão do arquivo de estado persistente
	DefaultStateFileName = ".crom_state.json"

	// MaxRelevantLogs limita o histórico de logs no estado para evitar crescimento infinito
	MaxRelevantLogs = 20

	// Status Operacionais
	StatusIdle             = "idle"
	StatusThinking         = "thinking"
	StatusReading          = "reading"
	StatusWriting          = "writing"
	StatusExecutingTool    = "executing_tool"
	StatusWaitingUserInput = "waiting_user_input"
	StatusFinished         = "finished"
	StatusCanceled         = "canceled"
	StatusFailed           = "failed"

	// Modos Cognitivos
	ModoPlanning    = "planning"
	ModoExecuting   = "executing"
	ModoVerifying   = "verifying"
	ModoDebugging   = "debugging"
	ModoInteracting = "interacting"
)

// ToolTrace representa o rastro exato de uma execução de ferramenta
type ToolTrace struct {
	ToolName   string `json:"tool_name"`
	Args       string `json:"args"`
	Success    bool   `json:"success"`
	Output     string `json:"output"`
	DurationMs int64  `json:"duration_ms"`
}

// TerminalTelemetry representa o estado de uma sessão de terminal ativa
type TerminalTelemetry struct {
	ID        string    `json:"id"`
	PID       int       `json:"pid"`
	Name      string    `json:"name"`
	Closed    bool      `json:"closed"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ProcessTelemetry representa um processo em execução em foreground ou background
type ProcessTelemetry struct {
	ID           string    `json:"id"`
	Command      string    `json:"command"`
	PID          int       `json:"pid"`
	Status       string    `json:"status"` // running, completed, failed, killed
	StartedAt    time.Time `json:"started_at"`
	IsBackground bool      `json:"is_background"`
}

// IterationLog contém todos os dados granulares de uma iteração do LLM
type IterationLog struct {
	Iteration         int           `json:"iteration"`
	Timestamp         time.Time     `json:"timestamp"`
	PromptTokens      int           `json:"prompt_tokens"`
	CompletionTokens  int           `json:"completion_tokens"`
	TotalTokens       int           `json:"total_tokens"`
	MessagesCount     int           `json:"messages_count"`
	Messages          []llm.Message `json:"messages"`
	SystemPromptsUsed []string      `json:"system_prompts_used,omitempty"`
	ToolsCalled       []ToolTrace   `json:"tools_called,omitempty"`
}

// TaskItem representa uma sub-tarefa no plano de execução do agente
type TaskItem struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status"` // pending, in_progress, completed, failed
}

// AgentState representa o estado completo e persistente de um agente
type AgentState struct {
	ID                      string              `json:"id,omitempty"`
	Name                    string              `json:"name,omitempty"`
	Status                  string              `json:"status,omitempty"` // Mapeia para o status da UI do frontend (compatibilidade)
	DiretorioAtual          string              `json:"diretorio_atual"`
	ArquivosFocados         []string            `json:"arquivos_focados"`
	TarefaEmAndamento       string              `json:"tarefa_em_andamento"`
	UltimoStatus            string              `json:"ultimo_status"` // Mapeia para StatusOperacional (compatibilidade)
	StatusOperacional       string              `json:"status_operacional"` // idle, thinking, reading, etc.
	ModoCognitivo           string              `json:"modo_cognitivo"`     // planning, executing, etc.
	LogsRelevantes          []string            `json:"logs_relevantes"`
	TokensGastos            int                 `json:"tokens_gastos"`
	TotalTurnos             int                 `json:"total_turnos"`
	Timestamp               time.Time           `json:"timestamp"`
	Messages                []llm.Message       `json:"messages,omitempty"`
	Plan                    []TaskItem          `json:"plan,omitempty"`
	BrowserURL              string              `json:"browser_url,omitempty"`
	SubagentsContext        map[string]string   `json:"subagents_context,omitempty"`
	FilesCreated            int                 `json:"files_created"`
	FilesValidated          int                 `json:"files_validated"`
	ToolCallsEmitted        int                 `json:"tool_calls_emitted"`
	ToolCallsFromTextParse  int                 `json:"tool_calls_from_text_parse"`
	CircuitBreakerTriggered bool                `json:"circuit_breaker_triggered"`
	ActiveTerminals         []TerminalTelemetry `json:"active_terminals"`
	ActiveProcesses         []ProcessTelemetry  `json:"active_processes"`
	CurrentStep             string              `json:"current_step"`
	CurrentStepDurationMs   int64               `json:"current_step_duration_ms"`
}

// StateManager gerencia a leitura, escrita e acesso concorrente ao estado do agente
type StateManager struct {
	mu       sync.RWMutex
	state    *AgentState
	filePath string
}

// NewStateManager cria um novo gerenciador de estado apontando para um diretório de armazenamento
func NewStateManager(storagePath string) *StateManager {
	return &StateManager{
		filePath: filepath.Join(storagePath, DefaultStateFileName),
		state:    newDefaultState(),
	}
}

// NewSessionStateManager cria um novo gerenciador de estado apontando para um arquivo de sessão específico dentro de uma subpasta dedicada
func NewSessionStateManager(storagePath, sessionName string) *StateManager {
	return &StateManager{
		filePath: filepath.Join(storagePath, "sessions", sessionName, "session.json"),
		state: &AgentState{
			ID:                sessionName,
			Name:              "Sessão", // Default
			UltimoStatus:      StatusIdle,
			Status:            StatusIdle,
			StatusOperacional: StatusIdle,
			ModoCognitivo:     ModoPlanning,
			ArquivosFocados:   []string{},
			LogsRelevantes:    []string{},
			Timestamp:         time.Now(),
			ActiveTerminals:   []TerminalTelemetry{},
			ActiveProcesses:   []ProcessTelemetry{},
		},
	}
}

// newDefaultState retorna um estado inicial limpo
func newDefaultState() *AgentState {
	return &AgentState{
		UltimoStatus:      StatusIdle,
		Status:            StatusIdle,
		StatusOperacional: StatusIdle,
		ModoCognitivo:     ModoPlanning,
		ArquivosFocados:   []string{},
		LogsRelevantes:    []string{},
		Timestamp:         time.Now(),
		ActiveTerminals:   []TerminalTelemetry{},
		ActiveProcesses:   []ProcessTelemetry{},
	}
}

// LoadState carrega o estado do disco. Se o arquivo não existir, mantém o estado padrão.
func (sm *StateManager) LoadState() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	data, err := os.ReadFile(sm.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Arquivo não existe: manter estado padrão e persistir
			return sm.saveStateLocked()
		}
		return fmt.Errorf("erro ao ler arquivo de estado: %w", err)
	}

	var loaded AgentState
	if err := json.Unmarshal(data, &loaded); err != nil {
		return fmt.Errorf("erro ao parsear JSON de estado: %w", err)
	}

	sm.state = &loaded

	// Compatibilidade e fallbacks para sessões legadas
	if sm.state.StatusOperacional == "" {
		if sm.state.UltimoStatus != "" {
			sm.state.StatusOperacional = sm.state.UltimoStatus
		} else {
			sm.state.StatusOperacional = StatusIdle
		}
	}
	if sm.state.ModoCognitivo == "" {
		sm.state.ModoCognitivo = ModoPlanning
	}
	if sm.state.ActiveTerminals == nil {
		sm.state.ActiveTerminals = []TerminalTelemetry{}
	}
	if sm.state.ActiveProcesses == nil {
		sm.state.ActiveProcesses = []ProcessTelemetry{}
	}
	// Garante consistência de campos legados
	sm.state.UltimoStatus = sm.state.StatusOperacional
	sm.state.Status = sm.state.StatusOperacional

	return nil
}

// SaveState persiste o estado atual no disco de forma atômica (escreve em temp e renomeia)
func (sm *StateManager) SaveState() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.saveStateLocked()
}

// saveStateLocked persiste o estado (deve ser chamado com o mutex já travado)
func (sm *StateManager) saveStateLocked() error {
	sm.state.Timestamp = time.Now()

	// Aplicar redação de dados sensíveis antes de serializar
	sm.state.TarefaEmAndamento = security.Redact(sm.state.TarefaEmAndamento)
	for i, l := range sm.state.LogsRelevantes {
		sm.state.LogsRelevantes[i] = security.Redact(l)
	}
	for i, m := range sm.state.Messages {
		sm.state.Messages[i].Content = security.Redact(m.Content)
	}
	for i, p := range sm.state.ActiveProcesses {
		sm.state.ActiveProcesses[i].Command = security.Redact(p.Command)
	}

	data, err := json.MarshalIndent(sm.state, "", "  ")
	if err != nil {
		return fmt.Errorf("erro ao serializar estado: %w", err)
	}

	dir := filepath.Dir(sm.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("erro ao criar diretório de estado: %w", err)
	}

	// Escrita atômica: grava em arquivo temporário e renomeia
	tmpFile := sm.filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("erro ao gravar arquivo temporário de estado: %w", err)
	}
	if err := os.Rename(tmpFile, sm.filePath); err != nil {
		return fmt.Errorf("erro ao renomear arquivo de estado: %w", err)
	}

	return nil
}

// GetState retorna uma cópia segura do estado atual (leitura concorrente)
func (sm *StateManager) GetState() AgentState {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return *sm.state
}

// SetStatus atualiza o status do agente e persiste no disco
func (sm *StateManager) SetStatus(status string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.state.StatusOperacional = status
	sm.state.UltimoStatus = status
	sm.state.Status = status
	return sm.saveStateLocked()
}

// SetOperationalStatus define o status operacional físico do agente e persiste no disco
func (sm *StateManager) SetOperationalStatus(status string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.state.StatusOperacional = status
	sm.state.UltimoStatus = status
	sm.state.Status = status
	return sm.saveStateLocked()
}

// SetCognitiveMode define a fase/modo de raciocínio cognitivo do agente e persiste no disco
func (sm *StateManager) SetCognitiveMode(mode string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.state.ModoCognitivo = mode
	return sm.saveStateLocked()
}

// SetActiveTask define a tarefa em andamento e persiste
func (sm *StateManager) SetActiveTask(task string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.state.TarefaEmAndamento = task
	sm.state.StatusOperacional = StatusThinking
	sm.state.UltimoStatus = StatusThinking
	sm.state.Status = StatusThinking
	return sm.saveStateLocked()
}

// AddLog adiciona uma entrada ao histórico de logs relevantes, respeitando o limite máximo
func (sm *StateManager) AddLog(entry string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.state.LogsRelevantes = append(sm.state.LogsRelevantes, entry)
	if len(sm.state.LogsRelevantes) > MaxRelevantLogs {
		sm.state.LogsRelevantes = sm.state.LogsRelevantes[len(sm.state.LogsRelevantes)-MaxRelevantLogs:]
	}

	return sm.saveStateLocked()
}

// RecordTokens incrementa o contador de tokens gastos
func (sm *StateManager) RecordTokens(tokens int) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.state.TokensGastos += tokens
	sm.state.TotalTurnos++
	return sm.saveStateLocked()
}

// FilePath retorna o caminho do arquivo de estado
func (sm *StateManager) FilePath() string {
	return sm.filePath
}

// GetWorkspaceDir retorna o diretório raiz do workspace (o diretório que contém a pasta .crom)
func (sm *StateManager) GetWorkspaceDir() string {
	path := sm.filePath
	dir := filepath.Dir(path)
	for {
		if filepath.Base(dir) == ".crom" {
			return filepath.Dir(dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	// Fallback caso não encontre .crom
	return filepath.Dir(filepath.Dir(path))
}

// GetMessages retorna uma cópia segura do histórico de mensagens da conversação
func (sm *StateManager) GetMessages() []llm.Message {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.state == nil {
		return nil
	}
	cp := make([]llm.Message, len(sm.state.Messages))
	copy(cp, sm.state.Messages)
	return cp
}

// SetMessages atualiza o histórico de mensagens e persiste no disco
func (sm *StateManager) SetMessages(messages []llm.Message) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.state.Messages = messages
	return sm.saveStateLocked()
}

// GetPlan retorna o plano de tarefas atual
func (sm *StateManager) GetPlan() []TaskItem {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.state == nil {
		return nil
	}
	cp := make([]TaskItem, len(sm.state.Plan))
	copy(cp, sm.state.Plan)
	return cp
}

// SetPlan atualiza o plano de tarefas e persiste
func (sm *StateManager) SetPlan(plan []TaskItem) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.state.Plan = plan
	return sm.saveStateLocked()
}

// GetBrowserURL retorna a URL do navegador do agente
func (sm *StateManager) GetBrowserURL() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.state == nil {
		return ""
	}
	return sm.state.BrowserURL
}

// SetBrowserURL atualiza a URL do navegador e persiste no disco
func (sm *StateManager) SetBrowserURL(url string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.state.BrowserURL = url
	return sm.saveStateLocked()
}

// SaveIterationLog grava o log granular de uma iteração na pasta 'iterations' da sessão
func (sm *StateManager) SaveIterationLog(iteration int, logData IterationLog) error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	dir := filepath.Join(filepath.Dir(sm.filePath), "iterations")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("erro ao criar diretório de iterations: %w", err)
	}

	fileName := fmt.Sprintf("%03d.json", iteration)
	fullPath := filepath.Join(dir, fileName)

	// Applica redação no conteúdo para log
	logCopy := logData
	if logCopy.Messages != nil {
		redactedMsgs := make([]llm.Message, len(logCopy.Messages))
		for i, m := range logCopy.Messages {
			m.Content = security.Redact(m.Content)
			redactedMsgs[i] = m
		}
		logCopy.Messages = redactedMsgs
	}
	for i, t := range logCopy.ToolsCalled {
		t.Args = security.Redact(t.Args)
		t.Output = security.Redact(t.Output)
		logCopy.ToolsCalled[i] = t
	}

	data, err := json.MarshalIndent(logCopy, "", "  ")
	if err != nil {
		return fmt.Errorf("erro ao serializar IterationLog: %w", err)
	}

	return os.WriteFile(fullPath, data, 0644)
}

// GetSummaryForAgent recupera o resumo de historico de um subagente especialista
func (sm *StateManager) GetSummaryForAgent(name string) string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.state == nil || sm.state.SubagentsContext == nil {
		return ""
	}
	return sm.state.SubagentsContext[name]
}

// UpdateSummaryForAgent atualiza o resumo de historico de um subagente e persiste no disco
func (sm *StateManager) UpdateSummaryForAgent(name, summary string) error {
	sm.mu.Lock()
	if sm.state == nil {
		sm.state = newDefaultState()
	}
	if sm.state.SubagentsContext == nil {
		sm.state.SubagentsContext = make(map[string]string)
	}
	sm.state.SubagentsContext[name] = summary
	sm.mu.Unlock()
	return sm.SaveState()
}

// RecordFileCreated incrementa a contagem de arquivos criados/editados.
func (sm *StateManager) RecordFileCreated() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.state.FilesCreated++
	return sm.saveStateLocked()
}

// RecordFileValidated incrementa a contagem de validações de arquivo.
func (sm *StateManager) RecordFileValidated() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.state.FilesValidated++
	return sm.saveStateLocked()
}

// RecordToolCallEmitted incrementa a contagem de chamadas de ferramentas emitidas.
func (sm *StateManager) RecordToolCallEmitted() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.state.ToolCallsEmitted++
	return sm.saveStateLocked()
}

// RecordToolCallsFromTextParse incrementa a contagem de chamadas de ferramenta extraídas via parser textual.
func (sm *StateManager) RecordToolCallsFromTextParse(count int) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.state.ToolCallsFromTextParse += count
	return sm.saveStateLocked()
}

// SetCircuitBreakerTriggered define que o circuit breaker foi disparado.
func (sm *StateManager) SetCircuitBreakerTriggered(triggered bool) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.state.CircuitBreakerTriggered = triggered
	return sm.saveStateLocked()
}

// UpdateActiveTerminals atualiza a lista de terminais interativos ativos e persiste o estado
func (sm *StateManager) UpdateActiveTerminals(terminals []TerminalTelemetry) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if terminals == nil {
		sm.state.ActiveTerminals = []TerminalTelemetry{}
	} else {
		sm.state.ActiveTerminals = terminals
	}
	return sm.saveStateLocked()
}

// UpdateActiveProcesses atualiza a lista de processos ativos (foreground/background) e persiste o estado
func (sm *StateManager) UpdateActiveProcesses(processes []ProcessTelemetry) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if processes == nil {
		sm.state.ActiveProcesses = []ProcessTelemetry{}
	} else {
		sm.state.ActiveProcesses = processes
	}
	return sm.saveStateLocked()
}

// SetCurrentStep define o passo/ação detalhado atual que o agente está fazendo e persiste
func (sm *StateManager) SetCurrentStep(step string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.state.CurrentStep = step
	return sm.saveStateLocked()
}

// SetCurrentStepDurationMs define a duração em milissegundos da execução da ferramenta atual e persiste
func (sm *StateManager) SetCurrentStepDurationMs(durationMs int64) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.state.CurrentStepDurationMs = durationMs
	return sm.saveStateLocked()
}

// ClearActiveTerminals limpa a lista de terminais e persiste
func (sm *StateManager) ClearActiveTerminals() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.state.ActiveTerminals = []TerminalTelemetry{}
	return sm.saveStateLocked()
}

// ClearActiveProcesses limpa a lista de processos ativos e persiste
func (sm *StateManager) ClearActiveProcesses() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.state.ActiveProcesses = []ProcessTelemetry{}
	return sm.saveStateLocked()
}

