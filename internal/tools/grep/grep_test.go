package grep_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crom/crom-agente/internal/tools/grep"
)

func TestGrepTool(t *testing.T) {
	ws := t.TempDir()
	tool := grep.NewGrepTool(ws, true)

	_ = os.WriteFile(filepath.Join(ws, "file1.txt"), []byte("este é o padrao que procuramos\noutra linha"), 0644)
	_ = os.WriteFile(filepath.Join(ws, "file2.txt"), []byte("nada aqui\npadrao"), 0644)
	_ = os.WriteFile(filepath.Join(ws, "binary.png"), []byte{0x89, 'P', 'N', 'G', 0x00, 0x01, 'a', 'b'}, 0644) // Arquivo com byte nulo (binário)

	// 1. Busca simples (case-insensitive por default)
	args := json.RawMessage(`{"query": "padrao"}`)
	res, err := tool.Execute(context.Background(), args)
	if err != nil || !res.Success {
		t.Fatalf("erro ao rodar grep: %v, res: %+v", err, res)
	}

	if !strings.Contains(res.Data, "file1.txt") || !strings.Contains(res.Data, "file2.txt") {
		t.Fatalf("grep não encontrou ocorrências nos arquivos de texto: %s", res.Data)
	}
	if strings.Contains(res.Data, "binary.png") {
		t.Fatal("grep buscou no arquivo binário binary.png e retornou correspondência")
	}

	// 2. Busca Regex
	argsRegex := json.RawMessage(`{"query": "este é .*", "is_regex": true}`)
	resRegex, err := tool.Execute(context.Background(), argsRegex)
	if err != nil || !resRegex.Success {
		t.Fatalf("erro ao rodar grep com regex: %v", err)
	}
	if !strings.Contains(resRegex.Data, "este é o padrao") {
		t.Fatalf("regex não encontrou padrão: %s", resRegex.Data)
	}
}
