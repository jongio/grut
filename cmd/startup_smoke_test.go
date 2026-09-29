package cmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const startupSmokeHelperEnv = "GRUT_STARTUP_SMOKE_HELPER"

type startupSmokeOutput struct {
	mu       sync.Mutex
	buffer   bytes.Buffer
	rendered chan struct{}
	once     sync.Once
}

func newStartupSmokeOutput() *startupSmokeOutput {
	return &startupSmokeOutput{rendered: make(chan struct{})}
}

func (o *startupSmokeOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	n, err := o.buffer.Write(p)
	rendered := strings.Contains(o.buffer.String(), "No file selected")
	o.mu.Unlock()
	if rendered {
		o.once.Do(func() { close(o.rendered) })
	}
	return n, err
}

func (o *startupSmokeOutput) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buffer.String()
}

func TestRootCommand_StartupSmoke_Subprocess(t *testing.T) {
	if os.Getenv(startupSmokeHelperEnv) == "1" {
		runRootStartupSmoke(t)
		return
	}
	if testing.Short() {
		t.Skip("subprocess integration test skipped with -short")
	}

	testRoot := t.TempDir()
	configHome := filepath.Join(testRoot, "config")
	dataHome := filepath.Join(testRoot, "data")
	projectDir := filepath.Join(testRoot, "project")
	require.NoError(t, os.MkdirAll(filepath.Join(configHome, config.AppName), 0o700))
	require.NoError(t, os.MkdirAll(projectDir, 0o700))
	require.NoError(t, os.WriteFile(
		filepath.Join(configHome, config.AppName, "config.toml"),
		[]byte("[general]\nshow_first_run_help = false\n\n[ai]\nenabled = false\n\n[session]\nenabled = false\n"),
		0o600,
	))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "README.md"), []byte("# Smoke test\n"), 0o600))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRootCommand_StartupSmoke_Subprocess$", "-test.v")
	command.Dir = projectDir
	command.Env = append(
		os.Environ(),
		startupSmokeHelperEnv+"=1",
		"GRUT_FORCE_TERMINAL=1",
		"GRUT_LOG=",
		"XDG_CONFIG_HOME="+configHome,
		"XDG_DATA_HOME="+dataHome,
	)
	output, err := command.CombinedOutput()

	if ctx.Err() != nil {
		t.Fatalf("grut startup smoke test timed out: %v\n%s", ctx.Err(), output)
	}
	require.NoErrorf(t, err, "grut startup smoke subprocess failed:\n%s", output)
}

func runRootStartupSmoke(t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	output := newStartupSmokeOutput()
	root, cleanup := buildRootCommandWithProgramFactory(func(model tea.Model) *tea.Program {
		program := tea.NewProgram(
			model,
			tea.WithContext(ctx),
			tea.WithInput(nil),
			tea.WithOutput(output),
			tea.WithWindowSize(100, 30),
			tea.WithoutSignalHandler(),
		)
		go func() {
			select {
			case <-output.rendered:
				program.Send(tea.KeyPressMsg{Code: 'q'})
			case <-ctx.Done():
			}
		}()
		return program
	})
	defer cleanup()

	var stderr bytes.Buffer
	root.SetArgs([]string{"--no-ai"})
	root.SetOut(io.Discard)
	root.SetErr(&stderr)

	err := root.Execute()
	require.NoErrorf(t, err, "grut startup failed: %s", stderr.String())
	require.NoError(t, ctx.Err())
	assert.Contains(t, output.String(), "No file selected")
}
