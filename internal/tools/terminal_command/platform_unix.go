//go:build !windows

package terminal_command

import (
	"os/exec"
	"syscall"
)

// configureSysProcAttr isola o processo num grupo separado (Setpgid) para poder
// matar toda a árvore com um único sinal, e aplica chroot quando há jail.
func configureSysProcAttr(c *exec.Cmd, newGroup bool, jailDir string) {
	attr := &syscall.SysProcAttr{Setpgid: newGroup}
	if jailDir != "" {
		attr.Chroot = jailDir
	}
	c.SysProcAttr = attr
}

// interruptProc envia SIGINT (Ctrl+C) ao processo.
func interruptProc(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return cmd.Process.Signal(syscall.SIGINT)
}

// killProcessTree envia SIGKILL para todo o grupo de processos (árvore) e garante
// o encerramento do processo principal.
func killProcessTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pid := cmd.Process.Pid
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	_ = cmd.Process.Kill()
}
