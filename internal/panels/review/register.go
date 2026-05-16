package review

import (
	"github.com/jongio/grut/internal/panelreg"
	"github.com/jongio/grut/internal/panels"
)

func init() {
	panelreg.Register("review", func(deps panelreg.Deps) panels.Panel {
		return deps.ApplyActionsCfg(New(deps.GitClient, deps.Theme))
	})
}
