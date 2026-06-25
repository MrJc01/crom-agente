package git_branch

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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

func TestGitBranch(t *testing.T) {
	dir := setupMockGitRepo(t)

	// É necessário ter pelo menos um commit para branches funcionarem corretamente
	file1 := filepath.Join(dir, "init.txt")
	_ = os.WriteFile(file1, []byte("init"), 0644)
	_ = exec.Command("git", "-C", dir, "add", ".").Run()
	_ = exec.Command("git", "-C", dir, "commit", "-m", "chore: init").Run()

	tool := NewGitBranchTool(dir)

	// 1. Listar branches
	res, err := tool.Execute(context.Background(), json.RawMessage(`{"action": "list"}`))
	if err != nil || !res.Success {
		t.Fatalf("falha no branch list: %v, res: %+v", err, res)
	}

	// 2. Criar branch
	res, err = tool.Execute(context.Background(), json.RawMessage(`{"action": "create", "name": "feature-test"}`))
	if err != nil || !res.Success {
		t.Fatalf("falha no branch create: %v, res: %+v", err, res)
	}

	// Verificar se branch foi criada/ativada
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = dir
	out, _ := cmd.Output()
	if strings.TrimSpace(string(out)) != "feature-test" {
		t.Fatalf("esperava estar na branch feature-test, mas está em: %s", strings.TrimSpace(string(out)))
	}

	// 3. Voltar para a main/master (ou a branch inicial)
	initialBranch := "master"
	if strings.Contains(string(out), "main") {
		initialBranch = "main"
	}
	res, err = tool.Execute(context.Background(), json.RawMessage(`{"action": "checkout", "name": "master"}`))
	if err != nil || !res.Success {
		// Tenta 'main' se falhar 'master'
		res, err = tool.Execute(context.Background(), json.RawMessage(`{"action": "checkout", "name": "main"}`))
		if err != nil || !res.Success {
			t.Fatalf("falha no branch checkout para %s: %v, res: %+v", initialBranch, err, res)
		}
	}

	// 4. Bloqueio de flags perigosas
	res, _ = tool.Execute(context.Background(), json.RawMessage(`{"action": "create", "name": "bad-branch --force"}`))
	if res.Success {
		t.Fatal("esperava erro de seguranca com flag --force")
	}
}
