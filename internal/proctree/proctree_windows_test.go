//go:build windows

package proctree

import (
	"os/exec"
	"testing"

	"golang.org/x/sys/windows"
)

func TestConfigure_NoOpOnWindows(t *testing.T) {
	cmd := &exec.Cmd{}
	Configure(cmd)
	if cmd.SysProcAttr != nil {
		t.Fatal("Configure must not set SysProcAttr on Windows")
	}
}

func TestAfterStart_AssignsJobObject(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/C", "ping -n 60 127.0.0.1 > NUL")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		Kill(cmd)
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	AfterStart(cmd)

	v, ok := jobHandles.Load(cmd)
	if !ok {
		t.Fatal("expected job handle to be stored after AfterStart")
	}
	job, ok := v.(windows.Handle)
	if !ok || job == 0 {
		t.Fatalf("stored job handle invalid: %v", v)
	}
}

func TestAfterStart_NilProcessStoresNothing(t *testing.T) {
	cmd := &exec.Cmd{}
	AfterStart(cmd)
	if _, ok := jobHandles.Load(cmd); ok {
		t.Fatal("expected no job handle for cmd with nil Process")
	}
}

func TestKill_TerminatesJobTreeAndCleansHandle(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/C", "ping -n 60 127.0.0.1 > NUL")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	AfterStart(cmd)

	if _, ok := jobHandles.Load(cmd); !ok {
		t.Fatal("expected job handle before Kill")
	}

	Kill(cmd)

	if _, ok := jobHandles.Load(cmd); ok {
		t.Fatal("expected job handle to be removed after Kill")
	}

	// Job close with KILL_ON_JOB_CLOSE terminates the tree; Wait returns.
	_ = cmd.Wait()
}
