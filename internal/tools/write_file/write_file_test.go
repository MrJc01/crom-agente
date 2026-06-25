package write_file

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileTool_SandboxJail(t *testing.T) {
	ws := t.TempDir()

	toolJail := NewWriteFileTool(ws, true)

	// Escreve arquivo interno (deve funcionar)
	argsOk := json.RawMessage(`{"path": "subdir/file.txt", "content": "gravado"}`)
	res, err := toolJail.Execute(context.Background(), argsOk)
	if err != nil || !res.Success {
		t.Fatalf("erro ao gravar arquivo interno: %v, res: %+v", err, res)
	}

	// Verifica gravação física
	data, _ := os.ReadFile(filepath.Join(ws, "subdir/file.txt"))
	if string(data) != "gravado" {
		t.Fatalf("conteúdo não foi gravado fisicamente: %s", string(data))
	}

	// Escreve arquivo externo com Jail (deve bloquear)
	argsBad := json.RawMessage(`{"path": "../outer_write.txt", "content": "hack"}`)
	res, _ = toolJail.Execute(context.Background(), argsBad)
	if res.Success {
		t.Fatal("esperava erro de sandbox para escrita externa")
	}
}
