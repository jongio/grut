package cmd

import (
	"fmt"

	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/update"
	"github.com/spf13/cobra"
)

// newVersionCmd creates the version command.
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   cmdVersion,
		Short: "Print the version of grut",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(config.AppVersion)
			// Show update notification inline.
			if info := update.CheckForUpdate(config.AppVersion); info != nil {
				fmt.Fprintf(cmd.ErrOrStderr(),
					"\nA new version of grut is available: v%s → v%s\nRun \"grut update\" to install it.\n",
					info.CurrentVersion, info.LatestVersion)
			}
		},
	}
}
