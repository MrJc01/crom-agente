package prompting

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/crom/crom-agente/internal/tools"
)

type dummyTool struct {
	id          string
	description string
}

func (d *dummyTool) ID() string                        { return d.id }
func (d *dummyTool) Description() string               { return d.description }
func (d *dummyTool) ParametersSchema() json.RawMessage { return nil }
func (d *dummyTool) RequiresApproval() bool            { return false }
func (d *dummyTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	return tools.Result{Success: true}, nil
}

func TestBuildToolsInstructions(t *testing.T) {
	// 1. Caso base: sem ferramentas
	if res := BuildToolsInstructions(nil, nil); res != "" {
		t.Errorf("esperava string vazia para nenhuma ferramenta, obteve: %q", res)
	}

	// 2. Ferramentas sem PromptManager (deve fazer fallback para Description)
	toolList := []tools.Tool{
		&dummyTool{id: "tool_b", description: "Desc B"},
		&dummyTool{id: "tool_a", description: "Desc A"},
	}

	res := BuildToolsInstructions(nil, toolList)
	// Deve ordenar alfabeticamente: tool_a depois tool_b
	if !strings.Contains(res, "tool_a") || !strings.Contains(res, "tool_b") {
		t.Errorf("esperava conter IDs das ferramentas, obteve: %q", res)
	}

	lines := strings.Split(strings.TrimSpace(res), "\n")
	if len(lines) < 3 {
		t.Fatalf("esperava pelo menos 3 linhas no resultado, obteve %d: %q", len(lines), res)
	}
	// Primeira linha é o título
	// Segunda linha deve ser tool_a
	if !strings.Contains(lines[1], "tool_a") || !strings.Contains(lines[1], "Desc A") {
		t.Errorf("esperava tool_a ordenado primeiro com sua descrição, linha: %q", lines[1])
	}
	// Terceira linha deve ser tool_b
	if !strings.Contains(lines[2], "tool_b") || !strings.Contains(lines[2], "Desc B") {
		t.Errorf("esperava tool_b ordenado segundo com sua descrição, linha: %q", lines[2])
	}
}
