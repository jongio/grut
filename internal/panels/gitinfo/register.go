package gitinfo

import (
	"github.com/jongio/grut/internal/panelreg"
	"github.com/jongio/grut/internal/panels"
)

func init() {
	panelreg.Register("gitinfo", func(deps panelreg.Deps) panels.Panel {
		client, cwd, err := deps.NewGitClient()
		if err != nil {
			return deps.Placeholder("gitinfo")
		}
		return New(client, deps.Config.Git, deps.Config.GitHub, deps.Config.Actions, cwd, deps.Config.FileTree.IconMode, deps.Theme)
	})

	panelreg.Register("github", func(deps panelreg.Deps) panels.Panel {
		client, cwd, err := deps.NewGitClient()
		if err != nil {
			return deps.Placeholder("github")
		}
		return NewGitHub(client, deps.Config.Git, deps.Config.GitHub, deps.Config.Actions, cwd, deps.Config.FileTree.IconMode, deps.Theme)
	})
}
