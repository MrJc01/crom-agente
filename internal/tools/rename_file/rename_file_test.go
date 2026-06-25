package rename_file_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/crom/crom-agente/internal/tools/rename_file"
)

func TestRenameFileTool(t *testing.T) {
	ws := t.TempDir()
	tool := rename_file.NewRenameFileTool(ws, true)

	srcFile := filepath.Join(ws, "origem.txt")
	_ = os.WriteFile(srcFile, []byte("dados"), 0644)

	// Renomear normal
	args := json.RawMessage(`{
		"src_path": "origem.txt",
		"dest_path": "subdir/destino.txt"
	}`)
	res, err := tool.Execute(context.Background(), args)
	if err != nil || !res.Success {
		t.Fatalf("erro ao renomear: %v, res: %+v", err, res)
	}

	// Validar criação e existência
	if _, err := os.Stat(filepath.Join(ws, "subdir/destino.txt")); err != nil {
		t.Fatalf("arquivo de destino não existe: %v", err)
	}
	if _, err := os.Stat(srcFile); !os.IsNotExist(err) {
		t.Fatal("arquivo de origem ainda existe")
	}

	// Validar jail em caminho externo
	argsBad := json.RawMessage(`{
		"src_path": "subdir/destino.txt",
		"dest_path": "../externo.txt"
	}`)
	res, _ = tool.Execute(context.Background(), argsBad)
	if res.Success {
		t.Fatal("esperava erro de jail ao mover para fora do workspace")
	}
}
