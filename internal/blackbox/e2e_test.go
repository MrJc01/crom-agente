package blackbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var binPath string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "crom-e2e-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpDir)

	binPath = filepath.Join(tmpDir, "crom-agente")

	// Compila o binario programaticamente para testes de caixa preta
	cmd := exec.Command("go", "build", "-tags", "headless", "-o", binPath, "../../cmd/crom-agente/main.go")
	if err := cmd.Run(); err != nil {
		panic(err)
	}

	code := m.Run()
	os.Exit(code)
}

func TestE2EBlackbox_Version(t *testing.T) {
	cmd := exec.Command(binPath, "version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("erro ao executar version: %v. Saida: %s", err, string(out))
	}

	output := strings.TrimSpace(string(out))
	if !strings.HasPrefix(output, "crom-agente") {
		t.Errorf("esperava output comecando com 'crom-agente', obteve: %q", output)
	}
}

func TestE2EBlackbox_DaemonStatus(t *testing.T) {
	tempHome := t.TempDir()

	cmd := exec.Command(binPath, "daemon", "status")
	cmd.Env = append(os.Environ(), "HOME="+tempHome)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("erro ao executar daemon status: %v. Saida: %s", err, string(out))
	}

	output := string(out)
	if !strings.Contains(output, "Daemon inativo") {
		t.Errorf("esperava que a saida mencionasse 'Daemon inativo', obteve: %q", output)
	}
}

func TestE2EBlackbox_ConfigResolved(t *testing.T) {
	tempHome := t.TempDir()

	cmd := exec.Command(binPath, "config", "resolved")
	cmd.Env = append(os.Environ(), "HOME="+tempHome)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("erro ao executar config resolved: %v. Saida: %s", err, string(out))
	}

	output := string(out)
	if !strings.Contains(output, "Configuração Resolvida") {
		t.Errorf("esperava titulo 'Configuração Resolvida', obteve: %q", output)
	}
}
