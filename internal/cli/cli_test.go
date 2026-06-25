package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/crom/crom-agente/internal/config"
	"github.com/spf13/cobra"
)

func executeCommand(root *cobra.Command, args ...string) (string, error) {
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)

	err := root.Execute()
	return buf.String(), err
}

func TestCLI_Version(t *testing.T) {
	output, err := executeCommand(rootCmd, "version")
	if err != nil {
		t.Fatalf("erro ao executar comando version: %v", err)
	}
	if !strings.Contains(output, "crom-agente") {
		t.Fatalf("esperado versão do agente, obteve: %q", output)
	}
}

func TestCLI_ConfigGlobalGetSet(t *testing.T) {
	// Setup do diretório global temporário
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	// Garante que o diretório ~/.crom existe e está vazio
	_, err := config.GlobalDir()
	if err != nil {
		t.Fatalf("erro ao obter global dir: %v", err)
	}

	// 1. Testa set
	_, err = executeCommand(rootCmd, "config", "global", "set", "default_provider", "anthropic")
	if err != nil {
		t.Fatalf("erro ao executar config global set: %v", err)
	}

	// 2. Testa get
	output, err := executeCommand(rootCmd, "config", "global", "get", "default_provider")
	if err != nil {
		t.Fatalf("erro ao executar config global get: %v", err)
	}
	if strings.TrimSpace(output) != "anthropic" {
		t.Fatalf("esperado 'anthropic', obteve %q", output)
	}

	// 3. Testa list
	listOut, err := executeCommand(rootCmd, "config", "global", "list")
	if err != nil {
		t.Fatalf("erro ao executar config global list: %v", err)
	}
	if !strings.Contains(listOut, "default_provider:                 anthropic") {
		t.Fatalf("esperado listar provider atualizado, obteve: %q", listOut)
	}
}

func TestCLI_ConfigEnv(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	// 1. Define variável
	_, err := executeCommand(rootCmd, "config", "env", "set", "OPENAI_API_KEY", "sk-1234567890abcdef")
	if err != nil {
		t.Fatalf("erro ao executar config env set: %v", err)
	}

	// 2. Lista com máscara
	listOut, err := executeCommand(rootCmd, "config", "env", "list")
	if err != nil {
		t.Fatalf("erro ao executar config env list: %v", err)
	}
	if !strings.Contains(listOut, "OPENAI_API_KEY=sk-1***********cdef") {
		t.Fatalf("esperado chave mascarada, obteve: %q", listOut)
	}

	// 3. Lista sem máscara (--reveal)
	revealOut, err := executeCommand(rootCmd, "config", "env", "list", "--reveal")
	if err != nil {
		t.Fatalf("erro ao executar config env list --reveal: %v", err)
	}
	if !strings.Contains(revealOut, "OPENAI_API_KEY=sk-1234567890abcdef") {
		t.Fatalf("esperado chave revelada, obteve: %q", revealOut)
	}
}

func TestCLI_ConfigWorkspace(t *testing.T) {
	tempWorkspace := t.TempDir()
	workspacePath = tempWorkspace // Atualiza a variável global usada na CLI

	// 1. Testa set
	_, err := executeCommand(rootCmd, "config", "workspace", "set", "provider", "gemini")
	if err != nil {
		t.Fatalf("erro ao definir config de workspace: %v", err)
	}

	// 2. Testa get
	getOut, err := executeCommand(rootCmd, "config", "workspace", "get", "provider")
	if err != nil {
		t.Fatalf("erro ao ler config de workspace: %v", err)
	}
	if strings.TrimSpace(getOut) != "gemini" {
		t.Fatalf("esperado 'gemini', obteve %q", getOut)
	}

	// 3. Testa set int pointer
	_, err = executeCommand(rootCmd, "config", "workspace", "set", "max_iterations", "42")
	if err != nil {
		t.Fatalf("erro ao definir max_iterations de workspace: %v", err)
	}

	// 4. Testa list
	listOut, err := executeCommand(rootCmd, "config", "workspace", "list")
	if err != nil {
		t.Fatalf("erro ao executar config workspace list: %v", err)
	}
	if !strings.Contains(listOut, "provider:                 gemini") || !strings.Contains(listOut, "max_iterations:           42") {
		t.Fatalf("listagem incorreta do workspace: %q", listOut)
	}
}

func TestCLI_ConfigResolved(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	tempWorkspace := t.TempDir()
	workspacePath = tempWorkspace

	// Define valor global
	_, _ = executeCommand(rootCmd, "config", "global", "set", "default_provider", "openai")

	// Define override no workspace
	_, _ = executeCommand(rootCmd, "config", "workspace", "set", "provider", "gemini")

	// Testa resolved padrão (deve ser gemini porque workspace sobrepõe global)
	resOut, err := executeCommand(rootCmd, "config", "resolved")
	if err != nil {
		t.Fatalf("erro ao executar config resolved: %v", err)
	}
	if !strings.Contains(resOut, "Provider:           gemini") {
		t.Fatalf("esperado provider resolvido como 'gemini', obteve: %q", resOut)
	}

	// Testa resolved com flag (CLI flag --provider sobrepõe tudo)
	resOutFlag, err := executeCommand(rootCmd, "config", "resolved", "--provider", "anthropic")
	if err != nil {
		t.Fatalf("erro ao executar config resolved com flag: %v", err)
	}
	if !strings.Contains(resOutFlag, "Provider:           anthropic") {
		t.Fatalf("esperado provider resolvido como 'anthropic', obteve: %q", resOutFlag)
	}
}

func TestCLI_Workspace(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	wsDir := t.TempDir()

	// 1. Testa add
	out, err := executeCommand(rootCmd, "workspace", "add", wsDir, "--name", "my-test-workspace")
	if err != nil {
		t.Fatalf("erro ao adicionar workspace via CLI: %v", err)
	}
	if !strings.Contains(out, "my-test-workspace") {
		t.Fatalf("esperado mensagem de sucesso para my-test-workspace, obteve %q", out)
	}

	// 2. Testa list
	listOut, err := executeCommand(rootCmd, "workspace", "list")
	if err != nil {
		t.Fatalf("erro ao listar workspaces via CLI: %v", err)
	}
	if !strings.Contains(listOut, "my-test-workspace") {
		t.Fatalf("esperado my-test-workspace listado, obteve %q", listOut)
	}

	// 3. Testa status --all
	statusOut, err := executeCommand(rootCmd, "status", "--all")
	if err != nil {
		t.Fatalf("erro ao executar status --all via CLI: %v", err)
	}
	if !strings.Contains(statusOut, "my-test-workspace") {
		t.Fatalf("esperado status para my-test-workspace, obteve %q", statusOut)
	}

	// 4. Testa remove
	removeOut, err := executeCommand(rootCmd, "workspace", "remove", "my-test-workspace")
	if err != nil {
		t.Fatalf("erro ao remover workspace via CLI: %v", err)
	}
	if !strings.Contains(removeOut, "removido") {
		t.Fatalf("esperado mensagem de remoção, obteve %q", removeOut)
	}
}

func TestCLI_Session(t *testing.T) {
	tempWorkspace := t.TempDir()
	storagePath = tempWorkspace // Atualiza a variável global usada na CLI

	// 1. Testa create
	createOut, err := executeCommand(rootCmd, "session", "create", "sessao-teste")
	if err != nil {
		t.Fatalf("erro ao criar sessão via CLI: %v", err)
	}
	if !strings.Contains(createOut, "criada com sucesso") {
		t.Fatalf("esperado mensagem de sucesso para sessao-teste, obteve %q", createOut)
	}

	// 2. Testa list
	listOut, err := executeCommand(rootCmd, "session", "list")
	if err != nil {
		t.Fatalf("erro ao listar sessões via CLI: %v", err)
	}
	if !strings.Contains(listOut, "sessao-teste") {
		t.Fatalf("esperado sessao-teste listada, obteve %q", listOut)
	}

	// 3. Testa delete
	deleteOut, err := executeCommand(rootCmd, "session", "delete", "sessao-teste")
	if err != nil {
		t.Fatalf("erro ao excluir sessão via CLI: %v", err)
	}
	if !strings.Contains(deleteOut, "excluída com sucesso") {
		t.Fatalf("esperado mensagem de exclusão de sessao-teste, obteve %q", deleteOut)
	}

	// 4. Lista novamente para verificar se sumiu
	listOut2, err := executeCommand(rootCmd, "session", "list")
	if err != nil {
		t.Fatalf("erro ao listar sessões após exclusão: %v", err)
	}
	if strings.Contains(listOut2, "sessao-teste") {
		t.Fatalf("sessao-teste não deveria estar listada após exclusão: %q", listOut2)
	}
}
