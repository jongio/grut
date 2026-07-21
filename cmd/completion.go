package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

const shellPowerShell = "powershell"

func newCompletionCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "completion [bash|zsh|fish|powershell]",
		Short:     "Generate shell completion scripts",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"bash", "zsh", "fish", shellPowerShell},
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
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
}
