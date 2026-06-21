package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestPTY_TerminalCommandRun(t *testing.T) {
	ws := t.TempDir()
	tool := NewTerminalCommandTool(ws, []string{"rm -rf /"})

	ctx := context.Background()
	args, _ := json.Marshal(map[string]interface{}{
		"command": "echo 'hello from pty'",
		"action":  "run",
	})

	res, err := tool.Execute(ctx, args)
	if err != nil {
		t.Fatalf("Execute falhou: %v", err)
	}

	if !res.Success {
		t.Errorf("Esperado sucesso, got erro: %s", res.Error)
	}

	if !strings.Contains(res.Data, "hello from pty") {
		t.Errorf("Saída inesperada: %s", res.Data)
	}
}

func TestPTY_BlockedCommand(t *testing.T) {
	ws := t.TempDir()
	tool := NewTerminalCommandTool(ws, []string{"sudo"})

	ctx := context.Background()
	args, _ := json.Marshal(map[string]interface{}{
		"command": "sudo cat /etc/shadow",
		"action":  "run",
	})

	res, _ := tool.Execute(ctx, args)
	if res.Success {
		t.Error("Comando bloqueado deveria ter falhado")
	}

	if !strings.Contains(res.Error, "bloqueado") {
		t.Errorf("Mensagem de erro incorreta: %s", res.Error)
	}
}
