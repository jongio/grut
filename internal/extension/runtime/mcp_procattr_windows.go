//go:build windows

package runtime

import (
	"os/exec"

	"github.com/jongio/grut/internal/proctree"
)

// The process-tree containment logic lives in the shared internal/proctree
// package. These thin wrappers preserve the runtime's existing call sites while
// avoiding a duplicate implementation (CWE-269).

// setProcGroup configures the subprocess for whole-tree termination before
// start. On Windows this is a no-op; containment is established after start.
func setProcGroup(cmd *exec.Cmd) { proctree.Configure(cmd) }

// postStartProcGroup assigns the started process to a Job Object so the whole
// tree is terminated together. Must be called after cmd.Start succeeds.
func postStartProcGroup(cmd *exec.Cmd) { proctree.AfterStart(cmd) }

// killProcGroup terminates the entire process tree rooted at cmd.
func killProcGroup(cmd *exec.Cmd) { proctree.Kill(cmd) }
