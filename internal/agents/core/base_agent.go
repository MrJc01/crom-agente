package core

import (
	"errors"

	"github.com/crom/crom-agente/internal/llm"
)

// BaseAgent fornece uma implementação base compartilhada para especialistas nativos
type BaseAgent struct {
	AgentName        string
	AgentDescription string
	AgentSysPrompt   string
	LLMProvider      llm.Provider
	AllowedToolIDs   []string
}

// Name retorna o nome do especialista
func (b *BaseAgent) Name() string {
	return b.AgentName
}

// Description retorna a descrição do especialista
func (b *BaseAgent) Description() string {
	return b.AgentDescription
}

// SystemPrompt retorna o prompt de sistema do especialista
func (b *BaseAgent) SystemPrompt() string {
	return b.AgentSysPrompt
}

// ToolIDs retorna as IDs das ferramentas associadas a este especialista
func (b *BaseAgent) ToolIDs() []string {
	return b.AllowedToolIDs
}

// Provider retorna a instância do provedor LLM configurada
func (b *BaseAgent) Provider() llm.Provider {
	return b.LLMProvider
}

// Validate verifica se o BaseAgent está configurado corretamente
func (b *BaseAgent) Validate() error {
	if b.AgentName == "" {
		return errors.New("nome do agente não pode ser vazio")
	}
	if b.LLMProvider == nil {
		return errors.New("provedor LLM não configurado")
	}
	return nil
}
