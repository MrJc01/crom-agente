package syntax_check

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSyntaxCheckTool(t *testing.T) {
	ws := t.TempDir()
	tool := NewSyntaxCheckTool(ws, false)

	// Test 1: Go file with valid syntax
	goodFile := filepath.Join(ws, "good.go")
	_ = os.WriteFile(goodFile, []byte("package main\n\nfunc main() {}\n"), 0644)
	args1, _ := json.Marshal(struct{ Path string }{Path: "good.go"})
	res1, err := tool.Execute(context.Background(), args1)
	if err != nil || !res1.Success {
		t.Fatalf("esperava sucesso para arquivo Go válido, obteve: %v, res: %+v", err, res1)
	}

	// Test 2: Go file with syntax error
	badFile := filepath.Join(ws, "bad.go")
	_ = os.WriteFile(badFile, []byte("package main\n\nfunc main() {\n"), 0644) // missing closing brace
	args2, _ := json.Marshal(struct{ Path string }{Path: "bad.go"})
	res2, err := tool.Execute(context.Background(), args2)
	if err != nil || res2.Success {
		t.Fatalf("esperava falha para arquivo Go inválido, obteve: %v, res: %+v", err, res2)
	}
}
