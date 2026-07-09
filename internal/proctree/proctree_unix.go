//go:build !windows

package proctree

import (
	"os/exec"
	"syscall"
)

// Configure places the subprocess in its own process group so the entire tree
// can be killed together, preventing orphaned children from persisting after
// the parent is terminated (CWE-269). It is called before cmd.Start.
func Configure(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// AfterStart is a no-op on Unix; process-group membership is configured before
// start via Configure (SysProcAttr.Setpgid).
func AfterStart(_ *exec.Cmd) {}

// Kill sends SIGKILL to the entire process group rooted at cmd. It is safe to
// call multiple times and on a process that has already exited.
func Kill(cmd *exec.Cmd) {
	if cmd.Process != nil {
		// A negative PID targets the whole process group.
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
