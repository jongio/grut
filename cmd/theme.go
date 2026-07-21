package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/jongio/grut/internal/theme"
	"github.com/spf13/cobra"
)

type themeListFunc func() []string

func newThemeCmd() *cobra.Command {
	themeCmd := &cobra.Command{
		Use:   "theme",
		Short: "Inspect grut themes",
	}
	themeCmd.AddCommand(newThemeListCmd(theme.ListThemes))
	return themeCmd
}

func newThemeListCmd(list themeListFunc) *cobra.Command {
	var asJSON bool

	listCmd := &cobra.Command{
		Use:   cmdList,
		Short: "List available themes",
		RunE: func(cmd *cobra.Command, args []string) error {
			themes := list()
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(themes)
			}
			for _, name := range themes {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), name)
			}
			return nil
		},
	}
	listCmd.Flags().BoolVar(&asJSON, "json", false, "Print theme names as JSON")
	return listCmd
}
