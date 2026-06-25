package read_file

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadFileTool_SandboxJail(t *testing.T) {
	ws := t.TempDir()

	// Escreve arquivo interno
	innerFile := filepath.Join(ws, "file.txt")
	_ = os.WriteFile(innerFile, []byte("conteúdo interno"), 0644)

	// Escreve arquivo externo
	outerFile := filepath.Join(filepath.Dir(ws), "outer.txt")
	_ = os.WriteFile(outerFile, []byte("conteúdo externo"), 0644)

	toolJail := NewReadFileTool(ws, true)
	toolFree := NewReadFileTool(ws, false)

	// 1. Lê arquivo interno com Jail (deve funcionar)
	argsOk := json.RawMessage(`{"path": "file.txt"}`)
	res, err := toolJail.Execute(context.Background(), argsOk)
	if err != nil || !res.Success {
		t.Fatalf("erro ao ler arquivo interno: %v, res: %+v", err, res)
	}
	if res.Data != "conteúdo interno" {
		t.Fatalf("conteúdo incorreto: %s", res.Data)
	}

	// 2. Lê arquivo externo com Jail (deve bloquear)
	argsBad := json.RawMessage(`{"path": "../outer.txt"}`)
	res, err = toolJail.Execute(context.Background(), argsBad)
	if err != nil || res.Success {
		t.Fatalf("esperava bloqueio de jail, res: %+v", res)
	}
	if !strings.Contains(res.Error, "está fora do sandbox") {
		t.Fatalf("mensagem de erro inválida: %s", res.Error)
	}

	// 3. Lê arquivo externo sem Jail (deve funcionar)
	res, err = toolFree.Execute(context.Background(), argsBad)
	if err != nil || !res.Success {
		t.Fatalf("erro ao ler sem jail: %v, res: %+v", err, res)
	}
	if res.Data != "conteúdo externo" {
		t.Fatalf("conteúdo sem jail incorreto: %s", res.Data)
	}
}
