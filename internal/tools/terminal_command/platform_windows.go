//go:build windows

package terminal_command

import "os/exec"

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

// killProcessTree encerra o processo. (Sem grupo de processos no Windows; para
// árvores completas seria necessário taskkill /T, fora do escopo atual.)
func killProcessTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
