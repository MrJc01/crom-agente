package refactor_auditor

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRefactorAuditorTool(t *testing.T) {
	ws := t.TempDir()
	tool := NewRefactorAuditorTool(ws, false)

	// Cria arquivo go com função longa
	longFuncCode := "package test\n\nfunc Long() {\n"
	for i := 0; i < 90; i++ {
		longFuncCode += "  _ = 1\n"
	}
	longFuncCode += "}\n"

	targetFile := filepath.Join(ws, "main.go")
	_ = os.WriteFile(targetFile, []byte(longFuncCode), 0644)

	// Cria arquivo temporário morto
	_ = os.WriteFile(filepath.Join(ws, "unrelated.tmp"), []byte("dead"), 0644)

	args := json.RawMessage(`{"path": "main.go"}`)
	res, err := tool.Execute(context.Background(), args)
	if err != nil || !res.Success {
		t.Fatalf("erro ao executar auditor: %v", err)
	}

	if !strings.Contains(res.Data, "Função muito longa") {
		t.Errorf("esperava detectar função muito longa, obteve:\n%s", res.Data)
	}
	if !strings.Contains(res.Data, "Arquivo temporário/morto encontrado") {
		t.Errorf("esperava detectar arquivo morto, obteve:\n%s", res.Data)
	}
}
