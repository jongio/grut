package fuzzyfinder

import (
	"github.com/jongio/grut/internal/panelreg"
	"github.com/jongio/grut/internal/panels"
)

func init() {
	panelreg.Register("fuzzyfinder", func(deps panelreg.Deps) panels.Panel {
		return New(deps.Theme)
	})
}
