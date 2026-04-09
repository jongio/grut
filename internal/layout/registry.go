package layout

import (
	"fmt"
	"os"
	"sync"

	"github.com/jongio/grut/internal/config"
	ctxbuilder "github.com/jongio/grut/internal/context"
	"github.com/jongio/grut/internal/extension"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/mcp"
	"github.com/jongio/grut/internal/panels"
	"github.com/jongio/grut/internal/panels/agents"
	"github.com/jongio/grut/internal/panels/branches"
	"github.com/jongio/grut/internal/panels/commits"
	"github.com/jongio/grut/internal/panels/conflicts"
	ctxpanel "github.com/jongio/grut/internal/panels/context"
	extpanel "github.com/jongio/grut/internal/panels/extensions"
	"github.com/jongio/grut/internal/panels/filetree"
	"github.com/jongio/grut/internal/panels/fuzzyfinder"
	"github.com/jongio/grut/internal/panels/gitdiff"
	"github.com/jongio/grut/internal/panels/gitinfo"
	"github.com/jongio/grut/internal/panels/gitlog"
	"github.com/jongio/grut/internal/panels/gitstatus"
	"github.com/jongio/grut/internal/panels/preview"
	"github.com/jongio/grut/internal/panels/review"
	"github.com/jongio/grut/internal/panels/stash"
	termpanel "github.com/jongio/grut/internal/panels/terminal"
	"github.com/jongio/grut/internal/panels/worktrees"
	"github.com/jongio/grut/internal/terminal"
	"github.com/jongio/grut/internal/theme"
)

// PanelFactory is a constructor function for creating panels by name.
type PanelFactory func() panels.Panel

// Registry maps panel names to their factory functions, enabling dynamic
// panel creation by the layout engine.
type Registry struct {
	factories map[string]PanelFactory
	mu        sync.RWMutex
}

// NewRegistry creates a new empty panel registry.
func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]PanelFactory),
	}
}

// Register adds a panel factory under the given name. If a factory with
// that name already exists, it is replaced.
func (r *Registry) Register(name string, factory PanelFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[name] = factory
}

// Create instantiates a panel by name using its registered factory.
// Returns an error if the name is not registered.
func (r *Registry) Create(name string) (panels.Panel, error) {
	r.mu.RLock()
	factory, ok := r.factories[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown panel: %s", name)
	}
	return factory(), nil
}

// Has returns true if a factory is registered for the given name.
func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.factories[name]
	return ok
}

// Names returns all registered panel names in no particular order.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	return names
}

// RegisterDefaults registers the built-in panels.
// Panels with real implementations use their concrete constructors;
// those still under development use placeholders.
// The cfg parameter provides the already-loaded configuration, avoiding
// redundant disk reads on every panel creation. The gc parameter provides
// the git client for git-aware panels; if nil, git panels are not registered.
// The th parameter provides the theme for styled panels; if nil, panels
// use fallback colors.
func RegisterDefaults(r *Registry, cfg *config.Config, gc git.GitClient, th *theme.Theme) {
	// Panels still using placeholders
	for _, name := range []string{"status"} {
		r.Register(name, func() panels.Panel {
			return panels.NewPlaceholder(name, th)
		})
	}
	// setActionsCfg injects the actions configuration into panels that
	// support it via the optional SetActionsCfg method.
	setActionsCfg := func(p panels.Panel) panels.Panel {
		if ac, ok := p.(interface{ SetActionsCfg(config.ActionsConfig) }); ok {
			ac.SetActionsCfg(cfg.Actions)
		}
		return p
	}
	// File tree panel — real implementation
	r.Register("filetree", func() panels.Panel {
		cwd, err := os.Getwd()
		if err != nil {
			cwd = "."
		}
		ft := filetree.New(cfg.FileTree, cwd, th)
		if gc != nil {
			ft.SetGitClient(gc)
		}
		return setActionsCfg(ft)
	})
	// Preview panel — real implementation
	r.Register("preview", func() panels.Panel {
		p := preview.New(cfg.Preview, th)
		if gc != nil {
			p.SetGitClient(gc)
		}
		return p
	})
	// Fuzzy finder panel — real implementation (used as overlay by app model)
	r.Register("fuzzyfinder", func() panels.Panel {
		return fuzzyfinder.New(th)
	})
	// Git status panel — real implementation
	r.Register("gitstatus", func() panels.Panel {
		cwd, err := os.Getwd()
		if err != nil {
			cwd = "."
		}
		client, err := git.NewClient(cwd)
		if err != nil {
			// Fall back to placeholder if git is unavailable.
			return panels.NewPlaceholder("gitstatus", th)
		}
		return setActionsCfg(gitstatus.New(client, th))
	})
	// Branch panel — real implementation
	r.Register("branches", func() panels.Panel {
		cwd, err := os.Getwd()
		if err != nil {
			cwd = "."
		}
		client, err := git.NewClient(cwd)
		if err != nil {
			// Fall back to placeholder if git is unavailable.
			return panels.NewPlaceholder("branches", th)
		}
		return branches.New(client, cfg.Git, cwd, th)
	})
	// Git diff panel — real implementation
	r.Register("gitdiff", func() panels.Panel {
		return setActionsCfg(gitdiff.New(gc, th))
	})
	// Git log panel — real implementation
	r.Register("gitlog", func() panels.Panel {
		cwd, err := os.Getwd()
		if err != nil {
			cwd = "."
		}
		client, err := git.NewClient(cwd)
		if err != nil {
			return panels.NewPlaceholder("gitlog", th)
		}
		return setActionsCfg(gitlog.New(client, cfg.Git, th))
	})
	// Commits panel — selection-driven commit history
	r.Register("commits", func() panels.Panel {
		cwd, err := os.Getwd()
		if err != nil {
			cwd = "."
		}
		client, err := git.NewClient(cwd)
		if err != nil {
			return panels.NewPlaceholder("commits", th)
		}
		return setActionsCfg(commits.New(client, th))
	})
	// Conflict resolution panel — real implementation
	r.Register("conflicts", func() panels.Panel {
		cwd, err := os.Getwd()
		if err != nil {
			cwd = "."
		}
		client, err := git.NewClient(cwd)
		if err != nil {
			return panels.NewPlaceholder("conflicts", th)
		}
		return setActionsCfg(conflicts.New(client, th))
	})
	// Worktree management panel — real implementation
	r.Register("worktrees", func() panels.Panel {
		cwd, err := os.Getwd()
		if err != nil {
			cwd = "."
		}
		client, err := git.NewClient(cwd)
		if err != nil {
			return panels.NewPlaceholder("worktrees", th)
		}
		return worktrees.New(client, cfg.Git, cwd, th)
	})
	// Stash management panel — real implementation
	r.Register("stash", func() panels.Panel {
		cwd, err := os.Getwd()
		if err != nil {
			cwd = "."
		}
		client, err := git.NewClient(cwd)
		if err != nil {
			return panels.NewPlaceholder("stash", th)
		}
		return setActionsCfg(stash.New(client, th))
	})
	// Git info panel — git tabs only (branches, worktrees, remotes, stash, tags, reflog)
	r.Register("gitinfo", func() panels.Panel {
		cwd, err := os.Getwd()
		if err != nil {
			cwd = "."
		}
		client, err := git.NewClient(cwd)
		if err != nil {
			return panels.NewPlaceholder("gitinfo", th)
		}
		return gitinfo.New(client, cfg.Git, cfg.GitHub, cfg.Actions, cwd, cfg.FileTree.IconMode, th)
	})
	// GitHub panel — GitHub tabs only (issues, PRs, actions, workflows, releases)
	r.Register("github", func() panels.Panel {
		cwd, err := os.Getwd()
		if err != nil {
			cwd = "."
		}
		client, err := git.NewClient(cwd)
		if err != nil {
			return panels.NewPlaceholder("github", th)
		}
		return gitinfo.NewGitHub(client, cfg.Git, cfg.GitHub, cfg.Actions, cwd, cfg.FileTree.IconMode, th)
	})
	// Diff review panel — real implementation
	r.Register("review", func() panels.Panel {
		return setActionsCfg(review.New(gc, th))
	})
	// Agent monitor panel — real implementation
	r.Register("agents", func() panels.Panel {
		maxProcs := cfg.MCP.Security.MaxAgentProcesses
		timeout := cfg.MCP.Security.AgentTimeout
		tracker := mcp.NewAgentTracker(maxProcs, timeout)
		return setActionsCfg(agents.New(tracker, th))
	})
	// Context builder panel — real implementation
	r.Register("context", func() panels.Panel {
		cwd, err := os.Getwd()
		if err != nil {
			cwd = "."
		}
		builder, err := ctxbuilder.NewBuilder(cwd)
		if err != nil {
			return panels.NewPlaceholder("context", th)
		}
		return setActionsCfg(ctxpanel.New(builder, th))
	})
	// Embedded terminal panel — real implementation
	r.Register("terminal", func() panels.Panel {
		shell := cfg.Terminal.Shell
		if shell == "" {
			shell = terminal.DefaultShell()
		}
		runner, err := terminal.New(shell, cfg.Terminal.Scrollback)
		if err != nil {
			return panels.NewPlaceholder("terminal", th)
		}
		return termpanel.New(cfg.Terminal, runner, shell, th)
	})
	// Extension management panel — real implementation
	r.Register("extensions", func() panels.Panel {
		installDir := cfg.Extensions.InstallDir
		if installDir == "" {
			installDir = "extensions"
		}
		mgr := extension.NewManager(installDir)
		if err := mgr.LoadAll(); err != nil {
			return panels.NewPlaceholder("extensions", th)
		}
		return setActionsCfg(extpanel.New(mgr, th))
	})
}
