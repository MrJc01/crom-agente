package tree_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crom/crom-agente/internal/tools/tree"
)

func TestTreeTool(t *testing.T) {
	ws := t.TempDir()
	tool := tree.NewTreeTool(ws, true)

	_ = os.MkdirAll(filepath.Join(ws, "dir1/subdir"), 0755)
	_ = os.WriteFile(filepath.Join(ws, "dir1/subdir/file.txt"), []byte("txt"), 0644)
	_ = os.MkdirAll(filepath.Join(ws, ".git"), 0755) // Deve ser ignorado

	args := json.RawMessage(`{"max_depth": 3}`)
	res, err := tool.Execute(context.Background(), args)
	if err != nil || !res.Success {
		t.Fatalf("erro ao executar tree: %v, res: %+v", err, res)
	}

	if strings.Contains(res.Data, ".git") {
		t.Fatal("tree listou a pasta oculta .git que deveria ser ignorada")
	}
	if !strings.Contains(res.Data, "dir1") || !strings.Contains(res.Data, "file.txt") {
		t.Fatalf("tree não listou arquivos esperados: %s", res.Data)
	}
}
