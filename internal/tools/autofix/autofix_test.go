package autofix

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crom/crom-agente/internal/llm"
)

type dummyProvider struct{}

func (d *dummyProvider) Name() string { return "dummy" }
func (d *dummyProvider) Model() string { return "dummy" }
func (d *dummyProvider) SupportsSystemPrompt() bool { return true }
func (d *dummyProvider) Capabilities() llm.ModelCapabilities { return llm.ModelCapabilities{} }
func (d *dummyProvider) SendMessages(ctx context.Context, msgs []llm.Message, opts llm.RequestOptions) (*llm.Response, error) {
	// Returns a fixed Go file that passes tests
	return &llm.Response{
		Message: llm.Message{
			Role:    "assistant",
			Content: "package test_autofix\n\nimport \"testing\"\n\nfunc TestDummy(t *testing.T) {}\n",
		},
	}, nil
}

func TestAutofixTool(t *testing.T) {
	ws := t.TempDir()
	
	// Create go.mod to satisfy module requirement
	_ = os.WriteFile(filepath.Join(ws, "go.mod"), []byte("module test_autofix\n\ngo 1.25.0\n"), 0644)

	// Create an initial failing test file
	filePath := filepath.Join(ws, "main_test.go")
	_ = os.WriteFile(filePath, []byte("package test_autofix\n\nimport \"testing\"\n\nfunc TestDummy(t *testing.T) { t.Fatal(\"fail\") }\n"), 0644)

	tool := NewAutofixTool(ws, false, &dummyProvider{})

	args := json.RawMessage(`{
		"path": "main_test.go",
		"error_message": "TestDummy failed"
	}`)

	res, err := tool.Execute(context.Background(), args)
	if err != nil || !res.Success {
		t.Fatalf("erro ao executar autofix: %v, res: %+v", err, res)
	}

	if !strings.Contains(res.Data, "Autofix aplicado com sucesso") {
		t.Errorf("esperava sucesso no autofix, obteve:\n%s", res.Data)
	}

	// Verify the file was updated with the corrected content
	content, _ := os.ReadFile(filePath)
	if !strings.Contains(string(content), "func TestDummy(t *testing.T) {}") {
		t.Errorf("arquivo não foi atualizado corretamenete, obteve:\n%s", string(content))
	}
}
