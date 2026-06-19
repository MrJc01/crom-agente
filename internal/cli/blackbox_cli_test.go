package cli

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBlackBox_CLI_TUI_Execution(t *testing.T) {
	tempDir := t.TempDir()

	binPath := filepath.Join(tempDir, "crom-agente-cli")
	if runtime.GOOS == "windows" {
		binPath += ".exe"
	}

	// Compila o binário do CLI
	cmdBuild := exec.Command("go", "build", "-tags", "headless", "-o", binPath, "../../cmd/crom-agente-cli/main.go")
	outBuild, err := cmdBuild.CombinedOutput()
	if err != nil {
		t.Fatalf("erro ao compilar crom-agente-cli: %v\nOutput: %s", err, string(outBuild))
	}

	// 1. Executa '--help'
	cmdHelp := exec.Command(binPath, "--help")
	outHelp, err := cmdHelp.CombinedOutput()
	if err != nil {
		t.Fatalf("erro ao executar help no CLI: %v", err)
	}
	if !strings.Contains(string(outHelp), "crom-agente-cli") {
		t.Errorf("saída do comando help do CLI inválida: %s", string(outHelp))
	}

	// 2. Executa com flags inválidas e verifica erro
	cmdInvalid := exec.Command(binPath, "--invalid-flag")
	outInvalid, err := cmdInvalid.CombinedOutput()
	if err == nil {
		t.Fatal("esperava erro ao usar flag inválida")
	}
	if !strings.Contains(string(outInvalid), "unknown flag") {
		t.Errorf("saída de erro inesperada: %s", string(outInvalid))
	}
}
