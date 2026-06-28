package core

import (
	"context"
	"encoding/json"

	"github.com/crom/crom-agente/internal/loop"
	"github.com/crom/crom-agente/internal/tools"
)

const (
	MaxIterations          = 15
	MaxConsecutiveFailures = 3
)

// --- Mock Tool para testes ---

type mockTool struct {
	id          string
	description string
	approve     bool
	executeFunc func(ctx context.Context, args json.RawMessage) (tools.Result, error)
}

func (m *mockTool) ID() string                        { return m.id }
func (m *mockTool) Description() string               { return m.description }
func (m *mockTool) ParametersSchema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (m *mockTool) RequiresApproval() bool            { return m.approve }
func (m *mockTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, args)
	}
	return tools.Result{Success: true, Data: "ok"}, nil
}

// --- Mock EventHandler para capturar eventos ---

type testEventHandler struct {
	StatusChanges []string
	Messages      []struct{ Role, Content string }
	Events        []loop.AgentEvent
}

func (h *testEventHandler) OnStatusChange(s string) {
	h.StatusChanges = append(h.StatusChanges, s)
}
func (h *testEventHandler) OnStreamChunk(chunk string) {}

func (h *testEventHandler) OnMessage(role, content string) {
	h.Messages = append(h.Messages, struct{ Role, Content string }{role, content})
}
func (h *testEventHandler) OnEvent(event loop.AgentEvent) {
	h.Events = append(h.Events, event)
}

type cancelOnRetryHandler struct {
	StatusChanges []string
	Messages      []struct{ Role, Content string }
	Events        []loop.AgentEvent
	cancelFunc    context.CancelFunc
}

func (h *cancelOnRetryHandler) OnStatusChange(s string) {
	h.StatusChanges = append(h.StatusChanges, s)
}
func (h *cancelOnRetryHandler) OnMessage(role, content string) {
	h.Messages = append(h.Messages, struct{ Role, Content string }{role, content})
}
func (h *cancelOnRetryHandler) OnStreamChunk(chunk string) {}
func (h *cancelOnRetryHandler) OnEvent(event loop.AgentEvent) {
	h.Events = append(h.Events, event)
	if event.Event == "retry" && h.cancelFunc != nil {
		h.cancelFunc()
	}
}

// --- Testes ---

