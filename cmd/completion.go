package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const shellPowerShell = "powershell"

func newCompletionCmd() *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:       "completion [bash|zsh|fish|powershell]",
		Short:     "Generate shell completion scripts",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"bash", "zsh", "fish", shellPowerShell},
		RunE: func(cmd *cobra.Command, args []string) error {
			out, closeOut, err := completionOutput(cmd, output)
			if err != nil {
				return err
			}
			if closeOut != nil {
				defer func() { _ = closeOut() }()
			}
			root := cmd.Root()

			switch shell := strings.ToLower(args[0]); shell {
			case "bash":
				return root.GenBashCompletion(out)
			case "zsh":
				return root.GenZshCompletion(out)
			case "fish":
				return root.GenFishCompletion(out, true)
			case shellPowerShell:
				return root.GenPowerShellCompletion(out)
			default:
				return fmt.Errorf("unsupported shell %q (supported: bash, zsh, fish, %s)", args[0], shellPowerShell)
			}
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "", "Write the completion script to a file")
	return cmd
}

func completionOutput(cmd *cobra.Command, output string) (io.Writer, func() error, error) {
	if strings.TrimSpace(output) == "" {
		return cmd.OutOrStdout(), nil, nil
	}
	dir := filepath.Dir(output)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, nil, fmt.Errorf("create completion output directory: %w", err)
		}
	}
	f, err := os.Create(output)
	if err != nil {
		return nil, nil, fmt.Errorf("create completion output file: %w", err)
	}
	return f, f.Close, nil
}
