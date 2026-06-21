package tools

import (
	"fmt"
	"os/exec"
	"runtime"
)

// IsCgroupsAvailable verifica se o systemd-run está disponível para uso do usuário sem senha.
// Se estiver, retorna true.
func IsCgroupsAvailable() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	// Testa se o comando systemd-run existe e pode ser chamado com --user --scope
	cmd := exec.Command("systemd-run", "--user", "--scope", "true")
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

// WrapCommandWithCgroup recebe um comando bash e o empacota num escopo transiente do systemd
// com os limites de recursos aplicados, protegendo o sistema host.
func WrapCommandWithCgroup(command string, memoryLimitMB int, cpuQuota int) (string, []string) {
	if !IsCgroupsAvailable() {
		// Se não tiver cgroup via systemd-run disponível, aplica apenas nice (fallback)
		return "nice", []string{"-n", "15", "bash", "-c", command}
	}

	memMax := fmt.Sprintf("MemoryMax=%dM", memoryLimitMB)
	cpuQ := fmt.Sprintf("CPUQuota=%d%%", cpuQuota)
	
	// Utiliza --user para não exigir root (necessita sessão logind ou persistência pro user)
	// --scope cria um cgroup temporário. -p TasksMax limita fork-bombs.
	args := []string{
		"--user",
		"--scope",
		"-p", memMax,
		"-p", cpuQ,
		"-p", "TasksMax=200",
		"bash", "-c", command,
	}

	return "systemd-run", args
}
