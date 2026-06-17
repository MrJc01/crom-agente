package cli

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBlackBox_CLI_Execution(t *testing.T) {
	tempDir := t.TempDir()

	binPath := filepath.Join(tempDir, "crom-agente")
	if runtime.GOOS == "windows" {
		binPath += ".exe"
	}

	// Compila o binário para o teste black-box (tags headless para evitar dependências de tray)
	cmdBuild := exec.Command("go", "build", "-tags", "headless", "-o", binPath, "../../cmd/crom-agente/main.go")
	outBuild, err := cmdBuild.CombinedOutput()
	if err != nil {
		t.Fatalf("erro ao compilar binário para teste: %v\nOutput: %s", err, string(outBuild))
	}

	// 1. Executa 'version'
	cmdVersion := exec.Command(binPath, "version")
	outVersion, err := cmdVersion.CombinedOutput()
	if err != nil {
		t.Fatalf("erro ao executar version: %v", err)
	}
	if !strings.Contains(string(outVersion), "crom-agente") {
		t.Errorf("saída do comando version inválida: %s", string(outVersion))
	}

	// 2. Executa '--help'
	cmdHelp := exec.Command(binPath, "--help")
	outHelp, err := cmdHelp.CombinedOutput()
	if err != nil {
		t.Fatalf("erro ao executar help: %v", err)
	}
	if !strings.Contains(string(outHelp), "crom-agente") {
		t.Errorf("saída do comando help inválida: %s", string(outHelp))
	}
}
