package agents

import (
	"sync"

	"github.com/crom/crom-agente/internal/agents/core"
	"github.com/crom/crom-agente/internal/llm"
)

// Config define o contexto/configuração passado a cada especialista na criação
type Config struct {
	WorkspacePath   string
	LLMProvider     llm.Provider
	BrowserHeadless bool
}

// AgentFactory representa uma função que constrói um core.Agent especialista a partir do Config
type AgentFactory func(cfg Config) core.Agent

var (
	registryMu sync.RWMutex
	registry   = make(map[string]AgentFactory)
)

// RegisterAgent registra um agente especialista nativo globalmente no ecossistema
func RegisterAgent(name string, factory AgentFactory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[name] = factory
}

// GetAgentInst busca e instacia uma nova cópia do agente especialista pelo nome
func GetAgentInst(name string, cfg Config) (core.Agent, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	factory, ok := registry[name]
	if !ok {
		return nil, false
	}
	return factory(cfg), true
}

// GetRegisteredAgents lista todos os nomes dos especialistas nativos registrados
func GetRegisteredAgents() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	list := make([]string, 0, len(registry))
	for k := range registry {
		list = append(list, k)
	}
	return list
}
