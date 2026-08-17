package cmd

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/update"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// buildRootCommand — PersistentPreRunE (profiling paths)
// ---------------------------------------------------------------------------

func TestBuildRootCommand_PprofFlagRegistered(t *testing.T) {
	root, cleanup := buildRootCommand()
	defer cleanup()
	flag := root.PersistentFlags().Lookup("pprof")
	require.NotNil(t, flag, "--pprof persistent flag must be registered")
	assert.Equal(t, "string", flag.Value.Type())
	assert.Equal(t, "", flag.DefValue)
}

func TestBuildRootCommand_PprofFlagInheritedBySubcommands(t *testing.T) {
	root, cleanup := buildRootCommand()
	defer cleanup()
	for _, sub := range root.Commands() {
		flag := sub.InheritedFlags().Lookup("pprof")
		assert.NotNil(t, flag, "subcommand %q must inherit --pprof", sub.Name())
	}
}

func TestBuildRootCommand_InvalidPprofPort(t *testing.T) {
	// Invalid pprof port should not crash — it prints a warning and continues.
	root, cleanup := buildRootCommand()
	defer cleanup()

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"--pprof", "invalid-port", "version"})

	err := root.Execute()
	assert.NoError(t, err, "invalid pprof port should not prevent command execution")
}

func TestBuildRootCommand_PprofPortOutOfRange(t *testing.T) {
	root, cleanup := buildRootCommand()
	defer cleanup()

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"--pprof", "99999", "version"})

	err := root.Execute()
	assert.NoError(t, err, "out-of-range pprof port should not prevent command execution")
}

func TestBuildRootCommand_PprofPortZero(t *testing.T) {
	root, cleanup := buildRootCommand()
	defer cleanup()

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"--pprof", "0", "version"})

	err := root.Execute()
	assert.NoError(t, err, "pprof port 0 should not prevent command execution")
}

func TestBuildRootCommand_PprofOccupiedPortReportsCommandStderr(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port

	root, cleanup := buildRootCommand()
	defer cleanup()
	var stderr bytes.Buffer
	root.SetErr(&stderr)
	root.SetArgs([]string{"--pprof", strconv.Itoa(port), "version"})

	require.NoError(t, root.Execute())
	assert.Contains(t, stderr.String(), "could not bind pprof server")
	assert.Contains(t, stderr.String(), strconv.Itoa(port))
}

func TestBuildRootCommand_PprofCleanupStopsServer(t *testing.T) {
	port := reserveTCPPort(t)
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	root, cleanup := buildRootCommand()
	defer cleanup()
	root.SetArgs([]string{"--pprof", strconv.Itoa(port), "version"})

	require.NoError(t, root.Execute())
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	require.NoError(t, err, "pprof listener should be bound before command execution returns")
	require.NoError(t, conn.Close())

	cleanup()
	conn, err = net.DialTimeout("tcp", addr, 100*time.Millisecond)
	if err == nil {
		conn.Close()
		t.Fatal("pprof listener remained reachable after cleanup")
	}
}

// ---------------------------------------------------------------------------
// buildRootCommand — cleanup idempotency
// ---------------------------------------------------------------------------

func TestBuildRootCommand_CleanupIdempotent(t *testing.T) {
	_, cleanup := buildRootCommand()
	// Calling cleanup multiple times should not panic.
	assert.NotPanics(t, func() {
		cleanup()
		cleanup()
		cleanup()
	}, "cleanup must be idempotent")
}

func TestBuildRootCommand_CleanupConcurrentSafe(t *testing.T) {
	_, cleanup := buildRootCommand()
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cleanup()
		}()
	}
	wg.Wait()
}

func TestBuildRootCommand_SamplingFlagsRegistered(t *testing.T) {
	root, cleanup := buildRootCommand()
	defer cleanup()

	mutexFlag := root.PersistentFlags().Lookup("mutex-profile-fraction")
	require.NotNil(t, mutexFlag)
	assert.Equal(t, "int", mutexFlag.Value.Type())

	blockFlag := root.PersistentFlags().Lookup("block-profile-rate")
	require.NotNil(t, blockFlag)
	assert.Equal(t, "int", blockFlag.Value.Type())
}

// ---------------------------------------------------------------------------
// buildRootCommand — CPU + mem profile combined
// ---------------------------------------------------------------------------

func TestBuildRootCommand_BothProfilesWritten(t *testing.T) {
	tmp := t.TempDir()
	cpuPath := filepath.Join(tmp, "cpu.prof")
	memPath := filepath.Join(tmp, "mem.prof")

	root, cleanup := buildRootCommand()
	defer cleanup()
	root.SetArgs([]string{
		"--cpu-profile", cpuPath,
		"--mem-profile", memPath,
		"version",
	})

	err := root.Execute()
	assert.NoError(t, err)
	cleanup() // flush profiles

	cpuInfo, err := os.Stat(cpuPath)
	require.NoError(t, err, "CPU profile file must be created")
	assert.Greater(t, cpuInfo.Size(), int64(0))

	memInfo, err := os.Stat(memPath)
	require.NoError(t, err, "memory profile file must be created")
	assert.Greater(t, memInfo.Size(), int64(0))
}

// ---------------------------------------------------------------------------
// buildRootCommand — version flag
// ---------------------------------------------------------------------------

func TestBuildRootCommand_VersionFlag(t *testing.T) {
	root, cleanup := buildRootCommand()
	defer cleanup()

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"--version"})

	err := root.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), config.AppVersion,
		"--version should print the app version")
}

// ---------------------------------------------------------------------------
// openLogPath — additional edge cases
// ---------------------------------------------------------------------------

func TestOpenLogPath_DotDotRejected(t *testing.T) {
	// A relative path starting with ".." should be rejected.
	f := openLogPath("../relative.log")
	assert.Nil(t, f, "relative ../path must be rejected")
}

func TestOpenLogPath_DotPathRejected(t *testing.T) {
	f := openLogPath("./relative.log")
	assert.Nil(t, f, "relative ./path must be rejected")
}

func TestOpenLogPath_AppendMode(t *testing.T) {
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "append-test.log")

	// Write something first.
	f1 := openLogPath(logPath)
	require.NotNil(t, f1)
	_, err := f1.WriteString("first\n")
	require.NoError(t, err)
	f1.Close()

	// Open again — should append, not overwrite.
	f2 := openLogPath(logPath)
	require.NotNil(t, f2)
	_, err = f2.WriteString("second\n")
	require.NoError(t, err)
	f2.Close()

	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "first")
	assert.Contains(t, string(data), "second")
}

// ---------------------------------------------------------------------------
// Execute — structure test
// ---------------------------------------------------------------------------

func TestExecute_FunctionExists(t *testing.T) {
	// We can't easily test Execute() without it trying to start the TUI,
	// but we can verify it compiles and is callable.
	// This is a compile-time test — if Execute signature changes, this breaks.
	fn := Execute
	assert.NotNil(t, fn, "Execute function must exist")
}

// ---------------------------------------------------------------------------
// buildRootCommand RunE — reset-welcome via root
// ---------------------------------------------------------------------------

func TestBuildRootCommand_ResetWelcomeViaRoot(t *testing.T) {
	root, cleanup := buildRootCommand()
	defer cleanup()

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"--reset-welcome"})

	err := root.Execute()
	// resetWelcomeState may fail if session dir is not writable, but
	// it exercises the flag-check path in RunE.
	_ = err // result depends on environment; we just want no panic
}

// ---------------------------------------------------------------------------
// showUpdateNotification — additional paths
// ---------------------------------------------------------------------------

func TestShowUpdateNotification_ClosedChannel(t *testing.T) {
	var buf bytes.Buffer
	ch := make(chan *update.UpdateInfo, 1)
	ch <- &update.UpdateInfo{
		CurrentVersion: "0.1.0",
		LatestVersion:  "9.9.9",
	}
	showUpdateNotification(&buf, ch)
	assert.Contains(t, buf.String(), "0.1.0")
	assert.Contains(t, buf.String(), "9.9.9")
}

func reserveTCPPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())
	return port
}
