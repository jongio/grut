package preview

import (
	"github.com/jongio/grut/internal/panelreg"
	"github.com/jongio/grut/internal/panels"
)

func init() {
	panelreg.Register("preview", func(deps panelreg.Deps) panels.Panel {
		p := New(deps.Config.Preview, deps.Config.Editor, deps.Theme)
		if deps.GitClient != nil {
			p.SetGitClient(deps.GitClient)
		}
		return p
	})
}
