package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crom/crom-agente/internal/tools/git_add"
	"github.com/crom/crom-agente/internal/tools/git_commit"
	"github.com/crom/crom-agente/internal/tools/git_diff"
	"github.com/crom/crom-agente/internal/tools/git_log"
	"github.com/crom/crom-agente/internal/tools/git_status"
)

func setupMockGitRepo(t *testing.T) string {
	dir := t.TempDir()

	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("falha ao inicializar git no teste: %v", err)
	}

	cmdEmail := exec.Command("git", "config", "user.email", "test@example.com")
	cmdEmail.Dir = dir
	_ = cmdEmail.Run()

	cmdName := exec.Command("git", "config", "user.name", "Test User")
	cmdName.Dir = dir
	_ = cmdName.Run()

	return dir
}

func TestGitStatusAndLogAndDiff(t *testing.T) {
	dir := setupMockGitRepo(t)

	// Cria e commita um arquivo inicial
	file1 := filepath.Join(dir, "file1.txt")
	_ = os.WriteFile(file1, []byte("conteudo inicial"), 0644)

	statusTool := git_status.NewGitStatusTool(dir)
	logTool := git_log.NewGitLogTool(dir)
	diffTool := git_diff.NewGitDiffTool(dir)
	addTool := git_add.NewGitAddTool(dir)
	commitTool := git_commit.NewGitCommitTool(dir)

	// 1. Testa git status antes do add
	res, err := statusTool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil || !res.Success {
		t.Fatalf("falha no git_status inicial: %v, res: %+v", err, res)
	}
	if !strings.Contains(res.Data, "file1.txt") {
		t.Fatalf("esperava file1.txt no status, obteve: %s", res.Data)
	}

	// 2. Testa git add
	addRes, err := addTool.Execute(context.Background(), json.RawMessage(`{"paths": ["file1.txt"]}`))
	if err != nil || !addRes.Success {
		t.Fatalf("falha no git_add: %v, res: %+v", err, addRes)
	}

	// 3. Testa git diff staged
	diffRes, err := diffTool.Execute(context.Background(), json.RawMessage(`{"staged": true}`))
	if err != nil || !diffRes.Success {
		t.Fatalf("falha no git_diff staged: %v, res: %+v", err, diffRes)
	}
	if !strings.Contains(diffRes.Data, "+conteudo inicial") {
		t.Fatalf("esperava ver diff da adição, obteve: %s", diffRes.Data)
	}

	// 4. Testa git commit com mensagem inválida
	commitRes, err := commitTool.Execute(context.Background(), json.RawMessage(`{"message": "commit feio"}`))
	if err != nil || commitRes.Success {
		t.Fatalf("esperava falha no commit por nao seguir conventional commits, res: %+v", commitRes)
	}

	// 5. Testa git commit correto
	commitRes, err = commitTool.Execute(context.Background(), json.RawMessage(`{"message": "feat(core): add file1"}`))
	if err != nil || !commitRes.Success {
		t.Fatalf("falha no git_commit valido: %v, res: %+v", err, commitRes)
	}

	// 6. Testa git log
	logRes, err := logTool.Execute(context.Background(), json.RawMessage(`{"limit": 5}`))
	if err != nil || !logRes.Success {
		t.Fatalf("falha no git_log: %v, res: %+v", err, logRes)
	}
	if !strings.Contains(logRes.Data, "feat(core): add file1") {
		t.Fatalf("esperava ver mensagem do commit no log, obteve: %s", logRes.Data)
	}
}
