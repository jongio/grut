package terminal

import (
	"github.com/jongio/grut/internal/panelreg"
	"github.com/jongio/grut/internal/panels"
	term "github.com/jongio/grut/internal/terminal"
)

func init() {
	panelreg.Register("terminal", func(deps panelreg.Deps) panels.Panel {
		shell := deps.Config.Terminal.Shell
		if shell == "" {
			shell = term.DefaultShell()
		}
		runner, err := term.New(deps.Ctx, shell, deps.Config.Terminal.Scrollback)
		if err != nil {
			return deps.Placeholder("terminal")
		}
		return New(deps.Config.Terminal, runner, shell, deps.Theme)
	})
}
