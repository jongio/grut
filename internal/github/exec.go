package github

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// ghExec runs a `gh` CLI command and returns its trimmed stdout.
// This is an escape hatch for operations not supported by the go-github SDK.
func ghExec(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("gh %s: %w: %s", strings.Join(args, " "), err, stderr.String())
	}
	return strings.TrimSpace(stdout.String()), nil
}
