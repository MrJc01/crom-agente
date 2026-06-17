package tools

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestValidatePath_NormalizationAndWindows(t *testing.T) {
	ws := t.TempDir()

	// 1. Caminho normal relativo
	res, err := ValidatePath(ws, "file.txt", true)
	if err != nil {
		t.Fatalf("erro ao validar caminho relativo simples: %v", err)
	}
	expected := filepath.Join(ws, "file.txt")
	if res != expected {
		t.Errorf("esperava %q, obteve %q", expected, res)
	}

	// 2. Caminho relativo com barras do Windows (mesmo rodando em Unix)
	winPath := "subdir\\file.txt"
	resWin, err := ValidatePath(ws, winPath, true)
	if err != nil {
		t.Fatalf("erro ao validar caminho com barras invertidas: %v", err)
	}
	
	if !strings.HasPrefix(resWin, ws) {
		t.Errorf("caminho retornado %q deveria estar sob o workspace %q", resWin, ws)
	}

	// 3. Simula tratamento de barras normalizando para formato cross-platform
	normalizedWinPath := filepath.ToSlash(winPath) // vira "subdir/file.txt"
	resNorm, err := ValidatePath(ws, normalizedWinPath, true)
	if err != nil {
		t.Fatalf("erro ao validar caminho normalizado: %v", err)
	}
	expectedNorm := filepath.Join(ws, "subdir", "file.txt")
	if resNorm != expectedNorm {
		t.Errorf("esperava %q, obteve %q", expectedNorm, resNorm)
	}

	// 4. Teste de path traversal
	_, err = ValidatePath(ws, "../outside.txt", true)
	if err == nil {
		t.Error("esperava erro de path traversal")
	}
}
