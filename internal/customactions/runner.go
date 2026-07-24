package customactions

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/proctree"
	"github.com/jongio/grut/internal/terminal"
)

const shellPwshExe = "pwsh.exe"

// Result describes a completed custom action run.
type Result struct {
	Action   config.CustomAction
	Output   string
	Err      error
	Duration time.Duration
	ExitCode int
}

// ShellInvocation resolves a shell command line for the current platform.
func ShellInvocation(shell, command string) (string, []string) {
	return shellInvocationForGOOS(runtime.GOOS, shell, command)
}

func shellInvocationForGOOS(goos, shell, command string) (string, []string) {
	if strings.TrimSpace(shell) == "" {
		shell = terminal.DefaultShell()
	}
	base := strings.ToLower(filepath.Base(shell))
	if goos == "windows" {
		switch base {
		case "powershell", "powershell.exe", "pwsh", shellPwshExe:
			return shell, []string{"-NoProfile", "-Command", command}
		default:
			return shell, []string{"/c", command}
		}
	}
	return shell, []string{"-c", command}
}

// Run executes action through the configured shell and captures combined
// stdout and stderr without blocking the Bubble Tea update loop.
func Run(ctx context.Context, action config.CustomAction, defaultDir, shell string) Result {
	name, args := ShellInvocation(shell, action.Command)
	dir := action.WorkDir
	if strings.TrimSpace(dir) == "" {
		dir = defaultDir
	}

	start := time.Now()
	var output bytes.Buffer
	cmd := proctree.Command(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := proctree.Run(cmd)

	exitCode := 0
	if err != nil {
		exitCode = -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
	}
	return Result{
		Action:   action,
		Output:   output.String(),
		Err:      err,
		Duration: time.Since(start),
		ExitCode: exitCode,
	}
}
