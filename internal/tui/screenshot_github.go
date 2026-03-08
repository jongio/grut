//go:build screenshots

package tui

// injectGitHubDemoData injects realistic fake GitHub data into any
// gitinfo-type panels for screenshot captures. It checks both the
// "gitinfo" and "github" panel names so that the split-panel layout
// gets demo data in both panes.
func injectGitHubDemoData(m *Model) {
	type demoInjector interface {
		InjectDemoGitHubData()
	}
	allPanels := m.engine.Panels()
	for _, name := range []string{"gitinfo", "github"} {
		p, ok := allPanels[name]
		if !ok {
			continue
		}
		if di, ok := p.(demoInjector); ok {
			di.InjectDemoGitHubData()
		}
	}
}
