package checkpoint

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestCheckpointTool(t *testing.T) {
	ws := t.TempDir()
	tool := NewCheckpointTool(ws, nil)

	// Test fallback file creation/listing
	argsCreate := json.RawMessage(`{"action": "create", "name": "test_checkpoint"}`)
	resCreate, err := tool.Execute(context.Background(), argsCreate)
	if err != nil || !resCreate.Success {
		t.Fatalf("erro ao criar checkpoint: %v", err)
	}

	argsList := json.RawMessage(`{"action": "list"}`)
	resList, err := tool.Execute(context.Background(), argsList)
	if err != nil || !resList.Success {
		t.Fatalf("erro ao listar checkpoints: %v", err)
	}

	if !strings.Contains(resList.Data, "test_checkpoint") {
		t.Errorf("esperava ver o checkpoint na lista, obteve:\n%s", resList.Data)
	}

	argsRestore := json.RawMessage(`{"action": "restore", "name": "test_checkpoint"}`)
	resRestore, err := tool.Execute(context.Background(), argsRestore)
	if err != nil || !resRestore.Success {
		t.Fatalf("erro ao restaurar checkpoint: %v", err)
	}
}
