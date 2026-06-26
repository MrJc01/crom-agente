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

	"github.com/crom/crom-agente/internal/agents"
	agentscore "github.com/crom/crom-agente/internal/agents/core"
	browserSpecialist "github.com/crom/crom-agente/internal/agents/specialists/browser"
	"github.com/crom/crom-agente/internal/agents/specialists/external"
	_ "github.com/crom/crom-agente/internal/agents/specialists/finalizer"
	_ "github.com/crom/crom-agente/internal/agents/specialists/reasoning"
	_ "github.com/crom/crom-agente/internal/agents/specialists/spawn"
	_ "github.com/crom/crom-agente/internal/agents/specialists/tester"
	"github.com/crom/crom-agente/internal/config"
	"github.com/crom/crom-agente/internal/config/topology"
	"github.com/crom/crom-agente/internal/llm/providers"
	"github.com/crom/crom-agente/internal/loop"
	"github.com/crom/crom-agente/internal/loop/agentic/core"
	"github.com/crom/crom-agente/internal/permission"
	"github.com/crom/crom-agente/internal/state"
	"github.com/crom/crom-agente/internal/tools"
	"github.com/crom/crom-agente/internal/tools/browser"
	"github.com/crom/crom-agente/internal/tools/registry"
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
	Loop          *core.AgenticLoop
	Ctx           context.Context
}

// MultiAgentManager coordena a execução simultânea de loops em múltiplos workspaces
type MultiAgentManager struct {
	mu                sync.RWMutex
	runningAgents     map[string]*RunningAgent // chave: workspace name
	activeBrowsers    map[string]*browser.BrowserTool
	activeSpecialists map[string]agentscore.Agent
	MCPManager        *MCPManager // gerenciador de servidores MCP globais
	OnSchedule        func(workspaceName, sessionName, task string, delaySecs int, provider, model string)
	OnBackgroundExit  func(workspaceName, sessionName, task string, provider, model string)
}

// NewMultiAgentManager cria um novo gerenciador multi-agente
func NewMultiAgentManager() *MultiAgentManager {
	m := &MultiAgentManager{
		runningAgents:     make(map[string]*RunningAgent),
		activeBrowsers:    make(map[string]*browser.BrowserTool),
		activeSpecialists: make(map[string]agentscore.Agent),
		MCPManager:        NewMCPManager(),
	}
	go m.startHeartbeatCleanup()
	return m
}

func (m *MultiAgentManager) startHeartbeatCleanup() {
	ticker := time.NewTicker(5 * time.Second)
	for range ticker.C {
		m.mu.Lock()
		if len(m.runningAgents) == 0 {
			for name, b := range m.activeBrowsers {
				b.Close()
				delete(m.activeBrowsers, name)
			}
			for name, agentInst := range m.activeSpecialists {
				if closer, ok := agentInst.(interface{ Close() }); ok {
					closer.Close()
				}
				delete(m.activeSpecialists, name)
			}
		} else {
			for name, b := range m.activeBrowsers {
				if _, running := m.runningAgents[name]; !running {
					b.Close()
					delete(m.activeBrowsers, name)
				}
			}
			for name, agentInst := range m.activeSpecialists {
				parts := strings.Split(name, "/")
				if len(parts) > 0 {
					wsName := parts[0]
					if _, running := m.runningAgents[wsName]; !running {
						if closer, ok := agentInst.(interface{ Close() }); ok {
							closer.Close()
						}
						delete(m.activeSpecialists, name)
					}
				}
			}
		}
		m.mu.Unlock()
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
func (m *MultiAgentManager) StartAgent(ctx context.Context, workspaceName, sessionName, task string, handler core.EventHandler) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 1. Carrega workspaces registrados
	workspaces, err := LoadWorkspaces()
	if err != nil {
		return err
	}

	var target *Workspace
	for i := range workspaces {
		if workspaces[i].Name == workspaceName || workspaces[i].Path == workspaceName {
			target = &workspaces[i]
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
			if err := m.AddWorkspace(name, workspaceName); err != nil {
				return fmt.Errorf("falha ao auto-registrar workspace '%s': %w", workspaceName, err)
			}
			// Recarrega a lista
			if updatedList, errLoad := LoadWorkspaces(); errLoad == nil {
				for i := range updatedList {
					if updatedList[i].Path == workspaceName || updatedList[i].Name == name {
						target = &updatedList[i]
						break
					}
				}
			} else {
				return fmt.Errorf("falha ao recarregar workspaces após registro: %w", errLoad)
			}
		} else {
			return fmt.Errorf("caminho do workspace não existe ou não é um diretório: %s", workspaceName)
		}
	}
	if target == nil {
		return fmt.Errorf("workspace '%s' não encontrado no registro", workspaceName)
	}

	wsName := target.Name

	// 2. Verifica se já está rodando
	if _, running := m.runningAgents[wsName]; running {
		return fmt.Errorf("já existe um agente em execução no workspace '%s'", wsName)
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
	// Mescla as chaves secretas e envs locais do projeto se existirem
	if localEnv, err := config.LoadEnvVars(target.Path); err == nil {
		for k, v := range localEnv.All() {
			env.Set(k, v)
		}
	}
	if localCromEnv, err := config.LoadEnvVars(filepath.Join(target.Path, ".crom")); err == nil {
		for k, v := range localCromEnv.All() {
			env.Set(k, v)
		}
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
	if maxIterOverride, ok := ctx.Value("max_iterations_override").(int); ok {
		resolved.MaxIterations = maxIterOverride
	}
	if maxConsecutiveFailOverride, ok := ctx.Value("max_consecutive_fail_override").(int); ok {
		resolved.MaxConsecutiveFail = maxConsecutiveFailOverride
	}
	if toolTimeoutOverride, ok := ctx.Value("tool_timeout_seconds_override").(int); ok {
		resolved.ToolTimeoutSeconds = toolTimeoutOverride
	}
	if headlessOverride, ok := ctx.Value("browser_headless_override").(bool); ok {
		resolved.BrowserHeadless = headlessOverride
	}
	if disableOptOverride, ok := ctx.Value("disable_prompt_optimization_override").(bool); ok {
		resolved.DisablePromptOptimization = disableOptOverride
	}

	// 4. Instancia LLM Provider
	provider, err := providers.NewProvider(resolved.Provider, resolved.Model, func(key string) string {
		return env.Get(key)
	})
	if err != nil {
		return err
	}

	// 5. Instancia StateManager (com isolamento por worker - Task 78)
	storageDir := filepath.Join(target.Path, ".crom")
	if sessionName == "" {
		sessionName = fmt.Sprintf("session-%d", time.Now().UnixNano())
	}
	// Se o sessionName contém indicador de worker, isolar em subpasta dedicada
	if strings.Contains(sessionName, "worker_") || strings.Contains(sessionName, "bench_") {
		storageDir = filepath.Join(target.Path, ".crom", "workers", sessionName)
		_ = os.MkdirAll(storageDir, 0755)
	}
	sm := state.NewSessionStateManager(storageDir, sessionName)
	if err := sm.LoadState(); err != nil {
		return err
	}

	// 6. Cria context cancelável
	agentCtx, cancel := context.WithCancel(ctx)
	agentCtx = context.WithValue(agentCtx, "session_name", sessionName)
	agentCtx = context.WithValue(agentCtx, "token_recorder_callback", func(tokens int) {
		if sm != nil {
			_ = sm.RecordTokens(tokens)
		}
	})

	// 7. Inicializa o PermissionManager
	askFunc := func(ctx context.Context, action, target string) (bool, bool) {
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
		AskPermission(ctx context.Context, action, target string) (bool, bool)
	}); ok {
		askFunc = pr.AskPermission
	} else if pr, ok := handler.(interface {
		AskPermission(action, target string) (bool, bool)
	}); ok {
		askFunc = func(ctx context.Context, action, target string) (bool, bool) {
			return pr.AskPermission(action, target)
		}
	}
	pm := permission.NewPermissionManager(target.Path, resolved.PermissionMode, askFunc)

	// 8. Cria AgenticLoop
	al := core.New(provider, sm, handler, resolved)

	if m.activeBrowsers == nil {
		m.activeBrowsers = make(map[string]*browser.BrowserTool)
	}
	if m.activeSpecialists == nil {
		m.activeSpecialists = make(map[string]agentscore.Agent)
	}

	browserTool, ok := m.activeBrowsers[wsName]
	if !ok {
		browserTool = browser.NewBrowserTool(target.Path, resolved.BrowserHeadless)
		m.activeBrowsers[wsName] = browserTool
	}
	browserTool.SetOnNavigate(func(url string) {
		_ = sm.SetBrowserURL(url)
		if handler != nil {
			handler.OnEvent(loop.AgentEvent{
				Timestamp: time.Now(),
				Event:     "browser_navigate",
				Data: map[string]interface{}{
					"url": url,
				},
			})
		}
	})
	browserTool.SetRestoreURL(func() string {
		return sm.GetBrowserURL()
	})

	// Instanciar e registrar as ferramentas nativas unificadas via registro centralizado
	builtinTools := registry.GetBuiltinTools(registry.RegistrationConfig{
		WorkspacePath:   target.Path,
		WorkspaceJail:   resolved.WorkspaceJail,
		BlockedCommands: resolved.BlockedCommands,
		TerminalOutput:  nil,
		OnSchedule: func(task string, durationSeconds int) {
			if m.OnSchedule != nil {
				m.OnSchedule(workspaceName, sessionName, task, durationSeconds, resolved.Provider, resolved.Model)
			}
		},
		OnBackgroundExit: func(bgID, cmdStr, logs string, success bool) {
			if m.OnBackgroundExit != nil {
				taskMsg := fmt.Sprintf("O comando em background '%s' (ID: %s) terminou com sucesso=%t. Verifique a saída e os logs de execução para responder ao usuário. Logs:\n%s", cmdStr, bgID, success, logs)
				m.OnBackgroundExit(workspaceName, sessionName, taskMsg, resolved.Provider, resolved.Model)
			}
		},
		BrowserTool:  browserTool,
		SubagentTool: nil,
		StateManager: sm,
		LLMProvider:  provider,
	})

	for _, t := range builtinTools {
		al.RegisterTool(t)
	}

	// Carrega a topologia de agentes dinâmicos
	topoPath := filepath.Join(target.Path, ".crom", config.AgentsTopologyFile)
	topo, err := topology.LoadTopology(topoPath)
	if err != nil {
		topo = topology.GetDefaultTopology()
	}

	for _, spec := range topo.GetSpecialists() {
		if spec.Type == "native" {
			cacheKey := wsName + "/" + spec.Name
			agentInst, exists := m.activeSpecialists[cacheKey]
			if !exists {
				var ok bool
				specProvider := provider
				if spec.Model != "" && spec.Model != resolved.Model {
					if p, err := providers.NewProvider(resolved.Provider, spec.Model, func(key string) string {
						return env.Get(key)
					}); err == nil {
						specProvider = p
					}
				}
				agentInst, ok = agents.GetAgentInst(spec.Name, agents.Config{
					WorkspacePath:   target.Path,
					LLMProvider:     specProvider,
					BrowserHeadless: resolved.BrowserHeadless,
				})
				if !ok {
					continue
				}
				m.activeSpecialists[cacheKey] = agentInst
			}

			// Injeta callbacks específicos do browser
			if spec.Name == "browser" || spec.Name == "browser_subagent" {
				if ba, ok := agentInst.(*browserSpecialist.BrowserAgent); ok {
					ba.SetOnNavigate(func(url string) {
						_ = sm.SetBrowserURL(url)
						if handler != nil {
							handler.OnEvent(loop.AgentEvent{
								Timestamp: time.Now(),
								Event:     "browser_navigate",
								Data: map[string]interface{}{
									"url": url,
								},
							})
						}
					})
					ba.SetRestoreURL(func() string {
						return sm.GetBrowserURL()
					})
				}
			}

			al.RegisterTool(tools.NewAgentToolAdapter(agentInst))
		} else if spec.Type == "external" {
			cacheKey := wsName + "/" + spec.Name
			agentInst, exists := m.activeSpecialists[cacheKey]
			if !exists {
				agentInst = external.NewExternalAgent(
					spec.Name,
					spec.Description,
					spec.SystemPrompt,
					spec.ToolIDs,
					spec.ExecPath,
					spec.Args,
					30*time.Second,
				)
				m.activeSpecialists[cacheKey] = agentInst
			}
			al.RegisterTool(tools.NewAgentToolAdapter(agentInst))
		}
	}
	if tp, ok := handler.(interface {
		GetCustomTools() []tools.Tool
	}); ok {
		for _, t := range tp.GetCustomTools() {
			al.RegisterTool(t)
		}
	}

	// Registrar ferramentas dos servidores MCP globais
	if m.MCPManager != nil {
		for _, t := range m.MCPManager.GetAllTools() {
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
		WorkspaceName: wsName,
		Task:          task,
		Cancel:        cancel,
		Loop:          al,
		Ctx:           agentCtx,
	}
	m.runningAgents[wsName] = agent

	// 8. Executa em background
	go func() {
		defer func() {
			m.mu.Lock()
			delete(m.runningAgents, wsName)
			m.mu.Unlock()
		}()

		err := al.Execute(agentCtx, task)
		if err != nil {
			handler.OnStatusChange(fmt.Sprintf("error: %v", err))
		}
	}()

	return nil
}

// StopAgent cancela a execução de um agente em execução de fundo e fecha o navegador
func (m *MultiAgentManager) StopAgent(workspaceName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	wsName := m.ResolveWorkspaceName(workspaceName)

	agent, running := m.runningAgents[wsName]
	if running {
		agent.Cancel()
		delete(m.runningAgents, wsName)
	}

	closedBrowser := false
	if b, ok := m.activeBrowsers[wsName]; ok {
		b.Close()
		delete(m.activeBrowsers, wsName)
		closedBrowser = true
	}
	for k, agentInst := range m.activeSpecialists {
		if strings.HasPrefix(k, wsName+"/") {
			if closer, ok := agentInst.(interface{ Close() }); ok {
				closer.Close()
			}
			delete(m.activeSpecialists, k)
			closedBrowser = true
		}
	}

	if !running && !closedBrowser {
		return fmt.Errorf("nenhum agente ou navegador ativo no workspace '%s'", workspaceName)
	}
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

// InitMCPFromConfig lê a configuração global (~/.crom/global.json) e inicializa todos os servidores MCP configurados.
// Deve ser chamado uma vez na inicialização do daemon, antes de aceitar sessões.
func (m *MultiAgentManager) InitMCPFromConfig(ctx context.Context) error {
	gDir, err := config.GlobalDir()
	if err != nil {
		return fmt.Errorf("falha ao obter diretório global: %w", err)
	}

	global, err := config.LoadGlobalConfig(gDir)
	if err != nil {
		return fmt.Errorf("falha ao carregar config global: %w", err)
	}

	if len(global.MCPServers) == 0 {
		return nil // Nenhum servidor MCP configurado — OK
	}

	if m.MCPManager == nil {
		m.MCPManager = NewMCPManager()
	}

	m.MCPManager.StartAll(ctx, global.MCPServers)
	return nil
}

// Shutdown encerra todos os agentes ativos e os servidores MCP de forma graciosa
func (m *MultiAgentManager) Shutdown() {
	m.mu.Lock()
	for name, agent := range m.runningAgents {
		agent.Cancel()
		delete(m.runningAgents, name)
	}
	for name, b := range m.activeBrowsers {
		b.Close()
		delete(m.activeBrowsers, name)
	}
	for name, agentInst := range m.activeSpecialists {
		if closer, ok := agentInst.(interface{ Close() }); ok {
			closer.Close()
		}
		delete(m.activeSpecialists, name)
	}
	m.mu.Unlock()

	if m.MCPManager != nil {
		m.MCPManager.StopAll()
	}
}

// ResolveWorkspaceName mapeia uma chave/caminho de workspace de volta ao nome registrado
func (m *MultiAgentManager) ResolveWorkspaceName(key string) string {
	workspaces, err := LoadWorkspaces()
	if err == nil {
		for _, ws := range workspaces {
			if ws.Name == key || ws.Path == key {
				return ws.Name
			}
		}
	}
	return key
}

// GetBrowserPageContent retorna o HTML e a URL ativa do browser do workspace
func (m *MultiAgentManager) GetBrowserPageContent(workspaceKey string) (string, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	wsName := m.ResolveWorkspaceName(workspaceKey)

	// Tenta subagent primeiro, depois browser normal
	if spec, ok := m.activeSpecialists[wsName+"/browser"]; ok {
		if sub, ok := spec.(interface {
			GetCurrentPageContent() (string, string, error)
		}); ok {
			html, url, err := sub.GetCurrentPageContent()
			if err == nil && html != "" {
				return html, url, nil
			}
		}
	}

	if b, ok := m.activeBrowsers[wsName]; ok {
		html, url, err := b.GetCurrentPageContent()
		if err == nil && html != "" {
			return html, url, nil
		}
	}

	return "", "", fmt.Errorf("nenhum navegador ativo encontrado para o workspace %s", workspaceKey)
}

// InjectUserMessage insere uma mensagem de usuário em tempo real no loop de um agente ativo
func (m *MultiAgentManager) InjectUserMessage(workspaceName, content string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	wsName := m.ResolveWorkspaceName(workspaceName)
	agent, running := m.runningAgents[wsName]
	if !running || agent == nil {
		return fmt.Errorf("nenhum agente ativo no workspace '%s'", workspaceName)
	}

	agent.Loop.QueueUserMessage(content)
	return nil
}

// BrowserTelemetry representa o estado do navegador na telemetria
type BrowserTelemetry struct {
	Active bool   `json:"active"`
	URL    string `json:"url"`
}

// AgentTelemetry encapsula a telemetria consolidada de um agente
type AgentTelemetry struct {
	WorkspaceName string             `json:"workspace_name"`
	IsRunning     bool               `json:"is_running"`
	AgentState    state.AgentState   `json:"agent_state"`
	Browser       BrowserTelemetry   `json:"browser"`
	MCPServers    []MCPServerStatus  `json:"mcp_servers"`
}

// GetAgentTelemetry consolida a telemetria do agente de um determinado workspace
func (m *MultiAgentManager) GetAgentTelemetry(workspaceName string) (*AgentTelemetry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	wsName := m.ResolveWorkspaceName(workspaceName)

	// Localizar o workspace cadastrado para saber o caminho físico
	workspaces, err := LoadWorkspaces()
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar workspaces: %w", err)
	}

	var target *Workspace
	for i := range workspaces {
		if workspaces[i].Name == wsName {
			target = &workspaces[i]
			break
		}
	}

	// Se não achou por nome, talvez o workspaceName passado seja o caminho do diretório
	if target == nil {
		for i := range workspaces {
			if workspaces[i].Path == workspaceName {
				target = &workspaces[i]
				wsName = workspaces[i].Name
				break
			}
		}
	}

	if target == nil {
		return nil, fmt.Errorf("workspace '%s' não encontrado", workspaceName)
	}

	telemetry := &AgentTelemetry{
		WorkspaceName: wsName,
		MCPServers:    []MCPServerStatus{},
	}

	// 1. Verificar se está rodando no momento
	agent, running := m.runningAgents[wsName]
	telemetry.IsRunning = running

	// 2. Obter estado do agente
	if running && agent.Loop != nil {
		sm := agent.Loop.GetStateManager()
		if sm != nil {
			telemetry.AgentState = sm.GetState()
		}
	} else {
		// Se não está rodando, tenta carregar o último estado salvo
		storageDir := filepath.Join(target.Path, ".crom")
		sm := state.NewStateManager(storageDir)
		if err := sm.LoadState(); err == nil {
			telemetry.AgentState = sm.GetState()
		}
	}

	// 3. Obter status do navegador
	if b, ok := m.activeBrowsers[wsName]; ok {
		telemetry.Browser.Active = true
		_, url, err := b.GetCurrentPageContent()
		if err == nil {
			telemetry.Browser.URL = url
		} else {
			telemetry.Browser.URL = telemetry.AgentState.BrowserURL
		}
	} else {
		telemetry.Browser.Active = false
		telemetry.Browser.URL = telemetry.AgentState.BrowserURL
	}

	// 4. Obter status dos servidores MCP
	if m.MCPManager != nil {
		telemetry.MCPServers = m.MCPManager.GetServerStatus()
	}

	return telemetry, nil
}
