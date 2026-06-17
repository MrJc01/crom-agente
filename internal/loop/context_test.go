package loop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectStack(t *testing.T) {
	ws := t.TempDir()

	al := &AgenticLoop{}

	// 1. Nada no workspace
	stack := al.detectStack(ws)
	if stack != "Desconhecida" {
		t.Fatalf("esperava 'Desconhecida', obteve '%s'", stack)
	}

	// 2. Go project
	_ = os.WriteFile(filepath.Join(ws, "go.mod"), []byte("module test"), 0644)
	stack = al.detectStack(ws)
	if !strings.Contains(stack, "Go (golang)") {
		t.Fatalf("esperava Go stack, obteve '%s'", stack)
	}

	// 3. Node project
	_ = os.WriteFile(filepath.Join(ws, "package.json"), []byte("{}"), 0644)
	stack = al.detectStack(ws)
	if !strings.Contains(stack, "Node.js") {
		t.Fatalf("esperava Node.js stack, obteve '%s'", stack)
	}
}

func TestLoadLocalRules(t *testing.T) {
	ws := t.TempDir()

	al := &AgenticLoop{}

	// 1. Sem regras
	rules := al.loadLocalRules(ws)
	if rules != "" {
		t.Fatalf("esperava regras vazias, obteve '%s'", rules)
	}

	// 2. Com .cromrules
	_ = os.WriteFile(filepath.Join(ws, ".cromrules"), []byte("rule 1"), 0644)
	rules = al.loadLocalRules(ws)
	if !strings.Contains(rules, "rule 1") || !strings.Contains(rules, "=== Regras de .cromrules ===") {
		t.Fatalf("esperava ver .cromrules, obteve '%s'", rules)
	}

	// 3. Com .voidrules também
	_ = os.WriteFile(filepath.Join(ws, ".voidrules"), []byte("rule 2"), 0644)
	rules = al.loadLocalRules(ws)
	if !strings.Contains(rules, "rule 2") || !strings.Contains(rules, "=== Regras de .voidrules ===") {
		t.Fatalf("esperava ver ambas as regras, obteve '%s'", rules)
	}
}

func TestErrorParser(t *testing.T) {
	output := `some compilation start
main.go:12:3: undefined: fmt.Println
another_file.js:45: error: missing semicolon
/path/to/source.rs:8:10: error[E0308]: mismatched types
exit status 2`

	errors := ParseTerminalErrors(output)
	if len(errors) < 2 {
		t.Fatalf("esperava extrair pelo menos 2 erros, obteve %d", len(errors))
	}

	// Verifica erro Go
	goErrFound := false
	for _, e := range errors {
		if e.File == "main.go" && e.Line == 12 && strings.Contains(e.Message, "undefined: fmt.Println") {
			goErrFound = true
			break
		}
	}
	if !goErrFound {
		t.Fatal("erro Go não foi parseado corretamente")
	}

	// Verifica formatação
	formatted := FormatContextualError(output)
	if !strings.Contains(formatted, "🔍 [ANÁLISE DE ERROS ESTRUTURADA]:") || !strings.Contains(formatted, "main.go") {
		t.Fatalf("erro formatado incorretamente: %s", formatted)
	}
}
