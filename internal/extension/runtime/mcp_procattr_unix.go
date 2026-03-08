//go:build !windows

package runtime

import (
	"os/exec"
	"syscall"
)

// setProcGroup places the subprocess in its own process group so the
// entire tree can be killed together, preventing orphaned children from
// persisting after the parent is terminated (CWE-269).
func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcGroup sends SIGKILL to the entire process group rooted at cmd.
func killProcGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		// Negative PID targets the process group.
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
