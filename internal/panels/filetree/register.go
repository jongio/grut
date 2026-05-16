package filetree

import (
	"github.com/jongio/grut/internal/panelreg"
	"github.com/jongio/grut/internal/panels"
)

func init() {
	panelreg.Register("filetree", func(deps panelreg.Deps) panels.Panel {
		cwd := deps.Cwd()
		ft := New(deps.Config.FileTree, cwd, deps.Theme)
		if deps.GitClient != nil {
			ft.SetGitClient(deps.GitClient)
		}
		ft.SetBaseBranch(deps.Config.Git.DefaultBranch)
		return deps.ApplyActionsCfg(ft)
	})
}
