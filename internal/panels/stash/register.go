package stash

import (
	"github.com/jongio/grut/internal/panelreg"
	"github.com/jongio/grut/internal/panels"
)

func init() {
	panelreg.Register("stash", func(deps panelreg.Deps) panels.Panel {
		client, _, err := deps.NewGitClient()
		if err != nil {
			return deps.Placeholder("stash")
		}
		return deps.ApplyActionsCfg(New(client, deps.Theme))
	})
}
