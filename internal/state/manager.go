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
)

// TaskItem representa uma sub-tarefa no plano de execução do agente
type TaskItem struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status"` // pending, in_progress, completed, failed
}

// AgentState representa o estado completo e persistente de um agente
type AgentState struct {
	ID                string        `json:"id,omitempty"`
	Name              string        `json:"name,omitempty"`
	Status            string        `json:"status,omitempty"` // Mapeia para o status da UI do frontend
	DiretorioAtual    string        `json:"diretorio_atual"`
	ArquivosFocados   []string      `json:"arquivos_focados"`
	TarefaEmAndamento string        `json:"tarefa_em_andamento"`
	UltimoStatus      string        `json:"ultimo_status"` // idle, thinking, executing, finished
	LogsRelevantes    []string      `json:"logs_relevantes"`
	TokensGastos      int           `json:"tokens_gastos"`
	TotalTurnos       int           `json:"total_turnos"`
	Timestamp         time.Time     `json:"timestamp"`
	Messages          []llm.Message `json:"messages,omitempty"`
	Plan              []TaskItem    `json:"plan,omitempty"`
	BrowserURL        string        `json:"browser_url,omitempty"`
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
			ID:           sessionName,
			Name:         "Sessão", // Default
			UltimoStatus: "idle",
			Status:       "idle",
			ArquivosFocados: []string{},
			LogsRelevantes:  []string{},
			Timestamp:       time.Now(),
		},
	}
}

// newDefaultState retorna um estado inicial limpo
func newDefaultState() *AgentState {
	return &AgentState{
		UltimoStatus:   "idle",
		ArquivosFocados: []string{},
		LogsRelevantes:  []string{},
		Timestamp:       time.Now(),
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
	sm.state.UltimoStatus = status
	return sm.saveStateLocked()
}

// SetActiveTask define a tarefa em andamento e persiste
func (sm *StateManager) SetActiveTask(task string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.state.TarefaEmAndamento = task
	sm.state.UltimoStatus = "thinking"
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
