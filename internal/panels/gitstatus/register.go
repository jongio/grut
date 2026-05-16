package gitstatus

import (
	"github.com/jongio/grut/internal/panelreg"
	"github.com/jongio/grut/internal/panels"
)

func init() {
	panelreg.Register("gitstatus", func(deps panelreg.Deps) panels.Panel {
		client, _, err := deps.NewGitClient()
		if err != nil {
			return deps.Placeholder("gitstatus")
		}
		return deps.ApplyActionsCfg(New(client, deps.Theme))
	})
}
