package diff_replace_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crom/crom-agente/internal/tools/diff_replace"
)

func TestDiffReplaceTool(t *testing.T) {
	ws := t.TempDir()
	tool := diff_replace.NewDiffReplaceTool(ws, true)

	content := "linha 1\nlinha 2\nbloco para substituir\nlinha 4\nbloco para substituir\nlinha 6"
	testFile := filepath.Join(ws, "test.txt")
	_ = os.WriteFile(testFile, []byte(content), 0644)

	// 1. Substituição bem-sucedida especificando intervalo de linhas para remover ambiguidade
	args := json.RawMessage(`{
		"path": "test.txt",
		"start_line": 1,
		"end_line": 3,
		"target_content": "bloco para substituir",
		"replacement_content": "bloco modificado"
	}`)
	res, err := tool.Execute(context.Background(), args)
	if err != nil || !res.Success {
		t.Fatalf("erro ao executar diff_replace: %v, res: %+v", err, res)
	}

	// Verificar alteração
	data, _ := os.ReadFile(testFile)
	expected := "linha 1\nlinha 2\nbloco modificado\nlinha 4\nbloco para substituir\nlinha 6"
	if string(data) != expected {
		t.Fatalf("substituição incorreta. Esperava:\n%s\nObteve:\n%s", expected, string(data))
	}

	// 2. Erro de ambiguidade (sem especificar intervalo de linhas)
	// Restaurar arquivo para ter múltiplas ocorrências de target_content
	_ = os.WriteFile(testFile, []byte(content), 0644)

	argsAmbiguous := json.RawMessage(`{
		"path": "test.txt",
		"target_content": "bloco para substituir",
		"replacement_content": "bloco modificado"
	}`)
	res, _ = tool.Execute(context.Background(), argsAmbiguous)
	if res.Success {
		t.Fatal("esperava erro de ambiguidade por ter múltiplas ocorrências")
	}
	if !strings.Contains(res.Error, "substituição ambígua") {
		t.Fatalf("erro esperado 'substituição ambígua', obteve: %s", res.Error)
	}

	// 3. Erro de conteúdo não encontrado
	argsMissing := json.RawMessage(`{
		"path": "test.txt",
		"target_content": "conteudo inexistente",
		"replacement_content": "novo"
	}`)
	res, _ = tool.Execute(context.Background(), argsMissing)
	if res.Success {
		t.Fatal("esperava erro de conteúdo não encontrado")
	}
}
