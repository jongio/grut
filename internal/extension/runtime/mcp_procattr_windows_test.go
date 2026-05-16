//go:build windows

package runtime

import (
	"os/exec"
	"testing"

	"golang.org/x/sys/windows"
)

func TestPostStartProcGroup_AssignsJobObject(t *testing.T) {
	// Start a long-running process (ping -t loops indefinitely on Windows).
	cmd := exec.Command("cmd.exe", "/C", "ping -n 60 127.0.0.1 > NUL")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		killProcGroup(cmd)
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	postStartProcGroup(cmd)

	// Verify a Job Object handle was stored for this cmd.
	v, ok := jobHandles.Load(cmd)
	if !ok {
		t.Fatal("expected job handle to be stored after postStartProcGroup")
	}
	job := v.(windows.Handle)
	if job == 0 {
		t.Fatal("stored job handle is zero")
	}
}

func TestPostStartProcGroup_NilProcess(t *testing.T) {
	cmd := &exec.Cmd{}
	// Should not panic when Process is nil.
	postStartProcGroup(cmd)

	if _, ok := jobHandles.Load(cmd); ok {
		t.Fatal("expected no job handle for cmd with nil Process")
	}
}

func TestKillProcGroup_TerminatesJobTree(t *testing.T) {
	// Start a process that spawns a child (cmd /C start /B ping).
	cmd := exec.Command("cmd.exe", "/C", "ping -n 60 127.0.0.1 > NUL")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	postStartProcGroup(cmd)

	// Verify handle exists.
	if _, ok := jobHandles.Load(cmd); !ok {
		t.Fatal("expected job handle")
	}

	// Kill via job object.
	killProcGroup(cmd)

	// Handle should be cleaned up.
	if _, ok := jobHandles.Load(cmd); ok {
		t.Fatal("expected job handle to be removed after killProcGroup")
	}

	// Process should be dead (Wait should return quickly).
	err := cmd.Wait()
	if err == nil {
		t.Log("process exited cleanly (may have been killed by job close)")
	}
}

func TestKillProcGroup_NoHandle(t *testing.T) {
	cmd := &exec.Cmd{}
	// Should not panic when no handle is stored.
	killProcGroup(cmd)
}

func TestSetProcGroup_NoOp(t *testing.T) {
	cmd := &exec.Cmd{}
	// Should not panic or modify cmd.
	setProcGroup(cmd)
	if cmd.SysProcAttr != nil {
		t.Fatal("setProcGroup should not modify SysProcAttr on Windows")
	}
}
