package git_conflict

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
