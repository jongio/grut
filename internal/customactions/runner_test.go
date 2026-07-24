package customactions

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
