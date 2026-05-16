package branches

import (
	"github.com/jongio/grut/internal/panelreg"
	"github.com/jongio/grut/internal/panels"
)

func init() {
	panelreg.Register("branches", func(deps panelreg.Deps) panels.Panel {
		client, cwd, err := deps.NewGitClient()
		if err != nil {
			return deps.Placeholder("branches")
		}
		return New(client, deps.Config.Git, cwd, deps.Theme)
	})
}
