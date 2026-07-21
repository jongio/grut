package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/update"
	"github.com/spf13/cobra"
)

type versionInfo struct {
	Version string `json:"version"`
}

// newVersionCmd creates the version command.
func newVersionCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   cmdVersion,
		Short: "Print the version of grut",
		Run: func(cmd *cobra.Command, args []string) {
			if jsonOutput {
				_ = json.NewEncoder(cmd.OutOrStdout()).Encode(versionInfo{Version: config.AppVersion})
				return
			}

			fmt.Fprintln(cmd.OutOrStdout(), config.AppVersion)
			// Show update notification inline.
			if info := update.CheckForUpdate(config.AppVersion); info != nil {
				fmt.Fprintf(cmd.ErrOrStderr(),
					"\nA new version of grut is available: v%s → v%s\nRun \"grut update\" to install it.\n",
					info.CurrentVersion, info.LatestVersion)
			}
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Print version information as JSON")
	return cmd
}
