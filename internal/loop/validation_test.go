package loop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgenticLoop_loadLocalRules(t *testing.T) {
	wsDir := t.TempDir()

	// Cria arquivo .cromrules na raiz
	rootRulesPath := filepath.Join(wsDir, ".cromrules")
	err := os.WriteFile(rootRulesPath, []byte("Regra Raiz 1"), 0644)
	if err != nil {
		t.Fatalf("falha ao criar arquivo de teste: %v", err)
	}

	// Cria pasta .crom e arquivo .cromrules dentro
	cromDir := filepath.Join(wsDir, ".crom")
	err = os.Mkdir(cromDir, 0755)
	if err != nil {
		t.Fatalf("falha ao criar diretorio .crom: %v", err)
	}
	cromRulesPath := filepath.Join(cromDir, ".cromrules")
	err = os.WriteFile(cromRulesPath, []byte("Regra Crom 1"), 0644)
	if err != nil {
		t.Fatalf("falha ao criar arquivo de teste em .crom: %v", err)
	}

	al := &AgenticLoop{}
	rulesStr := al.loadLocalRules(wsDir)

	if !strings.Contains(rulesStr, "Regra Raiz 1") {
		t.Errorf("esperava conter a regra da raiz, obteve: %s", rulesStr)
	}
	if !strings.Contains(rulesStr, "Regra Crom 1") {
		t.Errorf("esperava conter a regra de .crom, obteve: %s", rulesStr)
	}
	if !strings.Contains(rulesStr, "=== Regras de .cromrules ===") {
		t.Errorf("esperava conter cabecalho de regra da raiz, obteve: %s", rulesStr)
	}
	if !strings.Contains(rulesStr, "=== Regras de .crom/.cromrules ===") {
		t.Errorf("esperava conter cabecalho de regra do .crom, obteve: %s", rulesStr)
	}
}

func TestAgenticLoop_loadLocalRules_Empty(t *testing.T) {
	wsDir := t.TempDir()
	al := &AgenticLoop{}
	rulesStr := al.loadLocalRules(wsDir)

	if rulesStr != "" {
		t.Errorf("esperava string vazia para workspace sem regras, obteve: %s", rulesStr)
	}
}

func TestAgenticLoop_detectStack(t *testing.T) {
	wsDir := t.TempDir()
	al := &AgenticLoop{}

	// Teste com workspace vazio
	stack := al.detectStack(wsDir)
	if stack != "Desconhecida" {
		t.Errorf("esperava 'Desconhecida', obteve: %s", stack)
	}

	// Teste com Go
	os.WriteFile(filepath.Join(wsDir, "go.mod"), []byte("module teste"), 0644)
	stack = al.detectStack(wsDir)
	if !strings.Contains(stack, "Go (golang)") {
		t.Errorf("esperava detectar Go, obteve: %s", stack)
	}

	// Teste com Node
	os.WriteFile(filepath.Join(wsDir, "package.json"), []byte("{}"), 0644)
	stack = al.detectStack(wsDir)
	if !strings.Contains(stack, "Go") || !strings.Contains(stack, "Node.js") {
		t.Errorf("esperava detectar Go e Node.js, obteve: %s", stack)
	}
}
