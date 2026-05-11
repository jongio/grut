// Package keybindings embeds and exposes the canonical keybinding reference
// data for grut. The JSON file in this package is the single source of truth
// consumed by the help overlay and used to generate docs/keybindings.md.
package keybindings

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed keybindings.json
var raw []byte

// data is the parsed JSON, initialised at package init time.
var data keybindingsData

func init() {
	if err := json.Unmarshal(raw, &data); err != nil {
		panic(fmt.Sprintf("keybindings: failed to parse embedded JSON: %v", err))
	}
}

// ---------------------------------------------------------------------------
// JSON types
// ---------------------------------------------------------------------------

type keybindingsData struct {
	Version     string    `json:"version"`
	Description string    `json:"description"`
	Sections    []Section `json:"sections"`
}

// Section is a named group of keybindings (e.g. "Global", "File Tree").
type Section struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Bindings    []Binding `json:"bindings"`
	Note        string    `json:"note,omitempty"`
}

// Binding is a single key→action documentation entry.
type Binding struct {
	Key    string `json:"key"`
	Action string `json:"action"`
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// Sections returns the ordered list of keybinding sections parsed from the
// embedded JSON. The returned slice is a shallow copy; callers may replace
// elements but must not mutate individual Binding slice contents in place.
func Sections() []Section {
	out := make([]Section, len(data.Sections))
	copy(out, data.Sections)
	return out
}

// ---------------------------------------------------------------------------
// Markdown generation
// ---------------------------------------------------------------------------

// GenerateMarkdown produces the full contents of docs/keybindings.md from
// the embedded JSON data.
func GenerateMarkdown() string {
	var b strings.Builder

	b.WriteString("# Keybindings Reference\n\n")
	b.WriteString("grut supports three keybinding schemes: **default**, **vim**, and **classic**. ")
	b.WriteString("Set the scheme in `~/.config/grut/config.toml`:\n\n")
	b.WriteString("```toml\n[general]\n")
	b.WriteString("keybinding_scheme = \"default\"  # \"default\", \"vim\", \"classic\", or path to custom .toml\n")
	b.WriteString("```\n\n")
	b.WriteString("The bindings below document the **default** scheme. All schemes share the same ")
	b.WriteString("global and number-key bindings. This file is generated from ")
	b.WriteString("`internal/keybindings/keybindings.json` — do not edit by hand.\n\n")

	for _, sec := range data.Sections {
		b.WriteString("---\n\n")
		b.WriteString("## " + sec.Title + "\n\n")
		if sec.Description != "" {
			b.WriteString(sec.Description + "\n\n")
		}
		b.WriteString("| Key | Action |\n")
		b.WriteString("|-----|--------|\n")
		for _, bind := range sec.Bindings {
			b.WriteString("| `" + bind.Key + "` | " + bind.Action + " |\n")
		}
		if sec.Note != "" {
			b.WriteString("\n" + sec.Note + "\n")
		}
		b.WriteString("\n")
	}

	return b.String()
}
