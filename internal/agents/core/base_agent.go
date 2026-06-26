package core

import (
	"errors"
	"fmt"

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

// SystemPrompt retorna o prompt de sistema do especialista, injetando as ferramentas autorizadas
func (b *BaseAgent) SystemPrompt() string {
	if len(b.AllowedToolIDs) == 0 {
		return b.AgentSysPrompt + "\n\nNota: Você NÃO tem permissão para chamar ferramentas nesta execução. Confie inteiramente no seu raciocínio lógico."
	}
	toolsList := ""
	for _, id := range b.AllowedToolIDs {
		toolsList += "- " + id + "\n"
	}
	return fmt.Sprintf("%s\n\n[FERRAMENTAS AUTORIZADAS]\nVocê tem permissão para utilizar as seguintes ferramentas:\n%s\nUse-as conforme necessário para atingir o objetivo.", b.AgentSysPrompt, toolsList)
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
