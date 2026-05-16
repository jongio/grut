package context

import (
	ctxbuilder "github.com/jongio/grut/internal/context"
	"github.com/jongio/grut/internal/panelreg"
	"github.com/jongio/grut/internal/panels"
)

func init() {
	panelreg.Register("context", func(deps panelreg.Deps) panels.Panel {
		cwd := deps.Cwd()
		builder, err := ctxbuilder.NewBuilder(cwd)
		if err != nil {
			return deps.Placeholder("context")
		}
		return deps.ApplyActionsCfg(New(builder, deps.Theme))
	})
}
