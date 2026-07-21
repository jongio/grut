package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jongio/grut/internal/keybindings"
	"github.com/spf13/cobra"
)

func newKeysCmd() *cobra.Command {
	var (
		filter string
		asJSON bool
	)

	keysCmd := &cobra.Command{
		Use:   "keys",
		Short: "Print keybindings",
		Long:  "Print the built-in keybinding reference without starting the TUI.",
		RunE: func(cmd *cobra.Command, args []string) error {
			sections := filterKeybindingSections(keybindings.Sections(), filter)
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(sections)
			}
			printKeybindingSections(cmd, sections, filter)
			return nil
		},
	}

	keysCmd.Flags().StringVar(&filter, "filter", "", "Only show sections, keys, or actions matching this text")
	keysCmd.Flags().BoolVar(&asJSON, "json", false, "Print keybindings as JSON")
	return keysCmd
}

func filterKeybindingSections(sections []keybindings.Section, filter string) []keybindings.Section {
	query := strings.TrimSpace(strings.ToLower(filter))
	if query == "" {
		return sections
	}

	var out []keybindings.Section
	for _, section := range sections {
		if keybindingSectionMatches(section, query) {
			out = append(out, section)
			continue
		}

		filtered := section
		filtered.Bindings = nil
		for _, binding := range section.Bindings {
			if strings.Contains(strings.ToLower(binding.Key), query) ||
				strings.Contains(strings.ToLower(binding.Action), query) {
				filtered.Bindings = append(filtered.Bindings, binding)
			}
		}
		if len(filtered.Bindings) > 0 {
			out = append(out, filtered)
		}
	}
	return out
}

func keybindingSectionMatches(section keybindings.Section, query string) bool {
	return strings.Contains(strings.ToLower(section.ID), query) ||
		strings.Contains(strings.ToLower(section.Title), query) ||
		strings.Contains(strings.ToLower(section.Description), query)
}

func printKeybindingSections(cmd *cobra.Command, sections []keybindings.Section, filter string) {
	w := cmd.OutOrStdout()
	if len(sections) == 0 {
		if strings.TrimSpace(filter) == "" {
			_, _ = fmt.Fprintln(w, "No keybindings available.")
			return
		}
		_, _ = fmt.Fprintf(w, "No keybindings match %q.\n", filter)
		return
	}

	for i, section := range sections {
		if i > 0 {
			_, _ = fmt.Fprintln(w)
		}
		_, _ = fmt.Fprintf(w, "%s\n", section.Title)
		for _, binding := range section.Bindings {
			_, _ = fmt.Fprintf(w, "  %-14s %s\n", binding.Key, binding.Action)
		}
	}
}
