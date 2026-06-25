package delete_file_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crom/crom-agente/internal/tools/delete_file"
)

func TestDeleteFileTool(t *testing.T) {
	ws := t.TempDir()
	tool := delete_file.NewDeleteFileTool(ws, true)

	file := filepath.Join(ws, "deletar.txt")
	_ = os.WriteFile(file, []byte("deletar"), 0644)

	// 1. Deleção normal
	args := json.RawMessage(`{"path": "deletar.txt"}`)
	res, err := tool.Execute(context.Background(), args)
	if err != nil || !res.Success {
		t.Fatalf("erro ao deletar arquivo: %v, res: %+v", err, res)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("arquivo ainda existe pós deleção")
	}

	// 2. Travas de segurança (proibir deletar go.mod)
	gomod := filepath.Join(ws, "go.mod")
	_ = os.WriteFile(gomod, []byte("module test"), 0644)

	argsGomod := json.RawMessage(`{"path": "go.mod"}`)
	res, _ = tool.Execute(context.Background(), argsGomod)
	if res.Success {
		t.Fatal("esperava bloqueio de segurança ao tentar deletar go.mod")
	}
	if !strings.Contains(res.Error, "não é permitido deletar o go.mod") {
		t.Fatalf("erro esperado sobre go.mod, obteve: %s", res.Error)
	}

	// 3. Travas de segurança (.git)
	gitdir := filepath.Join(ws, ".git")
	_ = os.Mkdir(gitdir, 0755)
	argsGit := json.RawMessage(`{"path": ".git"}`)
	res, _ = tool.Execute(context.Background(), argsGit)
	if res.Success {
		t.Fatal("esperava bloqueio de segurança ao tentar deletar .git")
	}
}
