package cmd

import (
	"fmt"

	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/update"
	"github.com/spf13/cobra"
)

// updateCmdName is the name of the update subcommand.
const updateCmdName = "update"

// newUpdateCmd creates the update command.
func newUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   updateCmdName,
		Short: "Update grut to the latest release",
		Long: `Downloads and installs the latest release of grut from GitHub.

Verifies the download using SHA-256 checksums before replacing the
current binary. Development builds cannot be updated — install a
release build first.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := update.RunUpdate(cmd.Context(), config.AppVersion); err != nil {
				return fmt.Errorf("update: %w", err)
			}
			return nil
		},
	}
}
