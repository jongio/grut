package preview

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/jongio/grut/internal/github"
	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
)

// createGist creates a secret github gist from the previewed file and copies
// the gist URL to the clipboard. It is a no-op when the preview is not showing
// an on-disk file (for example GitHub content or an empty view).
func (p *Preview) createGist() (panels.Panel, tea.Cmd) {
	if p.ghMode || p.filePath == "" {
		return p, nil
	}
	path := p.filePath
	return p, func() tea.Msg {
		ctx := context.Background()
		link, err := github.CreateGist(ctx, path)
		if err != nil {
			return notify.ShowToastMsg{Message: "Gist failed: " + err.Error(), Level: notify.Error}
		}
		if cerr := panels.CopyToClipboard(ctx, link); cerr != nil {
			return notify.ShowToastMsg{Message: "Created gist (copy failed): " + link, Level: notify.Warn}
		}
		return notify.ShowToastMsg{Message: "Created secret gist (URL copied)", Level: notify.Success}
	}
}
