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
	themeCmd.AddCommand(newThemeShowCmd(theme.Load))
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

type themeLoadFunc func(string) (*theme.Theme, error)

type themeShowReport struct {
	Name    string       `json:"name"`
	Variant string       `json:"variant"`
	Mode    string       `json:"mode"`
	Colors  theme.Colors `json:"colors"`
}

func newThemeShowCmd(load themeLoadFunc) *cobra.Command {
	var asJSON bool
	showCmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Show resolved theme details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resolved, err := load(args[0])
			if err != nil {
				return err
			}
			report := themeShowReport{
				Name:    resolved.Name,
				Variant: resolved.Variant,
				Mode:    string(resolved.Mode),
				Colors:  resolved.Colors,
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(report)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Name:    %s\n", report.Name)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Variant: %s\n", report.Variant)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Mode:    %s\n", report.Mode)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Colors:  %d slots\n", themeColorSlotCount(report.Colors))
			return nil
		},
	}
	showCmd.Flags().BoolVar(&asJSON, "json", false, "Print resolved theme details as JSON")
	return showCmd
}

func themeColorSlotCount(colors theme.Colors) int {
	data, err := json.Marshal(colors)
	if err != nil {
		return 0
	}
	var slots map[string]string
	if err := json.Unmarshal(data, &slots); err != nil {
		return 0
	}
	return len(slots)
}
