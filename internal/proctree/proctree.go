// Package proctree runs child processes as a contained process tree so the
// entire tree (the direct child and every descendant) can be terminated
// together.
//
// On Unix the child is placed in its own process group (Setpgid) and the group
// is signalled with SIGKILL. On Windows the child is assigned to a Job Object
// configured with JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE, so closing the job handle
// terminates the whole tree. This prevents orphaned grandchild processes — for
// example git spawning git-remote-https, ssh, or a credential helper — that
// would otherwise leak when a command's context is cancelled or the application
// exits. Leaked children hold memory, file descriptors, and network
// connections, which is a resource-exhaustion risk over a long session
// (CWE-269, CWE-400).
//
// The platform-specific primitives Configure, AfterStart, and Kill live in
// proctree_unix.go and proctree_windows.go. Most callers should use the Command
// and Run helpers, which wire those primitives together with a bounded
// WaitDelay so a descendant that inherits an stdout/stderr pipe cannot block
// Wait forever.
package proctree

import (
	"context"
	"os/exec"
	"time"
)

// DefaultWaitDelay bounds how long Wait blocks after the process exits, or
// after its context is cancelled, waiting for inherited stdout/stderr pipes to
// close. Without it a descendant that inherits a pipe can block Wait — leaking
// the process, its output-copy goroutine, and file descriptors — indefinitely.
const DefaultWaitDelay = 5 * time.Second

// Command creates an *exec.Cmd bound to ctx that is configured for whole-tree
// termination. Cancelling ctx (or the ctx timing out) kills the entire process
// tree, not just the direct child. Callers set Stdout/Stderr/Dir/Env as usual,
// then either run it to completion with Run (for buffered commands) or manage
// the lifecycle themselves via Start/AfterStart/Kill (for streaming commands).
func Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	Configure(cmd)
	cmd.WaitDelay = DefaultWaitDelay
	// Replace the default cancel (which only kills the direct child) with a
	// whole-tree kill. The Process.Kill fallback covers the small window
	// between Start and AfterStart before the containment group exists.
	cmd.Cancel = func() error {
		Kill(cmd)
		if cmd.Process != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
	return cmd
}

// Run starts cmd, assigns it to its containment group, waits for it to exit,
// and guarantees the process tree and any OS handle are released — including on
// the normal-exit path. Use it for buffered, run-to-completion commands whose
// Stdout/Stderr are set to in-memory buffers. For streaming commands that need
// pipes, call Start, AfterStart, and Kill directly.
func Run(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	AfterStart(cmd)
	err := cmd.Wait()
	// Idempotent: tears down the group and releases the OS handle even when
	// the process exited on its own (no context cancellation).
	Kill(cmd)
	return err
}
