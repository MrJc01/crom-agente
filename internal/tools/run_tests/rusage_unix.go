//go:build !windows

package run_tests

import (
	"os/exec"
	"syscall"
)

func getMaxRSS(c *exec.Cmd) int64 {
	if c.ProcessState == nil {
		return 0
	}
	if rusage, ok := c.ProcessState.SysUsage().(*syscall.Rusage); ok {
		return int64(rusage.Maxrss) // Em KB no Linux
	}
	return 0
}
