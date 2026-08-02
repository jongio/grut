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
		filter  string
		section string
		asJSON  bool
	)

	keysCmd := &cobra.Command{
		Use:   "keys",
		Short: "Print keybindings",
		Long:  "Print the built-in keybinding reference without starting the TUI.",
		RunE: func(cmd *cobra.Command, args []string) error {
			sections := filterKeybindingSectionsByID(keybindings.Sections(), section)
			sections = filterKeybindingSections(sections, filter)
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
	keysCmd.Flags().StringVar(&section, "section", "", "Only show the section with this id")
	keysCmd.Flags().BoolVar(&asJSON, "json", false, "Print keybindings as JSON")
	return keysCmd
}

func filterKeybindingSectionsByID(sections []keybindings.Section, section string) []keybindings.Section {
	query := strings.TrimSpace(strings.ToLower(section))
	if query == "" {
		return sections
	}
	for _, item := range sections {
		if strings.ToLower(item.ID) == query {
			return []keybindings.Section{item}
		}
	}
	return []keybindings.Section{}
}

func filterKeybindingSections(sections []keybindings.Section, filter string) []keybindings.Section {
	if textFilterMatches(filter) {
		return sections
	}

	var out []keybindings.Section
	for _, section := range sections {
		if keybindingSectionMatches(section, filter) {
			out = append(out, section)
			continue
		}

		filtered := section
		filtered.Bindings = nil
		for _, binding := range section.Bindings {
			if textFilterMatches(filter, binding.Key, binding.Action) {
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
	return textFilterMatches(query, section.ID, section.Title, section.Description)
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
