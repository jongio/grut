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
			First:     &LeafNode{Panel: slotFiletree},
			Second: &SplitNode{
				Direction: Vertical,
				Ratio:     0.385, // 25/(25+20+20) ≈ 0.385
				First:     &LeafNode{Panel: slotGitinfo},
				Second: &SplitNode{
					Direction: Vertical,
					Ratio:     0.5, // 20/(20+20) = 0.5
					First:     &LeafNode{Panel: slotGithub},
					Second:    &LeafNode{Panel: slotCommits},
				},
			},
		},
		Second: &LeafNode{Panel: slotPreview},
	}
	return Preset{
		Name:   layoutExplorer,
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
		First:     &LeafNode{Panel: slotFiletree},
		Second:    &LeafNode{Panel: slotPreview},
	}
	return Preset{
		Name:   layoutGit,
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
		First:     &LeafNode{Panel: slotFiletree},
		Second: &SplitNode{
			Direction: Horizontal,
			Ratio:     0.625, // 50/(50+30) ≈ 0.625
			First:     &LeafNode{Panel: slotReview},
			Second:    &LeafNode{Panel: slotContext},
		},
	}
	return Preset{
		Name:   layoutReview,
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
		First:     &LeafNode{Panel: slotFiletree},
		Second: &SplitNode{
			Direction: Horizontal,
			Ratio:     0.5,
			First:     &LeafNode{Panel: slotTerminal},
			Second:    &LeafNode{Panel: slotAgents},
		},
	}
	return Preset{
		Name:   layoutAgent,
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
		First:     &LeafNode{Panel: slotFiletree},
		Second: &SplitNode{
			Direction: Horizontal,
			Ratio:     0.294, // 25/(25+35+25) ≈ 0.294
			First:     &LeafNode{Panel: slotGitstatus},
			Second: &SplitNode{
				Direction: Horizontal,
				Ratio:     0.583, // 35/(35+25) ≈ 0.583
				First:     &LeafNode{Panel: slotPreview},
				Second:    &LeafNode{Panel: slotTerminal},
			},
		},
	}
	return Preset{
		Name:   layoutFull,
		Tree:   tree,
		Panels: tree.PanelNames(),
	}
}

// Presets returns all built-in layout presets keyed by name.
func Presets() map[string]Preset {
	return map[string]Preset{
		layoutExplorer: ExplorerPreset(),
		layoutGit:      GitPreset(),
		layoutReview:   ReviewPreset(),
		layoutAgent:    AgentPreset(),
		layoutFull:     FullPreset(),
	}
}
