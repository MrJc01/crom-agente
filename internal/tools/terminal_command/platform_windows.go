//go:build windows

package terminal_command

import (
	"os/exec"
	"strconv"
	"syscall"
)

// No Windows não há Setpgid nem chroot. O agrupamento de processos e o jail via
// chroot não são suportados; o processo roda normalmente e é encerrado com Kill.
func configureSysProcAttr(c *exec.Cmd, newGroup bool, jailDir string) {
	_ = c
	_ = newGroup
	_ = jailDir
}

// interruptProc: Windows não tem SIGINT confiável para processos filhos; encerra
// o processo diretamente.
func interruptProc(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}

// killProcessTree encerra a árvore inteira via `taskkill /T /F` (o Windows não
// tem grupos de processos como o Unix). Se o taskkill falhar, mata ao menos o
// processo principal.
func killProcessTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	kill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
	kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := kill.Run(); err != nil {
		_ = cmd.Process.Kill()
	}
}
