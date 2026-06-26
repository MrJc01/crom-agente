package core

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// MistakeEntry registra tentativas e erros para uma ação específica
type MistakeEntry struct {
	Action      string    `json:"action"`       // nome da ferramenta
	Target      string    `json:"target"`       // arquivo ou recurso alvo
	Attempts    []string  `json:"attempts"`     // descrição de cada tentativa
	Errors      []string  `json:"errors"`       // erros obtidos
	LastAttempt time.Time `json:"last_attempt"` // timestamp da última tentativa
}

// MistakeMemory armazena um registro estruturado de falhas para evitar repetição
type MistakeMemory struct {
	mu      sync.Mutex
	entries map[string]*MistakeEntry // chave: "toolName:targetPath"
	order   []string                 // ordem de inserção para LRU
	maxSize int
}

// NewMistakeMemory cria uma nova memória de erros com limite LRU
func NewMistakeMemory(maxSize int) *MistakeMemory {
	if maxSize <= 0 {
		maxSize = 20
	}
	return &MistakeMemory{
		entries: make(map[string]*MistakeEntry),
		order:   make([]string, 0, maxSize),
		maxSize: maxSize,
	}
}

// Record registra uma tentativa falhada no registro de erros
func (mm *MistakeMemory) Record(toolName, target, attempt, errorMsg string) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	key := toolName + ":" + target

	entry, exists := mm.entries[key]
	if !exists {
		entry = &MistakeEntry{
			Action: toolName,
			Target: target,
		}
		mm.entries[key] = entry

		// LRU: se excedeu o limite, remove o mais antigo
		if len(mm.order) >= mm.maxSize {
			oldestKey := mm.order[0]
			mm.order = mm.order[1:]
			delete(mm.entries, oldestKey)
		}
		mm.order = append(mm.order, key)
	} else {
		// Move para o final da fila (mais recente)
		for i, k := range mm.order {
			if k == key {
				mm.order = append(mm.order[:i], mm.order[i+1:]...)
				mm.order = append(mm.order, key)
				break
			}
		}
	}

	// Limitar a 5 tentativas por entrada para não estourar o contexto
	if len(entry.Attempts) >= 5 {
		entry.Attempts = entry.Attempts[1:]
	}
	if len(entry.Errors) >= 5 {
		entry.Errors = entry.Errors[1:]
	}

	entry.Attempts = append(entry.Attempts, truncateMistakeStr(attempt, 150))
	entry.Errors = append(entry.Errors, truncateMistakeStr(errorMsg, 200))
	entry.LastAttempt = time.Now()
}

// HasRepeatedFailure verifica se uma ação+alvo já falhou N vezes consecutivas
func (mm *MistakeMemory) HasRepeatedFailure(toolName, target string, threshold int) bool {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	key := toolName + ":" + target
	entry, exists := mm.entries[key]
	if !exists {
		return false
	}
	return len(entry.Errors) >= threshold
}

// BuildPromptBlock gera o bloco de texto para injeção no prompt de sistema
func (mm *MistakeMemory) BuildPromptBlock() string {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	if len(mm.entries) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("[MISTAKE MEMORY — AÇÕES FALHADAS ANTERIORES]\n")
	sb.WriteString("ATENÇÃO: As seguintes ações já foram tentadas e falharam. NÃO repita a mesma abordagem.\n\n")

	count := 0
	// Iterar na ordem reversa (mais recentes primeiro)
	for i := len(mm.order) - 1; i >= 0 && count < 10; i-- {
		key := mm.order[i]
		entry, exists := mm.entries[key]
		if !exists {
			continue
		}

		sb.WriteString(fmt.Sprintf("• [%s] em '%s' — %d tentativa(s)\n", entry.Action, entry.Target, len(entry.Attempts)))

		// Mostrar o último erro
		if len(entry.Errors) > 0 {
			lastErr := entry.Errors[len(entry.Errors)-1]
			sb.WriteString(fmt.Sprintf("  Último erro: %s\n", lastErr))
		}

		// Se houve mais de 2 tentativas, alertar fortemente
		if len(entry.Attempts) >= 3 {
			sb.WriteString("  ⚠️ REPETIÇÃO EXCESSIVA: Mude COMPLETAMENTE de estratégia.\n")
		}
		count++
	}

	sb.WriteString("\nUse estas informações para evitar repetir erros. Adote abordagens diferentes.\n")
	return sb.String()
}

// Clear limpa toda a memória de erros
func (mm *MistakeMemory) Clear() {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	mm.entries = make(map[string]*MistakeEntry)
	mm.order = make([]string, 0, mm.maxSize)
}

// Size retorna o número de entradas na memória
func (mm *MistakeMemory) Size() int {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	return len(mm.entries)
}

func truncateMistakeStr(s string, max int) string {
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}
