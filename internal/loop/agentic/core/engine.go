package core

import (
	"context"
	"sync"
	"time"

	"github.com/crom/crom-agente/internal/config"
	"github.com/crom/crom-agente/internal/llm"
	"github.com/crom/crom-agente/internal/loop"
	"github.com/crom/crom-agente/internal/state"
	"github.com/crom/crom-agente/internal/tools"
)

// EventHandler permite que o chamador receba notificações do loop (CLI, SDK, etc.)
type EventHandler interface {
	OnStatusChange(status string)
	OnMessage(role string, content string)
	OnEvent(event loop.AgentEvent) // Eventos estruturados com metadados completos
}

// noopHandler é um handler vazio usado quando nenhum handler é fornecido
type noopHandler struct{}

func (n noopHandler) OnStatusChange(string)    {}
func (n noopHandler) OnMessage(string, string) {}
func (n noopHandler) OnEvent(loop.AgentEvent)  {}

type fastPathCacheEntry struct {
	response  string
	expiresAt time.Time
}

// AgenticLoop é o motor de execução do agente seguindo o padrão ReAct
type AgenticLoop struct {
	provider          llm.Provider
	tools             map[string]tools.Tool
	stateManager      *state.StateManager
	handler           EventHandler
	config            *config.ResolvedConfig
	permissionManager interface {
		Authorize(ctx context.Context, action, target string) (bool, error)
	}
	promptManager       *config.PromptManager
	mu                  sync.Mutex
	pendingUserMessages []string
	failureRetryDelay   time.Duration
	textOnlyMode        bool
	fastPathCache       map[string]fastPathCacheEntry
	fastPathCacheMu     sync.Mutex
	linterFailures      map[string]int
}

// QueueUserMessage adiciona uma mensagem do usuário na fila de injeção em tempo real
func (al *AgenticLoop) QueueUserMessage(content string) {
	al.mu.Lock()
	defer al.mu.Unlock()
	al.pendingUserMessages = append(al.pendingUserMessages, content)
}

// GetStateManager retorna o gerenciador de estado associado ao loop
func (al *AgenticLoop) GetStateManager() *state.StateManager {
	return al.stateManager
}

// SetPermissionManager define o gerenciador de permissões
func (al *AgenticLoop) SetPermissionManager(pm interface {
	Authorize(ctx context.Context, action, target string) (bool, error)
}) {
	al.permissionManager = pm
}

// New cria uma nova instância do AgenticLoop
func New(provider llm.Provider, sm *state.StateManager, handler EventHandler, cfg ...*config.ResolvedConfig) *AgenticLoop {
	if handler == nil {
		handler = noopHandler{}
	}

	var resolvedCfg *config.ResolvedConfig
	if len(cfg) > 0 && cfg[0] != nil {
		resolvedCfg = cfg[0]
	} else {
		// Defaults hardcoded para backward compatibility
		resolvedCfg = &config.ResolvedConfig{
			MaxIterations:                0,
			MaxConsecutiveFail:           3,
			ToolTimeoutSeconds:           30,
			MaxMessageHistory:            40,
			AutoVerify:                   true,
			PermissionMode:               "scoped",
			DisablePromptOptimization:    true, // Disables by default in tests that don't pass a custom config!
			ConsecutiveFailureRetry:      true,
			ConsecutiveFailureRetryLimit: 0,
			ConsecutiveFailureRetryDelay: 5,
		}
	}

	var pm *config.PromptManager
	if sm != nil {
		pm = config.NewPromptManager(sm.GetWorkspaceDir())
	}

	delay := 5 * time.Second
	if resolvedCfg.ConsecutiveFailureRetryDelay > 0 {
		delay = time.Duration(resolvedCfg.ConsecutiveFailureRetryDelay) * time.Second
	}

	return &AgenticLoop{
		provider:          provider,
		tools:             make(map[string]tools.Tool),
		stateManager:      sm,
		handler:           handler,
		config:            resolvedCfg,
		promptManager:     pm,
		failureRetryDelay: delay,
		fastPathCache:     make(map[string]fastPathCacheEntry),
		linterFailures:    make(map[string]int),
	}
}

// RegisterTool registra uma ferramenta disponível para o agente
func (al *AgenticLoop) RegisterTool(t tools.Tool) {
	al.tools[t.ID()] = t
}

// GetTools retorna a lista de ferramentas registradas
func (al *AgenticLoop) GetTools() []tools.Tool {
	result := make([]tools.Tool, 0, len(al.tools))
	for _, t := range al.tools {
		result = append(result, t)
	}
	return result
}
