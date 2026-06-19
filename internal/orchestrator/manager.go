package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"time"

	"github.com/crom/crom-agente/internal/config"
	"github.com/crom/crom-agente/internal/llm"
	"github.com/crom/crom-agente/internal/loop"
	"github.com/crom/crom-agente/internal/permission"
	"github.com/crom/crom-agente/internal/state"
	"github.com/crom/crom-agente/internal/tools"
)

// Workspace representa um projeto registrado no orquestrador
type Workspace struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// RunningAgent representa uma instância ativa de agente em execução de fundo
type RunningAgent struct {
	WorkspaceName string
	Task          string
	Cancel        context.CancelFunc
	Loop          *loop.AgenticLoop
	Ctx           context.Context
}

// MultiAgentManager coordena a execução simultânea de loops em múltiplos workspaces
type MultiAgentManager struct {
	mu            sync.RWMutex
	runningAgents map[string]*RunningAgent // chave: workspace name
	OnSchedule    func(workspaceName, sessionName, task string, delaySecs int, provider, model string)
}

// NewMultiAgentManager cria um novo gerenciador multi-agente
func NewMultiAgentManager() *MultiAgentManager {
	return &MultiAgentManager{
		runningAgents: make(map[string]*RunningAgent),
	}
}

// LoadWorkspaces carrega os workspaces registrados do arquivo global workspaces.json
func LoadWorkspaces() ([]Workspace, error) {
	gDir, err := config.GlobalDir()
	if err != nil {
		return nil, err
	}
	wsPath := filepath.Join(gDir, config.WorkspacesFile)

	data, err := os.ReadFile(wsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []Workspace{}, nil
		}
		return nil, fmt.Errorf("erro ao ler workspaces: %w", err)
	}

	var list []Workspace
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("erro ao parsear workspaces: %w", err)
	}
	return list, nil
}

// SaveWorkspaces persiste a lista de workspaces no workspaces.json
func SaveWorkspaces(list []Workspace) error {
	gDir, err := config.GlobalDir()
	if err != nil {
		return err
	}
	wsPath := filepath.Join(gDir, config.WorkspacesFile)

	if err := os.MkdirAll(gDir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(wsPath, data, 0644)
}

// AddWorkspace registra um novo workspace no orquestrador
func (m *MultiAgentManager) AddWorkspace(name, path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("caminho inválido: %w", err)
	}

	// Valida se o diretório existe
	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("caminho não existe: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("o caminho especificado não é um diretório")
	}

	list, err := LoadWorkspaces()
	if err != nil {
		return err
	}

	// Verifica duplicidade
	for _, ws := range list {
		if ws.Name == name {
			return fmt.Errorf("já existe um workspace com o nome '%s'", name)
		}
		if ws.Path == absPath {
			return fmt.Errorf("este caminho já está registrado sob o nome '%s'", ws.Name)
		}
	}

	list = append(list, Workspace{Name: name, Path: absPath})
	return SaveWorkspaces(list)
}

// RemoveWorkspace remove um workspace registrado
func (m *MultiAgentManager) RemoveWorkspace(name string) error {
	list, err := LoadWorkspaces()
	if err != nil {
		return err
	}

	found := false
	newList := make([]Workspace, 0, len(list))
	for _, ws := range list {
		if ws.Name == name {
			found = true
			continue
		}
		newList = append(newList, ws)
	}

	if !found {
		return fmt.Errorf("workspace '%s' não encontrado", name)
	}

	return SaveWorkspaces(newList)
}

// StartAgent inicia a execução de um loop ReAct em background para um determinado workspace e sessão
func (m *MultiAgentManager) StartAgent(ctx context.Context, workspaceName, sessionName, task string, handler loop.EventHandler) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 1. Verifica se já está rodando
	if _, running := m.runningAgents[workspaceName]; running {
		return fmt.Errorf("já existe um agente em execução no workspace '%s'", workspaceName)
	}

	// 2. Carrega workspaces registrados
	workspaces, err := LoadWorkspaces()
	if err != nil {
		return err
	}

	var target *Workspace
	for _, ws := range workspaces {
		if ws.Name == workspaceName || ws.Path == workspaceName {
			target = &ws
			break
		}
	}
	if target == nil {
		// Tenta criar o diretório no disco se não existir e registrar automaticamente
		_ = os.MkdirAll(workspaceName, 0755)
		if info, err := os.Stat(workspaceName); err == nil && info.IsDir() {
			name := filepath.Base(workspaceName)
			if name == "" || name == "." || name == "/" {
				name = "workspace-" + filepath.Base(filepath.Clean(workspaceName))
			}
			// Adiciona o workspace no registro
			_ = m.AddWorkspace(name, workspaceName)
			// Recarrega a lista
			if updatedList, errLoad := LoadWorkspaces(); errLoad == nil {
				for _, ws := range updatedList {
					if ws.Path == workspaceName || ws.Name == name {
						target = &ws
						break
					}
				}
			}
		}
	}
	if target == nil {
		return fmt.Errorf("workspace '%s' não encontrado no registro", workspaceName)
	}

	// 3. Carrega configurações e envs
	gDir, err := config.GlobalDir()
	if err != nil {
		return err
	}
	global, err := config.LoadGlobalConfig(gDir)
	if err != nil {
		return err
	}
	env, err := config.LoadEnvVars(gDir)
	if err != nil {
		return err
	}
	workspaceCfg, err := config.LoadWorkspaceConfig(target.Path)
	if err != nil {
		return err
	}

	// Resolve config (CLI flags vazias)
	resolved := config.Resolve(global, workspaceCfg, config.CLIFlags{})

	if providerOverride, ok := ctx.Value("provider_override").(string); ok && providerOverride != "" {
		resolved.Provider = providerOverride
	}
	if modelOverride, ok := ctx.Value("model_override").(string); ok && modelOverride != "" {
		resolved.Model = modelOverride
	}

	// 4. Instancia LLM Provider
	provider, err := llm.NewProvider(resolved.Provider, resolved.Model, func(key string) string {
		return env.Get(key)
	})
	if err != nil {
		return err
	}

	// 5. Instancia StateManager
	storageDir := filepath.Join(target.Path, ".crom")
	if sessionName == "" {
		sessionName = fmt.Sprintf("session-%d", time.Now().UnixNano())
	}
	sm := state.NewSessionStateManager(storageDir, sessionName)
	if err := sm.LoadState(); err != nil {
		return err
	}

	// 6. Cria context cancelável
	agentCtx, cancel := context.WithCancel(ctx)

	// 7. Inicializa o PermissionManager
	askFunc := func(action, target string) (bool, bool) {
		fmt.Printf("\n⚠️  [HITL Multi-Agente] Solicitação de permissão: [%s] no alvo: %q\n", action, target)
		fmt.Print("👉 Pressione [a] para aprovar uma vez, [s] para sempre permitir, [r] para rejeitar: ")
		var response string
		_, _ = fmt.Scanln(&response)
		response = strings.TrimSpace(strings.ToLower(response))
		if response == "s" {
			return true, true
		}
		if response == "a" {
			return true, false
		}
		return false, false
	}
	if pr, ok := handler.(interface {
		AskPermission(action, target string) (bool, bool)
	}); ok {
		askFunc = pr.AskPermission
	}
	pm := permission.NewPermissionManager(target.Path, resolved.PermissionMode, askFunc)

	// 8. Cria AgenticLoop
	al := loop.New(provider, sm, handler, resolved)

	// Registrar ferramentas nativas e gerenciador de permissões
	al.RegisterTool(tools.NewScheduleTimerTool(target.Path, func(task string, durationSeconds int) {
		if m.OnSchedule != nil {
			m.OnSchedule(workspaceName, sessionName, task, durationSeconds, resolved.Provider, resolved.Model)
		}
	}))
	al.RegisterTool(tools.NewReadFileTool(target.Path, resolved.WorkspaceJail))
	al.RegisterTool(tools.NewWriteFileTool(target.Path, resolved.WorkspaceJail))
	al.RegisterTool(tools.NewTerminalCommandTool(target.Path, resolved.BlockedCommands, nil))
	al.RegisterTool(tools.NewDiffReplaceTool(target.Path, resolved.WorkspaceJail))
	al.RegisterTool(tools.NewRenameFileTool(target.Path, resolved.WorkspaceJail))
	al.RegisterTool(tools.NewDeleteFileTool(target.Path, resolved.WorkspaceJail))
	al.RegisterTool(tools.NewTreeTool(target.Path, resolved.WorkspaceJail))
	al.RegisterTool(tools.NewGrepTool(target.Path, resolved.WorkspaceJail))
	al.RegisterTool(tools.NewPortMonitorTool(target.Path))
	al.RegisterTool(tools.NewGitStatusTool(target.Path))
	al.RegisterTool(tools.NewGitLogTool(target.Path))
	al.RegisterTool(tools.NewGitDiffTool(target.Path))
	al.RegisterTool(tools.NewGitAddTool(target.Path))
	al.RegisterTool(tools.NewGitCommitTool(target.Path))
	al.RegisterTool(tools.NewGitBranchTool(target.Path))
	al.RegisterTool(tools.NewGitConflictTool(target.Path, resolved.WorkspaceJail))
	al.RegisterTool(tools.NewHTTPClientTool(target.Path))
	al.RegisterTool(tools.NewScraperTool(target.Path))
	al.RegisterTool(tools.NewBrowserTool(target.Path, resolved.BrowserHeadless))
	al.RegisterTool(tools.NewComputerControlTool(target.Path))
	al.RegisterTool(tools.NewDatabaseTesterTool(target.Path))
	al.RegisterTool(tools.NewProxyTool(target.Path, resolved.WorkspaceJail))
	al.RegisterTool(tools.NewRunTestsTool(target.Path))


	al.RegisterTool(tools.NewStackTranslatorTool(target.Path, resolved.WorkspaceJail))
	al.RegisterTool(tools.NewDocGeneratorTool(target.Path, resolved.WorkspaceJail))
	al.RegisterTool(tools.NewCodeExplainerTool(target.Path, resolved.WorkspaceJail))
	al.RegisterTool(tools.NewMockGeneratorTool(target.Path, resolved.WorkspaceJail))
	al.RegisterTool(tools.NewComplexityReducerTool(target.Path, resolved.WorkspaceJail))
	al.RegisterTool(tools.NewMemoryLeakScannerTool(target.Path, resolved.WorkspaceJail))
	if tp, ok := handler.(interface {
		GetCustomTools() []tools.Tool
	}); ok {
		for _, t := range tp.GetCustomTools() {
			al.RegisterTool(t)
		}
	}

	// Carrega ferramentas dinâmicas da pasta .crom/tools do workspace
	dynamicToolsDir := filepath.Join(target.Path, ".crom", "tools")
	if loadedTools, err := tools.LoadScriptsFromDir(dynamicToolsDir); err == nil {
		for _, t := range loadedTools {
			al.RegisterTool(t)
		}
	}

	al.SetPermissionManager(pm)



	agent := &RunningAgent{
		WorkspaceName: workspaceName,
		Task:          task,
		Cancel:        cancel,
		Loop:          al,
		Ctx:           agentCtx,
	}
	m.runningAgents[workspaceName] = agent

	// 8. Executa em background
	go func() {
		defer func() {
			m.mu.Lock()
			delete(m.runningAgents, workspaceName)
			m.mu.Unlock()
		}()

		err := al.Execute(agentCtx, task)
		if err != nil {
			handler.OnStatusChange(fmt.Sprintf("error: %v", err))
		}
	}()

	return nil
}

// StopAgent cancela a execução de um agente em execução de fundo
func (m *MultiAgentManager) StopAgent(workspaceName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	agent, running := m.runningAgents[workspaceName]
	if !running {
		return fmt.Errorf("nenhum agente ativo em execução no workspace '%s'", workspaceName)
	}

	agent.Cancel()
	delete(m.runningAgents, workspaceName)
	return nil
}

// ListRunningAgents retorna as instâncias de agentes ativos atualmente
func (m *MultiAgentManager) ListRunningAgents() []*RunningAgent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agents := make([]*RunningAgent, 0, len(m.runningAgents))
	for _, agent := range m.runningAgents {
		agents = append(agents, agent)
	}
	return agents
}
