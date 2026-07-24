package customactions

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"github.com/jongio/grut/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShellInvocationForGOOS(t *testing.T) {
	tests := []struct {
		name     string
		goos     string
		shell    string
		command  string
		wantName string
		wantArgs []string
	}{
		{
			name:     "windows cmd",
			goos:     "windows",
			shell:    "cmd.exe",
			command:  "go test ./...",
			wantName: "cmd.exe",
			wantArgs: []string{"/c", "go test ./..."},
		},
		{
			name:     "windows powershell",
			goos:     "windows",
			shell:    shellPwshExe,
			command:  "go test ./...",
			wantName: shellPwshExe,
			wantArgs: []string{"-NoProfile", "-Command", "go test ./..."},
		},
		{
			name:     "unix shell",
			goos:     "linux",
			shell:    "/bin/zsh",
			command:  "go test ./...",
			wantName: "/bin/zsh",
			wantArgs: []string{"-c", "go test ./..."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotArgs := shellInvocationForGOOS(tt.goos, tt.shell, tt.command)
			assert.Equal(t, tt.wantName, gotName)
			assert.Equal(t, tt.wantArgs, gotArgs)
		})
	}
}

func TestShellInvocationForGOOS_WindowsPowerShellVariants(t *testing.T) {
	for _, shell := range []string{"powershell", "powershell.exe", "pwsh", shellPwshExe, `C:\Program Files\PowerShell\7\pwsh.exe`} {
		t.Run(shell, func(t *testing.T) {
			name, args := shellInvocationForGOOS("windows", shell, "go build")
			assert.Equal(t, shell, name)
			assert.Equal(t, []string{shellFlagNoProfile, shellFlagCommand, "go build"}, args)
		})
	}
}

func TestShellInvocationForGOOS_EmptyShellUsesDefault(t *testing.T) {
	// An empty shell resolves to the platform default shell rather than an
	// empty command name.
	name, args := shellInvocationForGOOS(runtime.GOOS, "", "echo hi")
	assert.NotEmpty(t, name)
	require.NotEmpty(t, args)
	assert.Equal(t, "echo hi", args[len(args)-1])
}

func TestShellInvocation_UsesRuntimeGOOS(t *testing.T) {
	name, args := ShellInvocation("", "echo hi")
	assert.NotEmpty(t, name)
	require.NotEmpty(t, args)
	// The command is always the final argument regardless of platform/shell.
	assert.Equal(t, "echo hi", args[len(args)-1])
}

func TestRun_Success(t *testing.T) {
	res := Run(
		context.Background(),
		config.CustomAction{Name: "echo", Command: "echo gruttest"},
		t.TempDir(),
		"",
	)
	require.NoError(t, res.Err)
	assert.Equal(t, 0, res.ExitCode)
	assert.Contains(t, res.Output, "gruttest")
	assert.Positive(t, res.Duration)
	assert.Equal(t, "echo", res.Action.Name)
}

func TestRun_FailureCapturesExitCode(t *testing.T) {
	res := Run(
		context.Background(),
		config.CustomAction{Name: "fail", Command: "exit 3"},
		t.TempDir(),
		"",
	)
	require.Error(t, res.Err)
	assert.Equal(t, 3, res.ExitCode)
}

func TestRun_UsesWorkDirWhenSet(t *testing.T) {
	dir := t.TempDir()
	// When WorkDir is set it takes precedence over defaultDir; the command
	// still runs and reports success.
	res := Run(
		context.Background(),
		config.CustomAction{Name: "ok", Command: "echo hi", WorkDir: dir},
		"/nonexistent-default-dir",
		"",
	)
	require.NoError(t, res.Err)
	assert.Equal(t, 0, res.ExitCode)
}

func TestRun_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res := Run(ctx, config.CustomAction{Name: "x", Command: "echo hi"}, t.TempDir(), "")
	// A pre-cancelled context must surface as an error, not a success.
	require.Error(t, res.Err)
	assert.NotEqual(t, 0, res.ExitCode)
	// Sanity: the recorded command name is preserved even on failure.
	assert.Equal(t, "x", res.Action.Name)
	_ = strings.TrimSpace(res.Output)
}
