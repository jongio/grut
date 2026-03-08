//go:build windows

package runtime

import "os/exec"

// setProcGroup is a no-op on Windows.
// TODO(security): CWE-269 — Use Windows Job Objects to contain child
// processes so the entire tree is terminated when the parent exits.
func setProcGroup(_ *exec.Cmd) {}

// killProcGroup is a no-op on Windows; only the direct child is killed.
// TODO(security): CWE-269 — Implement Job Object–based tree kill.
func killProcGroup(_ *exec.Cmd) {}
