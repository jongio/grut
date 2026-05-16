package extensions

import (
	"github.com/jongio/grut/internal/extension"
	"github.com/jongio/grut/internal/panelreg"
	"github.com/jongio/grut/internal/panels"
)

func init() {
	panelreg.Register("extensions", func(deps panelreg.Deps) panels.Panel {
		installDir := deps.Config.Extensions.InstallDir
		if installDir == "" {
			installDir = "extensions"
		}
		mgr := extension.NewManager(installDir)
		if err := mgr.LoadAll(); err != nil {
			return deps.Placeholder("extensions")
		}
		return deps.ApplyActionsCfg(New(mgr, deps.Theme))
	})
}
