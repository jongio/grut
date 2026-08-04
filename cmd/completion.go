package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

const (
	shellBash       = "bash"
	shellZsh        = "zsh"
	shellFish       = "fish"
	shellPowerShell = "powershell"
)

// supportedShells is the single source of truth for completion targets: it
// drives ValidArgs, argument validation, and the error message.
var supportedShells = []string{shellBash, shellZsh, shellFish, shellPowerShell}

func newCompletionCmd() *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:       "completion [bash|zsh|fish|powershell]",
		Short:     "Generate shell completion scripts",
		Args:      cobra.ExactArgs(1),
		ValidArgs: supportedShells,
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			shell := strings.ToLower(args[0])
			if !slices.Contains(supportedShells, shell) {
				return fmt.Errorf("unsupported shell %q (supported: %s)", args[0], strings.Join(supportedShells, ", "))
			}

			out, closeOut, err := completionOutput(cmd, output)
			if err != nil {
				return err
			}
			if closeOut != nil {
				defer func() {
					if cerr := closeOut(); err == nil {
						err = cerr
					}
				}()
			}
			root := cmd.Root()

			switch shell {
			case shellBash:
				return root.GenBashCompletion(out)
			case shellZsh:
				return root.GenZshCompletion(out)
			case shellFish:
				return root.GenFishCompletion(out, true)
			default:
				return root.GenPowerShellCompletion(out)
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
