//go:build !windows

package runtime

import (
	"os/exec"

	"github.com/jongio/grut/internal/proctree"
)

// The process-tree containment logic lives in the shared internal/proctree
// package. These thin wrappers preserve the runtime's existing call sites while
// avoiding a duplicate implementation (CWE-269).

// setProcGroup places the subprocess in its own process group before start so
// the entire tree can be killed together.
func setProcGroup(cmd *exec.Cmd) { proctree.Configure(cmd) }

// killProcGroup sends SIGKILL to the entire process group rooted at cmd.
func killProcGroup(cmd *exec.Cmd) { proctree.Kill(cmd) }

// postStartProcGroup is a no-op on Unix; process-group membership is configured
// before start via setProcGroup.
func postStartProcGroup(cmd *exec.Cmd) { proctree.AfterStart(cmd) }
