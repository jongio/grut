package commits

import (
	"github.com/jongio/grut/internal/panelreg"
	"github.com/jongio/grut/internal/panels"
)

func init() {
	panelreg.Register("commits", func(deps panelreg.Deps) panels.Panel {
		client, _, err := deps.NewGitClient()
		if err != nil {
			return deps.Placeholder("commits")
		}
		return deps.ApplyActionsCfg(New(client, deps.Theme))
	})
}
