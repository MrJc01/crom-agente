package security

import (
	"strings"
	"testing"
)

func TestRenderDiff_NewFile(t *testing.T) {
	result := RenderDiff("main.go", "", "package main\n\nfunc main() {}\n")

	if !strings.Contains(result, "DIFF ZONE: main.go") {
		t.Error("deveria conter o cabeçalho com nome do arquivo")
	}

	// Tudo é inserção (verde)
	if !strings.Contains(result, "\x1b[32m+ package main\x1b[0m") {
		t.Error("deveria marcar conteúdo novo em verde")
	}
}

func TestRenderDiff_DeletedContent(t *testing.T) {
	old := "linha1\nlinha2\nlinha3\n"
	new := "linha1\nlinha3\n"

	result := RenderDiff("test.txt", old, new)

	// Deveria ter pelo menos uma deleção
	if !strings.Contains(result, "\x1b[31m-") {
		t.Error("deveria marcar conteúdo removido em vermelho")
	}
}

func TestRenderDiff_Modification(t *testing.T) {
	old := "func hello() {\n\tfmt.Println(\"hello\")\n}\n"
	new := "func hello() {\n\tfmt.Println(\"world\")\n}\n"

	result := RenderDiff("hello.go", old, new)

	if !strings.Contains(result, "DIFF ZONE: hello.go") {
		t.Error("deveria conter o cabeçalho com nome do arquivo")
	}

	// Deveria ter deleção (vermelho) e inserção (verde)
	if !strings.Contains(result, "\x1b[31m-") {
		t.Error("deveria marcar a linha antiga em vermelho")
	}
	if !strings.Contains(result, "\x1b[32m+") {
		t.Error("deveria marcar a linha nova em verde")
	}
}

func TestRenderDiff_IdenticalContent(t *testing.T) {
	content := "package main\n\nfunc main() {}\n"
	result := RenderDiff("same.go", content, content)

	// Não deveria ter nenhuma deleção ou inserção
	if strings.Contains(result, "\x1b[31m-") || strings.Contains(result, "\x1b[32m+") {
		t.Error("não deveria ter diferenças em conteúdo idêntico")
	}
}

func TestRenderDiff_EmptyToContent(t *testing.T) {
	result := RenderDiff("new.txt", "", "linha1\nlinha2\n")

	// Conta as linhas verdes (inserção)
	greenCount := strings.Count(result, "\x1b[32m+")
	if greenCount < 2 {
		t.Errorf("esperado pelo menos 2 linhas verdes, obteve %d", greenCount)
	}
}

func TestRenderDiff_ContentToEmpty(t *testing.T) {
	result := RenderDiff("deleted.txt", "linha1\nlinha2\nlinha3\n", "")

	// Conta as linhas vermelhas (deleção)
	redCount := strings.Count(result, "\x1b[31m-")
	if redCount < 3 {
		t.Errorf("esperado pelo menos 3 linhas vermelhas, obteve %d", redCount)
	}
}

func TestRenderDiff_LargeFile(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 100; i++ {
		sb.WriteString("linha de código original\n")
	}
	old := sb.String()

	sb.Reset()
	for i := 0; i < 100; i++ {
		if i == 50 {
			sb.WriteString("linha modificada no meio\n")
		} else {
			sb.WriteString("linha de código original\n")
		}
	}
	new := sb.String()

	result := RenderDiff("large.go", old, new)
	if result == "" {
		t.Error("deveria gerar saída não vazia para arquivos grandes")
	}
}
