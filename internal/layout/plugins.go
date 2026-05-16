package layout

// Blank imports trigger init() in each panel package, which self-registers
// the panel's builder into the panelreg global registry. This is the single
// central location for all panel plugin imports.
import (
	_ "github.com/jongio/grut/internal/panels/agents"
	_ "github.com/jongio/grut/internal/panels/branches"
	_ "github.com/jongio/grut/internal/panels/commits"
	_ "github.com/jongio/grut/internal/panels/conflicts"
	_ "github.com/jongio/grut/internal/panels/context"
	_ "github.com/jongio/grut/internal/panels/extensions"
	_ "github.com/jongio/grut/internal/panels/filetree"
	_ "github.com/jongio/grut/internal/panels/fuzzyfinder"
	_ "github.com/jongio/grut/internal/panels/gitdiff"
	_ "github.com/jongio/grut/internal/panels/gitinfo"
	_ "github.com/jongio/grut/internal/panels/gitlog"
	_ "github.com/jongio/grut/internal/panels/gitstatus"
	_ "github.com/jongio/grut/internal/panels/preview"
	_ "github.com/jongio/grut/internal/panels/review"
	_ "github.com/jongio/grut/internal/panels/stash"
	_ "github.com/jongio/grut/internal/panels/terminal"
	_ "github.com/jongio/grut/internal/panels/worktrees"
)
