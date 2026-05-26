// Package terminal provides a pipe-based terminal backend for grut.
// It starts a shell process with piped stdin/stdout/stderr and captures
// output into a scrollback buffer. This is a v1 implementation using
// plain pipes; full PTY emulation can be added later.
package terminal

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sync"
)

// maxLineBytes caps the length of a single output line to prevent memory
// exhaustion from pathological output (e.g., a single multi-GiB line).
const maxLineBytes = 1024 * 1024 // 1 MiB

// Runner abstracts the terminal backend so that panels can use a mock
// implementation in tests.
type Runner interface {
	// Write sends data to the shell's stdin.
	Write(data []byte) error
	// Lines returns a snapshot of the current output lines (thread-safe).
	Lines() []string
	// Close kills the shell process and waits for it to exit.
	Close() error
	// Done returns a channel that is closed when the shell process exits.
	Done() <-chan struct{}
	// ExitCode returns the process exit code, or -1 if still running.
	ExitCode() int
}

// Terminal manages a shell process with piped stdin/stdout/stderr.
// It reads output in background goroutines and stores lines in a
// thread-safe scrollback buffer with a configurable maximum.
type Terminal struct {
	stdin    io.WriteCloser
	cmd      *exec.Cmd
	done     chan struct{}
	lines    []string
	maxLines int
	exitCode int
	mu       sync.RWMutex
}

// Compile-time interface check.
var _ Runner = (*Terminal)(nil)

// DefaultShell returns the platform-appropriate default shell.
// On Windows it returns "cmd.exe"; on Unix it checks $SHELL and
// falls back to "/bin/sh".
func DefaultShell() string {
	if runtime.GOOS == "windows" {
		return "cmd.exe"
	}
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}
	return "/bin/sh"
}

// New starts a shell process and begins reading its output.
// The provided context controls the lifetime of the shell process;
// cancelling it will terminate the subprocess.
// If shell is empty, DefaultShell() is used. If maxLines is <= 0,
// it defaults to 10000.
func New(ctx context.Context, shell string, maxLines int) (*Terminal, error) {
	if shell == "" {
		shell = DefaultShell()
	}
	if maxLines <= 0 {
		maxLines = 10000
	}
	cmd := exec.CommandContext(ctx, shell)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("creating stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("creating stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, fmt.Errorf("starting shell %q: %w", shell, err)
	}
	t := &Terminal{
		cmd:      cmd,
		stdin:    stdin,
		maxLines: maxLines,
		done:     make(chan struct{}),
		exitCode: -1,
	}
	// Wait for both stream readers to finish before calling cmd.Wait.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); t.readStream(stdout) }()
	go func() { defer wg.Done(); t.readStream(stderr) }()
	go func() {
		wg.Wait()
		t.waitForProcess()
	}()
	return t, nil
}

// Write sends data to the shell's stdin. Returns an error if the write
// fails (e.g. process has exited and pipe is closed).
func (t *Terminal) Write(data []byte) error {
	_, err := t.stdin.Write(data)
	return err
}

// Lines returns a copy of the current output lines. Safe for concurrent use.
func (t *Terminal) Lines() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	cp := make([]string, len(t.lines))
	copy(cp, t.lines)
	return cp
}

// LinesWindow returns only the visible line window and the total scrollback
// length. offsetFromBottom follows the panel convention: 0 means newest lines.
func (t *Terminal) LinesWindow(offsetFromBottom, height int) ([]string, int) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	total := len(t.lines)
	if height <= 0 || total == 0 {
		return nil, total
	}
	if offsetFromBottom < 0 {
		offsetFromBottom = 0
	}
	end := total - offsetFromBottom
	if end < 0 {
		end = 0
	}
	if end > total {
		end = total
	}
	start := end - height
	if start < 0 {
		start = 0
	}
	cp := make([]string, end-start)
	copy(cp, t.lines[start:end])
	return cp, total
}

// Close closes stdin, kills the process if still running, and waits for
// the done channel to be closed.
func (t *Terminal) Close() error {
	// Close stdin first to signal the shell to exit.
	_ = t.stdin.Close()
	// Kill the process if it's still running.
	if t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
	}
	// Wait for the process to finish and goroutines to clean up.
	<-t.done
	return nil
}

// Done returns a channel that is closed when the shell process exits.
func (t *Terminal) Done() <-chan struct{} {
	return t.done
}

// ExitCode returns the process exit code. Returns -1 if the process
// is still running or was killed.
func (t *Terminal) ExitCode() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.exitCode
}

// readStream reads lines from a reader (stdout or stderr) and appends
// them to the scrollback buffer, trimming when maxLines is exceeded.
// Individual lines are capped at maxLineBytes — lines exceeding this
// limit are silently dropped to prevent memory exhaustion.
func (t *Terminal) readStream(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineBytes)
	for scanner.Scan() {
		line := scanner.Text()
		t.mu.Lock()
		t.lines = append(t.lines, line)
		if len(t.lines) > t.maxLines {
			excess := len(t.lines) - t.maxLines
			// Copy to a new slice to avoid retaining the old backing array.
			trimmed := make([]string, t.maxLines)
			copy(trimmed, t.lines[excess:])
			t.lines = trimmed
		}
		t.mu.Unlock()
	}
}

// waitForProcess waits for the command to exit and records the exit code.
func (t *Terminal) waitForProcess() {
	err := t.cmd.Wait()
	t.mu.Lock()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.exitCode = exitErr.ExitCode()
		}
		// If it's not an ExitError, exitCode stays -1 (killed/signaled).
	} else {
		t.exitCode = 0
	}
	t.mu.Unlock()
	close(t.done)
}
