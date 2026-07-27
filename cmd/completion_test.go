package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompletionCmd_GeneratesPowerShell(t *testing.T) {
	root := &cobra.Command{Use: "app"}
	root.AddCommand(newCompletionCmd())

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"completion", "powershell"})

	err := root.Execute()

	require.NoError(t, err)
	assert.Contains(t, out.String(), "Register-ArgumentCompleter")
}

func TestCompletionCmd_WritesOutputFile(t *testing.T) {
	root := &cobra.Command{Use: "app"}
	root.AddCommand(newCompletionCmd())
	outPath := filepath.Join(t.TempDir(), "nested", "app.ps1")

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"completion", "powershell", "--output", outPath})

	err := root.Execute()

	require.NoError(t, err)
	assert.Empty(t, out.String())
	data, err := os.ReadFile(outPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "Register-ArgumentCompleter")
}

func TestCompletionCmd_RejectsUnsupportedShell(t *testing.T) {
	root := &cobra.Command{Use: "app"}
	root.AddCommand(newCompletionCmd())

	root.SetArgs([]string{"completion", "xonsh"})

	err := root.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported shell")
}

func TestRootRegistersCompletionCommand(t *testing.T) {
	root, cleanup := newRootCommand()
	defer cleanup()

	completionCmd, _, err := root.Find([]string{"completion"})

	require.NoError(t, err)
	require.NotNil(t, completionCmd)
	assert.Equal(t, "completion", completionCmd.Name())
}
