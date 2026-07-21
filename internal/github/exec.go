package github

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/jongio/grut/internal/proctree"
)

// ghExec runs a `gh` CLI command and returns its trimmed stdout.
// This is an escape hatch for operations not supported by the go-github SDK.
//
// The command runs through proctree so that cancelling ctx terminates the whole
// `gh` process tree (gh spawns git and network helpers) rather than orphaning
// its children, and a descendant that inherits the output pipes cannot block
// Wait forever (CWE-269, CWE-400).
func ghExec(ctx context.Context, args ...string) (string, error) {
	cmd := proctree.Command(ctx, "gh", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := proctree.Run(cmd); err != nil {
		return "", fmt.Errorf("gh %s: %w: %s", strings.Join(args, " "), err, stderr.String())
	}
	return strings.TrimSpace(stdout.String()), nil
}
