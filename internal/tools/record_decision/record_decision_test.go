package record_decision_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crom/crom-agente/internal/tools/record_decision"
)

func TestRecordDecisionTool_Properties(t *testing.T) {
	tool := record_decision.NewRecordDecisionTool(t.TempDir())

	if tool.ID() != "record_decision" {
		t.Errorf("ID esperado 'record_decision', obtido: %s", tool.ID())
	}

	if tool.Description() == "" {
		t.Errorf("descrição do tool não deveria ser vazia")
	}

	if tool.RequiresApproval() {
		t.Errorf("gravação de decisão não deveria exigir aprovação HITL")
	}
}

func TestRecordDecisionTool_Execute(t *testing.T) {
	tempDir := t.TempDir()
	tool := record_decision.NewRecordDecisionTool(tempDir)

	ctx := context.Background()
	args := json.RawMessage(`{"decision": "Usar Redis para cache de sessão"}`)

	res, err := tool.Execute(ctx, args)
	if err != nil {
		t.Fatalf("Execute falhou: %v", err)
	}

	if !res.Success {
		t.Errorf("esperava success=true, obteve false com erro: %s", res.Error)
	}

	logPath := filepath.Join(tempDir, ".crom", "decisions.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("falha ao ler decisões gravadas: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "Usar Redis para cache de sessão") {
		t.Errorf("esperava encontrar a decisão no decisions.log, obteve:\n%s", content)
	}
}
