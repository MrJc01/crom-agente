/*
Package core implementa o motor central de execução ReAct do crom-agente.
O AgenticLoop é responsável por gerenciar iterações de chamadas de LLM,
rastreamento de estado através do StateManager, parseamento de chamadas de ferramentas,
e a orquestração segura do ambiente.

Principais responsabilidades:
- Injeção contextual (Árvore de diretórios, regras locais, memória de erros)
- Parseamento robusto multimodais (Texto puro vs chamadas nativas)
- Limites agressivos e circuit breakers (early stopping por OOM ou Loops)
*/
package core

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/crom/crom-agente/internal/config"
	"github.com/crom/crom-agente/internal/llm"
	"github.com/crom/crom-agente/internal/loop"
	"github.com/crom/crom-agente/internal/memory/graph"
	"github.com/crom/crom-agente/internal/state"
	"github.com/crom/crom-agente/internal/tools"
)

// EventHandler permite que o chamador receba notificações do loop (CLI, SDK, etc.)
type EventHandler interface {
	OnStatusChange(status string)
	OnMessage(role string, content string)
	OnStreamChunk(chunk string)
	OnEvent(event loop.AgentEvent) // Eventos estruturados com metadados completos
}

// noopHandler é um handler vazio usado quando nenhum handler é fornecido
type noopHandler struct{}

func (n noopHandler) OnStatusChange(string)    {}
func (n noopHandler) OnMessage(string, string) {}
func (n noopHandler) OnStreamChunk(string)     {}
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
	mistakeMemory       *MistakeMemory
	timelineMemory      *TimelineMemory
	GraphStore          graph.Store
	startTime           time.Time
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

	var gStore graph.Store
	if resolvedCfg.CognitiveArchitecture.KnowledgeGraph.Enabled {
		storePath := "knowledge_graph.json"
		if resolvedCfg.CognitiveArchitecture.KnowledgeGraph.StorageType == "sqlite_disk" {
			storePath = "knowledge_graph.db"
		}
		if sm != nil {
			storePath = filepath.Join(sm.GetWorkspaceDir(), ".crom", storePath)
		}
		gs, err := graph.NewStore(resolvedCfg.CognitiveArchitecture.KnowledgeGraph.StorageType, storePath)
		if err == nil {
			gStore = gs
		}
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
		mistakeMemory:     NewMistakeMemory(20),
		timelineMemory:    NewTimelineMemory(10),
		GraphStore:        gStore,
	}
}

// RegisterTool registra uma ferramenta disponível para o agente
func (al *AgenticLoop) RegisterTool(t tools.Tool) {
	al.tools[t.ID()] = t
}

// extractFactsAsync é a rotina assíncrona (Fase 3) que escuta a comunicação
// e extrai tripletos de conhecimento no background.
func (al *AgenticLoop) extractFactsAsync(content string) {
	if al.GraphStore == nil {
		return
	}

	// Criamos um contexto com timeout para não prender recursos da goroutine
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	prompt := fmt.Sprintf(`Extraia os principais fatos técnicos do texto abaixo no formato de Tripletos (Sujeito, Predicado, Objeto).
Retorne APENAS um array JSON. Exemplo: [{"subject":"App","predicate":"usa","object":"SQLite"}]

Texto:
%s`, content)

	opts := llm.RequestOptions{
		MaxTokens: func() *int { t := 1000; return &t }(),
	}

	resp, err := al.provider.SendMessages(ctx, []llm.Message{{Role: "user", Content: prompt}}, opts)
	if err != nil || resp == nil {
		return // Silencioso no background
	}

	// Limpar possíveis blocos markdown
	jsonStr := resp.Message.Content
	startIdx := strings.Index(jsonStr, "[")
	endIdx := strings.LastIndex(jsonStr, "]")
	if startIdx != -1 && endIdx != -1 && endIdx >= startIdx {
		jsonStr = jsonStr[startIdx : endIdx+1]
	}

	var triplets []graph.Triplet
	if err := json.Unmarshal([]byte(jsonStr), &triplets); err == nil {
		for _, t := range triplets {
			_ = al.GraphStore.SaveTriplet(context.Background(), t)
		}
	}
}

// GetTools retorna a lista de ferramentas registradas
func (al *AgenticLoop) GetTools() []tools.Tool {
	result := make([]tools.Tool, 0, len(al.tools))
	for _, t := range al.tools {
		result = append(result, t)
	}
	return result
}

// Execute é o ponto de entrada principal. Se a decomposição estrutural estiver ativada, delega para o PlannerLoop.
// Caso contrário, executa o motor ReAct tradicional.
func (al *AgenticLoop) Execute(ctx context.Context, intent string) error {
	if al.config != nil && al.config.CognitiveArchitecture.StructuralDecomposition {
		planner := NewPlannerLoop(al)
		return planner.Execute(ctx, intent)
	}
	return al.executeCoreLoop(ctx, intent)
}
