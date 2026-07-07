package gitdiff

import (
	"github.com/jongio/grut/internal/panelreg"
	"github.com/jongio/grut/internal/panels"
)

func init() {
	panelreg.Register("gitdiff", func(deps panelreg.Deps) panels.Panel {
		d := New(deps.GitClient, deps.Theme)
		if deps.Config != nil {
			d.SetWordHighlight(deps.Config.Git.DiffWordHighlight)
		}
		return deps.ApplyActionsCfg(d)
	})
}
