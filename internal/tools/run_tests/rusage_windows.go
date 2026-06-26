//go:build windows

package run_tests

import "os/exec"

func getMaxRSS(c *exec.Cmd) int64 {
	return 0
}
