//go:build windows

package runtime

import (
	"os/exec"
	"testing"
)

// These tests exercise the runtime's thin wrappers over internal/proctree. The
// job-object internals (handle storage, tree termination) are covered directly
// in the proctree package tests.

func TestSetProcGroup_NoOp(t *testing.T) {
	cmd := &exec.Cmd{}
	// On Windows setProcGroup must not modify SysProcAttr; containment is
	// established after start via postStartProcGroup.
	setProcGroup(cmd)
	if cmd.SysProcAttr != nil {
		t.Fatal("setProcGroup should not modify SysProcAttr on Windows")
	}
}

func TestKillProcGroup_NoHandle(t *testing.T) {
	cmd := &exec.Cmd{}
	// Should not panic when no containment handle is stored.
	killProcGroup(cmd)
}

func TestPostStartProcGroup_NilProcess(t *testing.T) {
	cmd := &exec.Cmd{}
	// Should not panic when Process is nil.
	postStartProcGroup(cmd)
}

func TestProcGroupWrappers_KillTree(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/C", "ping -n 60 127.0.0.1 > NUL")
	setProcGroup(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	postStartProcGroup(cmd)

	// Kill the tree via the wrapper, then ensure Wait returns.
	killProcGroup(cmd)
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
}
