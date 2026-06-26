package core

import (
	"fmt"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/crom/crom-agente/internal/config"
	"github.com/crom/crom-agente/internal/llm/providers"
	"github.com/crom/crom-agente/internal/loop"
	"github.com/crom/crom-agente/internal/state"
	"github.com/crom/crom-agente/internal/tools"
)

// ═══════════════════════════════════════════════════════════
// Testes de Eventos Estruturados (loop.AgentEvent)
// ═══════════════════════════════════════════════════════════

func eventTypes(events []loop.AgentEvent) []string {
	types := make([]string, len(events))
	for i, e := range events {
		types[i] = e.Event
	}
	return types
}

func findEvent(events []loop.AgentEvent, eventType string) *loop.AgentEvent {
	for i := range events {
		if events[i].Event == eventType {
			return &events[i]
		}
	}
	return nil
}

func findEventWithCode(events []loop.AgentEvent, code string) *loop.AgentEvent {
	for i := range events {
		if events[i].Event == "error" {
			if agentErr, ok := events[i].Data["error"].(loop.AgentError); ok {
				if agentErr.Code == code {
					return &events[i]
				}
			}
		}
	}
	return nil
}

func assertEventExists(t *testing.T, events []loop.AgentEvent, eventType string) {
	t.Helper()
	if findEvent(events, eventType) == nil {
		t.Fatalf("evento '%s' esperado mas não encontrado. Eventos presentes: %v", eventType, eventTypes(events))
	}
}

func TestAgentEvent_SimpleTextEmitsThinkingMessageFinished(t *testing.T) {
	provider := providers.NewMockProvider(
		providers.MockTextResponse("Resposta simples.", 150),
	)
	sm := state.NewStateManager(t.TempDir())
	handler := &testEventHandler{}

	al := New(provider, sm, handler)
	err := al.Execute(context.Background(), "Diga olá")
	if err != nil {
		t.Fatalf("esperado sucesso, obteve: %v", err)
	}

	// Deve ter pelo menos: thinking, message, finished
	if len(handler.Events) < 3 {
		t.Fatalf("esperado pelo menos 3 eventos, obteve %d: %v", len(handler.Events), eventTypes(handler.Events))
	}

	assertEventExists(t, handler.Events, "thinking")
	assertEventExists(t, handler.Events, "message")
	assertEventExists(t, handler.Events, "finished")

	// Verificar message com dados completos
	msgEvt := findEvent(handler.Events, "message")
	if msgEvt.Iteration != 1 {
		t.Fatalf("esperado iteration=1, obteve %d", msgEvt.Iteration)
	}
	if msgEvt.Data["role"] != "assistant" {
		t.Fatalf("esperado role=assistant, obteve %v", msgEvt.Data["role"])
	}
	if msgEvt.Data["content"] != "Resposta simples." {
		t.Fatalf("esperado content correto, obteve %v", msgEvt.Data["content"])
	}
	usage, ok := msgEvt.Data["usage"].(map[string]int)
	if !ok {
		t.Fatalf("esperado campo 'usage' do tipo map[string]int, obteve %T", msgEvt.Data["usage"])
	}
	if usage["total_tokens"] != 150 {
		t.Fatalf("esperado total_tokens=150, obteve %d", usage["total_tokens"])
	}

	// Verificar finished com reason=completed
	finEvt := findEvent(handler.Events, "finished")
	if finEvt.Data["reason"] != "completed" {
		t.Fatalf("esperado reason=completed, obteve %v", finEvt.Data["reason"])
	}
}

func TestAgentEvent_ToolCallEmitsToolEvents(t *testing.T) {
	provider := providers.NewMockProvider(
		providers.MockToolCallResponse("read_file", `{"path":"/tmp/test.txt"}`, 200),
		providers.MockTextResponse("Arquivo lido.", 100),
		providers.MockTextResponse("Verificação ok.", 50),
	)
	sm := state.NewStateManager(t.TempDir())
	handler := &testEventHandler{}

	al := New(provider, sm, handler)
	al.RegisterTool(&mockTool{
		id:          "read_file",
		description: "Lê arquivo",
		executeFunc: func(ctx context.Context, args json.RawMessage) (tools.Result, error) {
			return tools.Result{Success: true, Data: "conteudo"}, nil
		},
	})

	err := al.Execute(context.Background(), "Leia /tmp/test.txt")
	if err != nil {
		t.Fatalf("esperado sucesso, obteve: %v", err)
	}

	tcEvt := findEvent(handler.Events, "tool_call")
	if tcEvt == nil {
		t.Fatal("evento 'tool_call' não encontrado")
	}
	if tcEvt.Data["tool"] != "read_file" {
		t.Fatalf("esperado tool=read_file, obteve %v", tcEvt.Data["tool"])
	}

	trEvt := findEvent(handler.Events, "tool_result")
	if trEvt == nil {
		t.Fatal("evento 'tool_result' não encontrado")
	}
	if trEvt.Data["success"] != true {
		t.Fatalf("esperado success=true, obteve %v", trEvt.Data["success"])
	}
}

func TestAgentEvent_ToolFailureEmitsErrorResult(t *testing.T) {
	provider := providers.NewMockProvider(
		providers.MockToolCallResponse("read_file", `{"path":"/nope"}`, 200),
		providers.MockTextResponse("Não encontrado.", 50),
		providers.MockTextResponse("Ok.", 30),
	)
	sm := state.NewStateManager(t.TempDir())
	handler := &testEventHandler{}

	al := New(provider, sm, handler)
	al.RegisterTool(&mockTool{
		id:          "read_file",
		description: "Lê arquivo",
		executeFunc: func(ctx context.Context, args json.RawMessage) (tools.Result, error) {
			return tools.Result{Success: false, Error: "file not found"}, nil
		},
	})

	_ = al.Execute(context.Background(), "Leia /nope")

	trEvt := findEvent(handler.Events, "tool_result")
	if trEvt == nil {
		t.Fatal("evento 'tool_result' não encontrado")
	}
	if trEvt.Data["success"] != false {
		t.Fatalf("esperado success=false, obteve %v", trEvt.Data["success"])
	}
	if trEvt.Data["error"] != "file not found" {
		t.Fatalf("esperado error='file not found', obteve %v", trEvt.Data["error"])
	}
	if trEvt.Data["error_code"] != loop.ErrToolExecution {
		t.Fatalf("esperado error_code=%s, obteve %v", loop.ErrToolExecution, trEvt.Data["error_code"])
	}
}

func TestAgentEvent_ToolNotFoundEmitsError(t *testing.T) {
	provider := providers.NewMockProvider(
		providers.MockToolCallResponse("non_existent", `{}`, 100),
		providers.MockTextResponse("Ok.", 30),
		providers.MockTextResponse("Fim.", 20),
	)
	sm := state.NewStateManager(t.TempDir())
	handler := &testEventHandler{}

	al := New(provider, sm, handler)
	_ = al.Execute(context.Background(), "Use tool")

	errEvt := findEvent(handler.Events, "error")
	if errEvt == nil {
		t.Fatal("evento 'error' não encontrado")
	}
	agentErr, ok := errEvt.Data["error"].(loop.AgentError)
	if !ok {
		t.Fatalf("esperado AgentError, obteve %T: %v", errEvt.Data["error"], errEvt.Data["error"])
	}
	if agentErr.Code != loop.ErrToolNotFound {
		t.Fatalf("esperado code=%s, obteve %s", loop.ErrToolNotFound, agentErr.Code)
	}
}

func TestAgentEvent_LLMErrorEmitsTypedError(t *testing.T) {
	provider := providers.NewMockProvider(
		providers.MockErrorResponse("connection refused"),
	)
	sm := state.NewStateManager(t.TempDir())
	handler := &testEventHandler{}

	al := New(provider, sm, handler)
	_ = al.Execute(context.Background(), "Qualquer coisa")

	errEvt := findEvent(handler.Events, "error")
	if errEvt == nil {
		t.Fatal("evento 'error' não encontrado após falha do LLM")
	}
	agentErr, ok := errEvt.Data["error"].(loop.AgentError)
	if !ok {
		t.Fatalf("esperado AgentError, obteve %T", errEvt.Data["error"])
	}
	if agentErr.Message == "" {
		t.Fatal("esperado mensagem de erro não vazia")
	}
}

func TestAgentEvent_MaxIterationsEmitsFinished(t *testing.T) {
	responses := make([]providers.MockResponse, MaxIterations+1)
	for i := range responses {
		responses[i] = providers.MockToolCallResponse("echo", fmt.Sprintf(`{"msg":"loop %d"}`, i), 10)
	}
	provider := providers.NewMockProvider(responses...)
	sm := state.NewStateManager(t.TempDir())
	handler := &testEventHandler{}

	al := New(provider, sm, handler, &config.ResolvedConfig{
		MaxIterations:     15,
		MaxMessageHistory: 100,
	})
	al.RegisterTool(&mockTool{id: "echo", description: "Ecoa"})

	_ = al.Execute(context.Background(), "Loop")

	finEvt := findEvent(handler.Events, "finished")
	if finEvt == nil {
		t.Fatal("evento 'finished' não encontrado")
	}
	if finEvt.Data["reason"] != "max_iterations" {
		t.Fatalf("esperado reason=max_iterations, obteve %v", finEvt.Data["reason"])
	}
}

func TestAgentEvent_ContextCanceledEmitsError(t *testing.T) {
	provider := providers.NewMockProvider(
		providers.MockTextResponse("nunca", 100),
	)
	sm := state.NewStateManager(t.TempDir())
	handler := &testEventHandler{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	al := New(provider, sm, handler)
	_ = al.Execute(ctx, "Cancelada")

	errEvt := findEvent(handler.Events, "error")
	if errEvt == nil {
		t.Fatal("evento 'error' não encontrado após cancelamento")
	}
	agentErr, ok := errEvt.Data["error"].(loop.AgentError)
	if !ok {
		t.Fatalf("esperado AgentError, obteve %T", errEvt.Data["error"])
	}
	if agentErr.Code != loop.ErrContextCanceled {
		t.Fatalf("esperado code=%s, obteve %s", loop.ErrContextCanceled, agentErr.Code)
	}
}

func TestAgentEvent_ConsecutiveFailuresEmitsRetry(t *testing.T) {
	provider := providers.NewMockProvider(
		providers.MockEmptyResponse(),
		providers.MockEmptyResponse(),
		providers.MockEmptyResponse(),
	)
	sm := state.NewStateManager(t.TempDir())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := &cancelOnRetryHandler{
		cancelFunc: cancel,
	}

	al := New(provider, sm, handler)
	al.failureRetryDelay = 1 * time.Millisecond
	_ = al.Execute(ctx, "Falhe")

	emptyErr := findEventWithCode(handler.Events, loop.ErrLLMEmptyResponse)
	if emptyErr == nil {
		t.Fatal("esperado ERR_LLM_EMPTY_RESPONSE")
	}

	// Deve ter emitido o evento retry
	hasRetry := false
	for _, ev := range handler.Events {
		if ev.Event == "retry" {
			hasRetry = true
			if ev.Data["reason"] != "consecutive_failures" {
				t.Fatalf("esperado reason=consecutive_failures, obteve %v", ev.Data["reason"])
			}
			if ev.Data["error_type"] != "empty_llm_response" {
				t.Fatalf("esperado error_type=empty_llm_response, obteve %v", ev.Data["error_type"])
			}
		}
	}
	if !hasRetry {
		t.Fatal("esperado evento 'retry' no handler")
	}

	// Deve ter emitido o evento error indicando cancelamento de contexto
	var cancelErrEvt *loop.AgentEvent
	for idx := range handler.Events {
		if handler.Events[idx].Event == "error" {
			if agentErr, ok := handler.Events[idx].Data["error"].(loop.AgentError); ok {
				if agentErr.Code == loop.ErrContextCanceled {
					cancelErrEvt = &handler.Events[idx]
					break
				}
			}
		}
	}
	if cancelErrEvt == nil {
		t.Fatal("evento 'error' com código ERR_CONTEXT_CANCELED não encontrado")
	}
}

func TestAgentEvent_ThinkingHasProviderAndIteration(t *testing.T) {
	provider := providers.NewMockProvider(
		providers.MockTextResponse("Ok.", 50),
	)
	sm := state.NewStateManager(t.TempDir())
	handler := &testEventHandler{}

	al := New(provider, sm, handler)
	_ = al.Execute(context.Background(), "Diga ok")

	thinkEvt := findEvent(handler.Events, "thinking")
	if thinkEvt == nil {
		t.Fatal("evento 'thinking' não encontrado")
	}
	if thinkEvt.Data["provider"] == nil || thinkEvt.Data["provider"] == "" {
		t.Fatal("provider vazio no thinking")
	}
	if thinkEvt.Iteration < 1 {
		t.Fatalf("iteration=%d, esperado >= 1", thinkEvt.Iteration)
	}
	if thinkEvt.Timestamp.IsZero() {
		t.Fatal("timestamp zero")
	}
}

func TestAgentEvent_AllEventsHaveTimestamp(t *testing.T) {
	provider := providers.NewMockProvider(
		providers.MockToolCallResponse("read_file", `{"path":"/x"}`, 100),
		providers.MockTextResponse("Ok.", 50),
		providers.MockTextResponse("Fim.", 30),
	)
	sm := state.NewStateManager(t.TempDir())
	handler := &testEventHandler{}

	al := New(provider, sm, handler)
	al.RegisterTool(&mockTool{id: "read_file", description: "Lê"})

	_ = al.Execute(context.Background(), "Leia x")

	for i, evt := range handler.Events {
		if evt.Timestamp.IsZero() {
			t.Fatalf("evento[%d] (type=%s) tem timestamp zero", i, evt.Event)
		}
		if evt.Event == "" {
			t.Fatalf("evento[%d] tem event type vazio", i)
		}
	}
	t.Logf("Total de eventos emitidos: %d — tipos: %v", len(handler.Events), eventTypes(handler.Events))
}
