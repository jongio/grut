package terminal

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Unit tests (pipe-based, no shell needed)
// ---------------------------------------------------------------------------

func TestReadStreamAppendsLines(t *testing.T) {
	term := &Terminal{
		maxLines: 100,
		done:     make(chan struct{}),
		exitCode: -1,
	}

	r, w := io.Pipe()
	go term.readStream(r)

	for i := 0; i < 5; i++ {
		_, _ = fmt.Fprintf(w, "line %d\n", i)
	}
	_ = w.Close()

	// Give the goroutine time to process.
	time.Sleep(100 * time.Millisecond)

	lines := term.Lines()
	require.Len(t, lines, 5)
	assert.Equal(t, "line 0", lines[0])
	assert.Equal(t, "line 4", lines[4])
}

func TestScrollbackTrimming(t *testing.T) {
	term := &Terminal{
		maxLines: 3,
		done:     make(chan struct{}),
		exitCode: -1,
	}

	r, w := io.Pipe()
	go term.readStream(r)

	for i := 0; i < 10; i++ {
		_, _ = fmt.Fprintf(w, "line%d\n", i)
	}
	_ = w.Close()

	time.Sleep(100 * time.Millisecond)

	lines := term.Lines()
	assert.Len(t, lines, 3)
	// Should keep the last 3 lines.
	assert.Equal(t, "line7", lines[0])
	assert.Equal(t, "line8", lines[1])
	assert.Equal(t, "line9", lines[2])
}

func TestScrollbackSingleLine(t *testing.T) {
	term := &Terminal{
		maxLines: 1,
		done:     make(chan struct{}),
		exitCode: -1,
	}

	r, w := io.Pipe()
	go term.readStream(r)

	for i := 0; i < 5; i++ {
		_, _ = fmt.Fprintf(w, "line%d\n", i)
	}
	_ = w.Close()

	time.Sleep(100 * time.Millisecond)

	lines := term.Lines()
	assert.Len(t, lines, 1)
	assert.Equal(t, "line4", lines[0])
}

func TestLinesReturnsDefensiveCopy(t *testing.T) {
	term := &Terminal{
		maxLines: 100,
		done:     make(chan struct{}),
		exitCode: -1,
		lines:    []string{"a", "b", "c"},
	}

	lines := term.Lines()
	assert.Len(t, lines, 3)

	// Mutating the copy should not affect the original.
	lines[0] = "modified"
	assert.Equal(t, "a", term.lines[0])
}

func TestLinesEmptyInitially(t *testing.T) {
	term := &Terminal{
		maxLines: 100,
		done:     make(chan struct{}),
		exitCode: -1,
	}

	lines := term.Lines()
	assert.Empty(t, lines)
}

func TestExitCodeDefault(t *testing.T) {
	term := &Terminal{
		maxLines: 100,
		done:     make(chan struct{}),
		exitCode: -1,
	}
	assert.Equal(t, -1, term.ExitCode())
}

func TestConcurrentReadWrite(t *testing.T) {
	term := &Terminal{
		maxLines: 1000,
		done:     make(chan struct{}),
		exitCode: -1,
	}

	r, w := io.Pipe()
	go term.readStream(r)

	var wg sync.WaitGroup

	// Concurrent writes to the pipe.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, _ = fmt.Fprintf(w, "concurrent %d\n", n)
		}(i)
	}

	// Concurrent reads of Lines().
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = term.Lines()
		}()
	}

	wg.Wait()
	_ = w.Close()

	time.Sleep(100 * time.Millisecond)

	lines := term.Lines()
	assert.Equal(t, 10, len(lines))
}

func TestReadStreamCapsLineLength(t *testing.T) {
	term := &Terminal{
		maxLines: 100,
		done:     make(chan struct{}),
		exitCode: -1,
	}

	r, w := io.Pipe()
	go term.readStream(r)

	// Write in a goroutine because io.Pipe blocks when the reader stops
	// consuming (scanner hits token-too-long and exits).
	go func() {
		_, _ = fmt.Fprintf(w, "short line\n")
		// Write a line exceeding maxLineBytes — scanner will error and stop.
		long := strings.Repeat("X", maxLineBytes+100)
		_, _ = fmt.Fprintf(w, "%s\n", long)
		_, _ = fmt.Fprintf(w, "after\n")
		_ = w.Close()
	}()

	// Wait for readStream to finish (pipe close causes reader exit).
	// Poll with short intervals instead of a single long sleep to avoid
	// flakiness on slow CI machines.
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for readStream to process lines")
		default:
		}
		if len(term.Lines()) >= 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	lines := term.Lines()
	// At minimum the first line should have been read before scanner
	// stopped at the oversized token.
	require.GreaterOrEqual(t, len(lines), 1)
	assert.Equal(t, "short line", lines[0])
	// No stored line should exceed the limit.
	for i, l := range lines {
		assert.LessOrEqual(t, len(l), maxLineBytes,
			"line %d exceeds maxLineBytes", i)
	}
}

func TestDefaultShell(t *testing.T) {
	shell := DefaultShell()
	assert.NotEmpty(t, shell)

	if runtime.GOOS == "windows" {
		assert.Equal(t, "cmd.exe", shell)
	} else {
		// Should be $SHELL or /bin/sh.
		assert.True(t, shell == "/bin/sh" || strings.HasSuffix(shell, "sh") || strings.HasSuffix(shell, "zsh") || strings.HasSuffix(shell, "bash") || strings.HasSuffix(shell, "fish"),
			"unexpected shell: %s", shell)
	}
}

// ---------------------------------------------------------------------------
// Integration tests (requires a shell)
// ---------------------------------------------------------------------------

func TestNewCreatesRunningProcess(t *testing.T) {
	term, err := New(context.Background(), "", 100)
	require.NoError(t, err)
	defer func() { _ = term.Close() }()

	// Process should be running (Done not closed).
	select {
	case <-term.Done():
		t.Fatal("process should still be running")
	default:
	}

	assert.Equal(t, -1, term.ExitCode())
}

func TestNewWithInvalidShell(t *testing.T) {
	_, err := New(context.Background(), "/nonexistent/shell/binary", 100)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "starting shell")
}

func TestWriteAndReadOutput(t *testing.T) {
	term, err := New(context.Background(), "", 100)
	require.NoError(t, err)
	defer func() { _ = term.Close() }()

	err = term.Write([]byte("echo hello_from_terminal\n"))
	require.NoError(t, err)

	// Poll until we see the expected output.
	lines := waitForOutput(t, term, func(lines []string) bool {
		for _, l := range lines {
			if strings.Contains(l, "hello_from_terminal") {
				return true
			}
		}
		return false
	}, 5*time.Second)

	found := false
	for _, l := range lines {
		if strings.Contains(l, "hello_from_terminal") {
			found = true
			break
		}
	}
	assert.True(t, found, "expected 'hello_from_terminal' in output, got: %v", lines)
}

func TestScrollbackLimitIntegration(t *testing.T) {
	term, err := New(context.Background(), "", 5)
	require.NoError(t, err)
	defer func() { _ = term.Close() }()

	// Generate many lines of output.
	for i := 0; i < 20; i++ {
		err = term.Write([]byte(fmt.Sprintf("echo scrollback_line_%d\n", i)))
		require.NoError(t, err)
	}

	// Wait for output to be processed.
	waitForOutput(t, term, func(lines []string) bool {
		return len(lines) >= 5
	}, 5*time.Second)

	// Allow all output to settle.
	time.Sleep(500 * time.Millisecond)

	lines := term.Lines()
	assert.LessOrEqual(t, len(lines), 5, "scrollback should be limited to 5 lines, got %d", len(lines))
}

func TestCloseKillsProcess(t *testing.T) {
	term, err := New(context.Background(), "", 100)
	require.NoError(t, err)

	err = term.Close()
	assert.NoError(t, err)

	// Done channel should be closed after Close.
	select {
	case <-term.Done():
		// expected
	case <-time.After(5 * time.Second):
		t.Fatal("process did not exit after Close")
	}
}

func TestDoneSignalsOnExit(t *testing.T) {
	term, err := New(context.Background(), "", 100)
	require.NoError(t, err)

	// Tell the shell to exit.
	err = term.Write([]byte("exit\n"))
	require.NoError(t, err)

	select {
	case <-term.Done():
		// expected
	case <-time.After(5 * time.Second):
		_ = term.Close()
		t.Fatal("process did not exit after 'exit' command")
	}

	assert.Equal(t, 0, term.ExitCode())
}

func TestWriteAfterClose(t *testing.T) {
	term, err := New(context.Background(), "", 100)
	require.NoError(t, err)

	_ = term.Close()

	// Writing after close should return an error.
	err = term.Write([]byte("echo test\n"))
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// waitForOutput polls the terminal's Lines() until the predicate is satisfied
// or the timeout expires.
func waitForOutput(t *testing.T, term *Terminal, pred func([]string) bool, timeout time.Duration) []string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		lines := term.Lines()
		if pred(lines) {
			return lines
		}
		time.Sleep(50 * time.Millisecond)
	}
	lines := term.Lines()
	t.Fatalf("timed out waiting for output condition; current lines: %v", lines)
	return lines
}
