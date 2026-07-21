package proctree

import (
	"context"
	"errors"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

// exitZeroCmd returns a platform-appropriate command that exits 0 immediately.
func exitZeroCmd() (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd.exe", []string{"/c", "exit", "0"}
	}
	return "sh", []string{"-c", "exit 0"}
}

// exitOneCmd returns a platform-appropriate command that exits 1 immediately.
func exitOneCmd() (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd.exe", []string{"/c", "exit", "1"}
	}
	return "sh", []string{"-c", "exit 1"}
}

// sleepCmd returns a platform-appropriate command that runs for ~30 seconds.
func sleepCmd() (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd.exe", []string{"/c", "ping -n 30 127.0.0.1 > NUL"}
	}
	return "sh", []string{"-c", "sleep 30"}
}

func TestCommand_ConfiguresWaitDelayAndCancel(t *testing.T) {
	name, args := exitZeroCmd()
	cmd := Command(context.Background(), name, args...)
	if cmd.WaitDelay != DefaultWaitDelay {
		t.Fatalf("WaitDelay = %v, want %v", cmd.WaitDelay, DefaultWaitDelay)
	}
	if cmd.Cancel == nil {
		t.Fatal("Command must set a Cancel function for whole-tree termination")
	}
}

func TestRun_Success(t *testing.T) {
	name, args := exitZeroCmd()
	cmd := Command(context.Background(), name, args...)
	if err := Run(cmd); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}
}

func TestRun_NonZeroExitPropagates(t *testing.T) {
	name, args := exitOneCmd()
	cmd := Command(context.Background(), name, args...)
	err := Run(cmd)
	if err == nil {
		t.Fatal("Run: expected non-nil error for non-zero exit")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("Run: expected *exec.ExitError, got %T: %v", err, err)
	}
}

func TestRun_ContextCancelTerminatesPromptly(t *testing.T) {
	name, args := sleepCmd()
	ctx, cancel := context.WithCancel(context.Background())
	cmd := Command(ctx, name, args...)

	// Cancel shortly after start; Run must return well before the command's
	// natural ~30s runtime, proving the whole tree was terminated.
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	err := Run(cmd)
	elapsed := time.Since(start)
	cancel()

	if err == nil {
		t.Fatal("Run: expected non-nil error after context cancellation")
	}
	// Allow generous slack for CI, but far below the 30s natural runtime.
	if elapsed > 20*time.Second {
		t.Fatalf("Run: took %v after cancel; process tree may not have been killed", elapsed)
	}
}

func TestRun_AlreadyCancelledContext(t *testing.T) {
	name, args := exitZeroCmd()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd := Command(ctx, name, args...)
	if err := Run(cmd); err == nil {
		t.Fatal("Run: expected error when context is already cancelled")
	}
}

func TestKill_NilProcessNoPanic(t *testing.T) {
	// Kill must be safe on a command that was never started.
	Kill(&exec.Cmd{})
}

func TestKill_Idempotent(t *testing.T) {
	name, args := sleepCmd()
	cmd := Command(context.Background(), name, args...)
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	AfterStart(cmd)
	// Multiple Kill calls must not panic.
	Kill(cmd)
	Kill(cmd)
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
}
