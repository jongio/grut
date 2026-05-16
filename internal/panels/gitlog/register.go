package gitlog

import (
	"github.com/jongio/grut/internal/panelreg"
	"github.com/jongio/grut/internal/panels"
)

func init() {
	panelreg.Register("gitlog", func(deps panelreg.Deps) panels.Panel {
		client, _, err := deps.NewGitClient()
		if err != nil {
			return deps.Placeholder("gitlog")
		}
		return deps.ApplyActionsCfg(New(client, deps.Config.Git, deps.Theme))
	})
}
