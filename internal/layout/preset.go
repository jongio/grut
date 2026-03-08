package layout

// Preset is a named layout configuration consisting of a layout tree
// and the list of panel names it requires.
type Preset struct {
	Name   string
	Tree   Node
	Panels []string
}

// ExplorerPreset returns the default "explorer" layout:
// Left column (30%): filetree (35%) on top, gitinfo (25%), github (20%), commits (20%) on bottom.
// Right column (70%): preview (100%).
func ExplorerPreset() Preset {
	tree := &SplitNode{
		Direction: Horizontal,
		Ratio:     0.3,
		First: &SplitNode{
			Direction: Vertical,
			Ratio:     0.35,
			First:     &LeafNode{Panel: "filetree"},
			Second: &SplitNode{
				Direction: Vertical,
				Ratio:     0.385, // 25/(25+20+20) ≈ 0.385
				First:     &LeafNode{Panel: "gitinfo"},
				Second: &SplitNode{
					Direction: Vertical,
					Ratio:     0.5, // 20/(20+20) = 0.5
					First:     &LeafNode{Panel: "github"},
					Second:    &LeafNode{Panel: "commits"},
				},
			},
		},
		Second: &LeafNode{Panel: "preview"},
	}
	return Preset{
		Name:   "explorer",
		Tree:   tree,
		Panels: tree.PanelNames(),
	}
}

// GitPreset returns the "git" layout:
// filetree (30%) | preview (70%)
// The filetree already shows git status indicators on each file,
// so a separate gitstatus panel is not needed.
func GitPreset() Preset {
	tree := &SplitNode{
		Direction: Horizontal,
		Ratio:     0.3,
		First:     &LeafNode{Panel: "filetree"},
		Second:    &LeafNode{Panel: "preview"},
	}
	return Preset{
		Name:   "git",
		Tree:   tree,
		Panels: tree.PanelNames(),
	}
}

// ReviewPreset returns the "review" layout:
// filetree (20%) | review (50%) | context (30%)
func ReviewPreset() Preset {
	tree := &SplitNode{
		Direction: Horizontal,
		Ratio:     0.2,
		First:     &LeafNode{Panel: "filetree"},
		Second: &SplitNode{
			Direction: Horizontal,
			Ratio:     0.625, // 50/(50+30) ≈ 0.625
			First:     &LeafNode{Panel: "review"},
			Second:    &LeafNode{Panel: "context"},
		},
	}
	return Preset{
		Name:   "review",
		Tree:   tree,
		Panels: tree.PanelNames(),
	}
}

// AgentPreset returns the "agent" layout:
// filetree (20%) | terminal (40%) | agents (40%)
func AgentPreset() Preset {
	tree := &SplitNode{
		Direction: Horizontal,
		Ratio:     0.2,
		First:     &LeafNode{Panel: "filetree"},
		Second: &SplitNode{
			Direction: Horizontal,
			Ratio:     0.5,
			First:     &LeafNode{Panel: "terminal"},
			Second:    &LeafNode{Panel: "agents"},
		},
	}
	return Preset{
		Name:   "agent",
		Tree:   tree,
		Panels: tree.PanelNames(),
	}
}

// FullPreset returns the "full" layout:
// filetree (15%) | gitstatus (25%) | preview (35%) | terminal (25%)
func FullPreset() Preset {
	tree := &SplitNode{
		Direction: Horizontal,
		Ratio:     0.15,
		First:     &LeafNode{Panel: "filetree"},
		Second: &SplitNode{
			Direction: Horizontal,
			Ratio:     0.294, // 25/(25+35+25) ≈ 0.294
			First:     &LeafNode{Panel: "gitstatus"},
			Second: &SplitNode{
				Direction: Horizontal,
				Ratio:     0.583, // 35/(35+25) ≈ 0.583
				First:     &LeafNode{Panel: "preview"},
				Second:    &LeafNode{Panel: "terminal"},
			},
		},
	}
	return Preset{
		Name:   "full",
		Tree:   tree,
		Panels: tree.PanelNames(),
	}
}

// Presets returns all built-in layout presets keyed by name.
func Presets() map[string]Preset {
	return map[string]Preset{
		"explorer": ExplorerPreset(),
		"git":      GitPreset(),
		"review":   ReviewPreset(),
		"agent":    AgentPreset(),
		"full":     FullPreset(),
	}
}
