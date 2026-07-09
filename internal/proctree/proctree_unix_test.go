//go:build !windows

package proctree

import (
	"os/exec"
	"syscall"
	"testing"
)

func TestConfigure_SetsSetpgid(t *testing.T) {
	cmd := &exec.Cmd{}
	Configure(cmd)
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		t.Fatal("Configure must set SysProcAttr.Setpgid on Unix")
	}
}

func TestConfigure_PreservesExistingSysProcAttr(t *testing.T) {
	cmd := &exec.Cmd{}
	// Simulate a caller that already set an attribute; Configure must not
	// clobber it, only add Setpgid.
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	Configure(cmd)
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		t.Fatal("Configure must set Setpgid while preserving the existing struct")
	}
}
