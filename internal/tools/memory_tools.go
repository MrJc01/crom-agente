package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/crom/crom-agente/internal/state"
)

// CoreMemoryAppendTool adiciona texto à memória central do agente
type CoreMemoryAppendTool struct {
	stateManager *state.StateManager
}

func NewCoreMemoryAppendTool(sm *state.StateManager) *CoreMemoryAppendTool {
	return &CoreMemoryAppendTool{stateManager: sm}
}

func (t *CoreMemoryAppendTool) ID() string {
	return "core_memory_append"
}

func (t *CoreMemoryAppendTool) Description() string {
	return "Anexa uma nova string de texto à sua memória central (Core Memory). Use isso para guardar fatos, intenções ou variáveis que você precisará lembrar nos próximos turnos."
}

func (t *CoreMemoryAppendTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"content": {
				"type": "string",
				"description": "Texto a ser anexado no final da memória central."
			}
		},
		"required": ["content"]
	}`)
}

func (t *CoreMemoryAppendTool) RequiresApproval() bool {
	return false
}

func (t *CoreMemoryAppendTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var params struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return Result{Success: false, Error: err.Error()}, fmt.Errorf("argumentos inválidos: %w", err)
	}

	if t.stateManager == nil {
		return Result{Success: false, Error: "state manager não configurado"}, fmt.Errorf("state manager não configurado")
	}

	current := t.stateManager.GetCoreMemory()
	newMem := current + "\n" + params.Content

	// Limitar tamanho para não estourar (Ex: 4000 bytes). Idealmente usaríamos config, mas faremos limite de fallback aqui
	if len(newMem) > 8000 {
		return Result{Success: false, Error: "memória central cheia (limite atingido)"}, fmt.Errorf("erro: memória central cheia (limite de 8000 caracteres atingido). Use core_memory_replace para limpar/resumir.")
	}

	if err := t.stateManager.SetCoreMemory(newMem); err != nil {
		return Result{Success: false, Error: err.Error()}, fmt.Errorf("falha ao salvar memória: %w", err)
	}

	return Result{Success: true, Data: "Texto anexado com sucesso na memória central."}, nil
}

// CoreMemoryReplaceTool substitui totalmente a memória central do agente
type CoreMemoryReplaceTool struct {
	stateManager *state.StateManager
}

func NewCoreMemoryReplaceTool(sm *state.StateManager) *CoreMemoryReplaceTool {
	return &CoreMemoryReplaceTool{stateManager: sm}
}

func (t *CoreMemoryReplaceTool) ID() string {
	return "core_memory_replace"
}

func (t *CoreMemoryReplaceTool) Description() string {
	return "Substitui completamente o conteúdo da sua memória central. Use isso quando a memória estiver cheia e você precisar reescrever um resumo condensado dos fatos."
}

func (t *CoreMemoryReplaceTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"content": {
				"type": "string",
				"description": "O novo texto completo que substituirá a memória central."
			}
		},
		"required": ["content"]
	}`)
}

func (t *CoreMemoryReplaceTool) RequiresApproval() bool {
	return false
}

func (t *CoreMemoryReplaceTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var params struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return Result{Success: false, Error: err.Error()}, fmt.Errorf("argumentos inválidos: %w", err)
	}

	if t.stateManager == nil {
		return Result{Success: false, Error: "state manager não configurado"}, fmt.Errorf("state manager não configurado")
	}

	if err := t.stateManager.SetCoreMemory(params.Content); err != nil {
		return Result{Success: false, Error: err.Error()}, fmt.Errorf("falha ao substituir memória: %w", err)
	}

	return Result{Success: true, Data: "Memória central substituída com sucesso."}, nil
}

// CoreMemorySearchTool busca no histórico de mensagens arquivadas
type CoreMemorySearchTool struct {
	stateManager *state.StateManager
}

func NewCoreMemorySearchTool(sm *state.StateManager) *CoreMemorySearchTool {
	return &CoreMemorySearchTool{stateManager: sm}
}

func (t *CoreMemorySearchTool) ID() string {
	return "archive_search"
}

func (t *CoreMemorySearchTool) Description() string {
	return "Busca no histórico de mensagens antigas da conversa (arquivadas) por uma palavra-chave. Use isso para lembrar detalhes que saíram do seu contexto imediato de trabalho."
}

func (t *CoreMemorySearchTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "Termo de busca (palavra-chave) para procurar no histórico."
			}
		},
		"required": ["query"]
	}`)
}

func (t *CoreMemorySearchTool) RequiresApproval() bool {
	return false
}

func (t *CoreMemorySearchTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var params struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return Result{Success: false, Error: err.Error()}, fmt.Errorf("argumentos inválidos: %w", err)
	}

	if t.stateManager == nil {
		return Result{Success: false, Error: "state manager não configurado"}, fmt.Errorf("state manager não configurado")
	}

	messages := t.stateManager.GetMessages()
	var results []string

	queryTokens := strings.Fields(strings.ToLower(params.Query))
	if len(queryTokens) == 0 {
		return Result{Success: false, Error: "query vazia"}, fmt.Errorf("query vazia")
	}

	for i, msg := range messages {
		if len(msg.Content) > 0 {
			lowerContent := strings.ToLower(msg.Content)
			allMatch := true
			for _, token := range queryTokens {
				if !strings.Contains(lowerContent, token) {
					allMatch = false
					break
				}
			}
			if allMatch {
				snippet := msg.Content
				if len(snippet) > 300 {
					snippet = snippet[:300] + "..."
				}
				results = append(results, fmt.Sprintf("Turno %d [%s]: %s", i, msg.Role, snippet))
			}
		}
	}

	if len(results) == 0 {
		return Result{Success: true, Data: "Nenhum resultado encontrado no histórico para a query: " + params.Query}, nil
	}

	return Result{Success: true, Data: "Resultados da busca no arquivo morto (Archive Search):\n" + strings.Join(results, "\n\n")}, nil
}
