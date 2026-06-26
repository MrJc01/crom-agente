package bug_explainer

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestBugExplainerTool(t *testing.T) {
	tool := NewBugExplainerTool(t.TempDir(), nil)

	// Test fallback static diagnosis
	args := json.RawMessage(`{"error_log": "panic: runtime error: index out of range\nmain.go:42"}`)
	res, err := tool.Execute(context.Background(), args)
	if err != nil || !res.Success {
		t.Fatalf("erro na execução do bug_explainer: %v", err)
	}

	if !strings.Contains(res.Data, "Crítico") {
		t.Errorf("esperava gravidade crítica, obteve:\n%s", res.Data)
	}
	if !strings.Contains(res.Data, "main.go") || !strings.Contains(res.Data, "42") {
		t.Errorf("esperava encontrar localização no diagnóstico, obteve:\n%s", res.Data)
	}
}
