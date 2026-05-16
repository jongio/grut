package worktrees

import (
	"github.com/jongio/grut/internal/panelreg"
	"github.com/jongio/grut/internal/panels"
)

func init() {
	panelreg.Register("worktrees", func(deps panelreg.Deps) panels.Panel {
		client, cwd, err := deps.NewGitClient()
		if err != nil {
			return deps.Placeholder("worktrees")
		}
		return New(client, deps.Config.Git, cwd, deps.Theme)
	})
}
