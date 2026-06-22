//go:build !linux

package tools

// IsCgroupsAvailable returns false on non-Linux platforms.
func IsCgroupsAvailable() bool {
	return false
}

// WrapCommandWithCgroup returns the command run with nice under bash.
func WrapCommandWithCgroup(command string, memoryLimitMB int, cpuQuota int) (string, []string) {
	return "nice", []string{"-n", "15", "bash", "-c", command}
}
