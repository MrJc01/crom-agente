package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/crom/crom-agente/internal/agents/core"
	"github.com/crom/crom-agente/internal/tools"
)

// MockCoreAgent implementa a interface core.Agent para teste do adaptador
type MockCoreAgent struct {
	NameVal        string
	DescriptionVal string
	SysPromptVal   string
	ExecuteRes     core.AgentResult
	ExecuteErr     error
	ReceivedPrompt string
	ReceivedPrior  string
}

func (m *MockCoreAgent) Name() string {
	return m.NameVal
}

func (m *MockCoreAgent) Description() string {
	return m.DescriptionVal
}

func (m *MockCoreAgent) SystemPrompt() string {
	return m.SysPromptVal
}

func (m *MockCoreAgent) ToolIDs() []string {
	return nil
}

func (m *MockCoreAgent) Execute(ctx context.Context, prompt string, priorSummary string) (core.AgentResult, error) {
	m.ReceivedPrompt = prompt
	m.ReceivedPrior = priorSummary
	return m.ExecuteRes, m.ExecuteErr
}

func TestAgentToolAdapter(t *testing.T) {
	inner := &MockCoreAgent{
		NameVal:        "expert-coder",
		DescriptionVal: "Codes beautifully",
		ExecuteRes: core.AgentResult{
			Success:        true,
			Output:         "Completed coding",
			ContextSummary: "Finished all modules",
		},
	}

	adapter := tools.NewAgentToolAdapter(inner)

	if adapter.ID() != "expert-coder" {
		t.Errorf("ID incorreto: %s", adapter.ID())
	}

	// Teste Execute com JSON malformado (deve usar fallback de string crua)
	resMalformed, err := adapter.Execute(context.Background(), json.RawMessage("{invalid"))
	if err != nil {
		t.Fatalf("Execute falhou com erro Go: %v", err)
	}
	if !resMalformed.Success {
		t.Errorf("esperava success=true devido ao fallback com JSON malformado")
	}
	if inner.ReceivedPrompt != "{invalid" {
		t.Errorf("esperava prompt '{invalid', obteve '%s'", inner.ReceivedPrompt)
	}

	// Teste Execute com sucesso
	input := `{"prompt": "refactor main.go", "prior_summary": "first step done"}`
	res, err := adapter.Execute(context.Background(), json.RawMessage(input))
	if err != nil {
		t.Fatalf("Execute falhou: %v", err)
	}

	if !res.Success {
		t.Errorf("esperava success=true, obteve false com erro: %s", res.Error)
	}

	if inner.ReceivedPrompt != "refactor main.go" {
		t.Errorf("prompt recebido incorreto: %s", inner.ReceivedPrompt)
	}

	if inner.ReceivedPrior != "first step done" {
		t.Errorf("prior_summary recebido incorreto: %s", inner.ReceivedPrior)
	}

	if !strings.Contains(res.Data, "Completed coding") {
		t.Errorf("esperava encontrar 'Completed coding' no relatório, obteve: %s", res.Data)
	}

	if !strings.Contains(res.Data, "Finished all modules") {
		t.Errorf("esperava encontrar 'Finished all modules' no relatório, obteve: %s", res.Data)
	}
}
