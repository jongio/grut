package conflicts

import (
	"github.com/jongio/grut/internal/panelreg"
	"github.com/jongio/grut/internal/panels"
)

func init() {
	panelreg.Register("conflicts", func(deps panelreg.Deps) panels.Panel {
		client, _, err := deps.NewGitClient()
		if err != nil {
			return deps.Placeholder("conflicts")
		}
		return deps.ApplyActionsCfg(New(client, deps.Theme))
	})
}
