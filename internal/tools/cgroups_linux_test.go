package tools

import (
	"strings"
	"testing"
)

func TestWrapCommandWithCgroup_Linux(t *testing.T) {
	cmdName, cmdArgs := WrapCommandWithCgroup("echo hello", 1024, 50)

	if cmdName != "nice" && cmdName != "systemd-run" {
		t.Errorf("expected nice or systemd-run, got %s", cmdName)
	}

	joinedArgs := strings.Join(cmdArgs, " ")

	if cmdName == "nice" {
		if !strings.Contains(joinedArgs, "-n 15 bash -c echo hello") {
			t.Errorf("unexpected args for nice fallback: %s", joinedArgs)
		}
	} else {
		// systemd-run
		if !strings.Contains(joinedArgs, "--user --scope") {
			t.Errorf("expected --user --scope in args, got %s", joinedArgs)
		}
		if !strings.Contains(joinedArgs, "MemoryMax=1024M") {
			t.Errorf("expected MemoryMax=1024M in args, got %s", joinedArgs)
		}
		if !strings.Contains(joinedArgs, "CPUQuota=50%") {
			t.Errorf("expected CPUQuota=50%% in args, got %s", joinedArgs)
		}
		if !strings.Contains(joinedArgs, "bash -c echo hello") {
			t.Errorf("expected bash -c echo hello in args, got %s", joinedArgs)
		}
	}
}
