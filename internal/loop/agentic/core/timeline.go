package core

import (
	"strings"
	"sync"
)

// TimelineMemory rastreia as últimas ações bem-sucedidas do agente para criar um histórico reduzido
// útil como contexto durante a compactação (Task 73).
type TimelineMemory struct {
	actions []string
	maxSize int
	mu      sync.RWMutex
}

func NewTimelineMemory(maxSize int) *TimelineMemory {
	if maxSize <= 0 {
		maxSize = 10
	}
	return &TimelineMemory{
		actions: make([]string, 0, maxSize),
		maxSize: maxSize,
	}
}

func (tm *TimelineMemory) RecordAction(action string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Truncar ações muito longas para economizar tokens
	if len(action) > 150 {
		action = action[:147] + "..."
	}

	tm.actions = append(tm.actions, action)
	if len(tm.actions) > tm.maxSize {
		tm.actions = tm.actions[1:] // Buffer circular
	}
}

func (tm *TimelineMemory) GetTimeline() string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if len(tm.actions) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("TIMELINE RECENTE DE AÇÕES BEM-SUCEDIDAS:\n")
	for i, a := range tm.actions {
		sb.WriteString("- " + a + "\n")
		// _ suppresses "declared and not used" if i is unused, but we can just do range tm.actions
		_ = i
	}
	return sb.String()
}
