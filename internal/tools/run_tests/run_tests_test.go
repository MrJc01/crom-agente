package run_tests_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crom/crom-agente/internal/tools/run_tests"
)

func TestRunTestsTool(t *testing.T) {
	ws := t.TempDir()
	tool := run_tests.NewRunTestsTool(ws)

	// 1. Testa detecção de stack vazia
	res, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("erro ao executar run_tests: %v", err)
	}
	if res.Success {
		t.Fatal("esperava falha por falta de testes detectáveis")
	}

	// 2. Simula Go project
	_ = os.WriteFile(filepath.Join(ws, "go.mod"), []byte("module test"), 0644)

	// Executa com comando customizado leve que sempre passa
	argsCustom := json.RawMessage(`{"command": "echo 'tests passed'"}`)
	res, err = tool.Execute(context.Background(), argsCustom)
	if err != nil || !res.Success {
		t.Fatalf("falha ao rodar testes customizados: %v, res: %+v", err, res)
	}
	if !strings.Contains(res.Data, "tests passed") {
		t.Fatalf("saída inesperada: %s", res.Data)
	}
}
