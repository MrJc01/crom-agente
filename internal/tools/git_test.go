package tools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupMockGitRepo inicializa um repositório git temporário para testes
func setupMockGitRepo(t *testing.T) string {
	dir := t.TempDir()

	// git init
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("falha ao inicializar git no teste: %v", err)
	}

	// Configurações básicas de user para o git commit funcionar
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

	statusTool := NewGitStatusTool(dir)
	logTool := NewGitLogTool(dir)
	diffTool := NewGitDiffTool(dir)
	addTool := NewGitAddTool(dir)
	commitTool := NewGitCommitTool(dir)

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

func TestGitConflict(t *testing.T) {
	dir := t.TempDir()
	tool := NewGitConflictTool(dir, true)

	// 1. Testa scan em diretório limpo
	res, err := tool.Execute(context.Background(), json.RawMessage(`{"action": "scan"}`))
	if err != nil || !res.Success {
		t.Fatalf("erro ao rodar scan: %v, res: %+v", err, res)
	}
	if !strings.Contains(res.Data, "Nenhum arquivo com conflitos") {
		t.Fatalf("esperava no conflicts, obteve: %s", res.Data)
	}

	// 2. Criar um arquivo com conflito mockado
	conflictContent := `linha comum
<<<<<<< HEAD
minha alteracao local
=======
alteracao da branch remota
>>>>>>> remote-branch
outra linha`
	
	conflictFile := filepath.Join(dir, "conflito.go")
	_ = os.WriteFile(conflictFile, []byte(conflictContent), 0644)

	// Scan deve encontrar o arquivo
	res, err = tool.Execute(context.Background(), json.RawMessage(`{"action": "scan"}`))
	if err != nil || !res.Success {
		t.Fatalf("erro ao rodar scan com conflito: %v, res: %+v", err, res)
	}
	if !strings.Contains(res.Data, "conflito.go") {
		t.Fatalf("esperava encontrar conflito.go no scan, obteve: %s", res.Data)
	}

	// Analyze deve extrair os blocos
	res, err = tool.Execute(context.Background(), json.RawMessage(`{"action": "analyze", "path": "conflito.go"}`))
	if err != nil || !res.Success {
		t.Fatalf("erro ao rodar analyze: %v, res: %+v", err, res)
	}

	var analysis struct {
		Conflicts []ConflictBlock `json:"conflicts"`
	}
	if err := json.Unmarshal([]byte(res.Data), &analysis); err != nil {
		t.Fatalf("erro ao desserializar analise: %v", err)
	}

	if len(analysis.Conflicts) != 1 {
		t.Fatalf("esperava 1 conflito, obteve: %d", len(analysis.Conflicts))
	}

	c := analysis.Conflicts[0]
	if c.Ours != "minha alteracao local" || c.Theirs != "alteracao da branch remota" || c.Marker != "remote-branch" {
		t.Fatalf("dados do conflito incorretos: %+v", c)
	}
}
