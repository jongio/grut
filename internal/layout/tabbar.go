package layout

import "strings"

const (
	// maxTabNameLen is the maximum display length for a tab name before truncation.
	maxTabNameLen = 20

	// tabBarActiveMarker is appended to the active tab's name.
	tabBarActiveMarker = "*"
)

// RenderTabBar renders a one-line tab bar showing open tabs with their preset
// number, plus hints for unopened presets. The active tab is shown in
// uppercase. The result is padded or truncated to fit the given width.
//
// v1: hidden — returns "" so the tab bar takes no vertical space.
// The full rendering logic is preserved below for v2 multi-tab.
func RenderTabBar(tabs []Tab, activeIdx, width int) string {
	if SingleTabMode {
		return "" // v1: single-tab mode, tab bar hidden
	}

	if width <= 0 {
		return ""
	}

	type presetInfo struct {
		key  string
		name string
	}
	allPresets := []presetInfo{
		{"1", layoutExplorer},
		{"2", layoutGit},
		{"3", layoutReview},
		{"4", layoutAgent},
		{"5", layoutFull},
	}

	openNames := make(map[string]bool)
	for _, tab := range tabs {
		openNames[tab.Name] = true
	}

	var b strings.Builder

	// Render open tabs
	for i, tab := range tabs {
		if i > 0 {
			b.WriteString("│")
		}

		name := tab.Name
		if len(name) > maxTabNameLen {
			name = name[:maxTabNameLen-1] + "…"
		}

		// Find preset key for this tab
		key := ""
		for _, p := range allPresets {
			if p.name == tab.Name {
				key = p.key
				break
			}
		}

		if i == activeIdx {
			b.WriteString(" ")
			if key != "" {
				b.WriteString(key + ":")
			}
			b.WriteString(strings.ToUpper(name))
			b.WriteString(tabBarActiveMarker)
			b.WriteString(" ")
		} else {
			b.WriteString(" ")
			if key != "" {
				b.WriteString(key + ":")
			}
			b.WriteString(name)
			b.WriteString(" ")
		}
	}

	// Show hints for tabs not yet opened (dimmed)
	hintParts := []string{}
	for _, p := range allPresets {
		if !openNames[p.name] {
			hintParts = append(hintParts, p.key+":"+p.name)
		}
	}
	if len(hintParts) > 0 {
		b.WriteString("  ")
		b.WriteString(strings.Join(hintParts, " "))
	}

	bar := b.String()

	// Truncate to width.
	runes := []rune(bar)
	if len(runes) > width {
		runes = runes[:width]
		bar = string(runes)
	}

	// Pad to full width.
	if pad := width - len(runes); pad > 0 {
		bar += strings.Repeat(" ", pad)
	}

	return bar
}
