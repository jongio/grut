package status

import (
	"context"

	"github.com/jongio/grut/internal/panelreg"
	"github.com/jongio/grut/internal/panels"
)

func init() {
	panelreg.Register("status", func(deps panelreg.Deps) panels.Panel {
		client, _, err := deps.NewGitClient()
		if err != nil {
			return New(nil, deps.Theme)
		}
		if ok, err := client.IsRepo(context.Background()); err != nil || !ok {
			return New(nil, deps.Theme)
		}
		return New(client, deps.Theme)
	})
}
