package cmd

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/jongio/grut/internal/ai"
	"github.com/jongio/grut/internal/ai/middleware"
	"github.com/jongio/grut/internal/bookmarks"
	"github.com/jongio/grut/internal/chat"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/keymap"
	"github.com/jongio/grut/internal/layout"
	"github.com/jongio/grut/internal/mcp"
	"github.com/jongio/grut/internal/session"
	"github.com/jongio/grut/internal/theme"
	"github.com/jongio/grut/internal/tui"
	"github.com/jongio/grut/internal/update"
	"github.com/spf13/cobra"
)

// newRootCommand creates and returns the fully-configured root cobra command
// with all subcommands and flags registered, along with a cleanup function
// that flushes profiling resources. Callers must defer the cleanup function
// after invoking cmd.Execute() to guarantee CPU/memory profiles are written
// even when RunE returns an error.
//
// This is intentionally unexported — external callers should use Execute().
func newRootCommand() (*cobra.Command, func()) {
	return buildRootCommand()
}

// buildRootCommand creates the root command and returns a cleanup function
// that must be deferred after cmd.Execute(). This guarantees profiling
// resources are released even when RunE returns an error (Cobra does not
// call PersistentPostRunE on RunE failure).
func buildRootCommand() (rootCmd *cobra.Command, cleanup func()) {
	// cpuProfileFile is scoped to this function, not package-level,
	// eliminating shared mutable state (CR-008).
	var cpuProfileFile *os.File

	rootCmd = &cobra.Command{
		Use:   "grut",
		Short: "AI-native terminal file explorer, git client, and agent orchestrator",
		Long: `grut is an AI-native terminal file explorer, git client, and agent orchestrator
built for developers who work alongside AI coding agents.

Features include a tree-view file explorer with git status markers, full git client
operations, tmux-like panes, AI agent integration via MCP, and worktree-first workflows.

Environment:
  GRUT_LOG              Path to a log file (enables debug logging)
  GRUT_FORCE_TERMINAL   Bypass the MinTTY/MSYS compatibility check (set to any
                        non-empty value, e.g. GRUT_FORCE_TERMINAL=1)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Handle --reset-welcome: reset first-run state and exit.
			if resetWelcome, _ := cmd.Flags().GetBool("reset-welcome"); resetWelcome {
				return resetWelcomeState()
			}

			// Start background update check early so it can run concurrently
			// with config loading and TUI startup.
			updateCh := make(chan *update.UpdateInfo, 1)
			go func() {
				updateCh <- update.CheckForUpdate(config.AppVersion)
			}()

			// Handle --demo: create a temporary project and chdir into it.
			demo, _ := cmd.Flags().GetBool("demo")
			if demo {
				dir, cleanup, err := setupDemoProject()
				if err != nil {
					return fmt.Errorf("set up demo project: %w", err)
				}
				defer cleanup()
				if err := os.Chdir(dir); err != nil {
					return fmt.Errorf("chdir to demo: %w", err)
				}
				fmt.Fprintf(os.Stderr, "Demo project created at %s\n", dir)
			}

			// Capture original stderr BEFORE redirection so error messages
			// (and the update notification) still reach the real console.
			origStderr := captureOriginalStderr()
			if origStderr == nil {
				origStderr = os.Stderr
			}
			if origStderr != os.Stderr {
				defer origStderr.Close()
			}

			// Redirect stderr BEFORE starting Bubble Tea. Subprocess stderr
			// (MCP agents, extension runtimes) would otherwise leak into the
			// alt-screen buffer. GRUT_LOG enables structured debug logging.
			var logFile *os.File
			logWriter := io.Discard
			if logPath := os.Getenv("GRUT_LOG"); logPath != "" {
				logFile = openLogPath(logPath)
				if logFile != nil {
					logWriter = logFile
				}
			}
			if logFile != nil {
				if err := redirectStderr(logFile); err != nil {
					slog.Warn("stderr redirect failed", "error", err)
				}
				defer logFile.Close()
			} else {
				if devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0); err == nil {
					if err := redirectStderr(devNull); err != nil {
						slog.Warn("stderr redirect to devnull failed", "error", err)
					}
					defer devNull.Close()
				}
			}

			// Configure slog to write to the log file (or discard) instead
			// of stderr, preventing structured log output from contaminating
			// the Bubble Tea alt-screen.
			slog.SetDefault(slog.New(slog.NewTextHandler(logWriter, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			})))

			// Detect incompatible terminal environments (e.g. MSYS/Git Bash
			// without a real Windows console handle) and bail out with a
			// helpful message rather than showing a blank screen.
			if msg := checkTerminalCompat(); msg != "" {
				fmt.Fprintln(origStderr, msg)
				return fmt.Errorf("incompatible terminal")
			}

			// Load configuration
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			// Override AI if --no-ai flag is set.
			applyNoAIFlag(cmd, cfg)

			// Load theme from config
			th, err := theme.Load(cfg.Theme.Name)
			if err != nil {
				return fmt.Errorf("load theme: %w", err)
			}

			// Create panel registry with defaults
			reg := layout.NewRegistry()

			// Create git client for git-aware panels (nil if unavailable)
			cwd, _ := os.Getwd()
			var gc git.GitClient
			var undoMgr *git.UndoManager
			if client, err := git.NewClient(cwd); err == nil {
				gc = client
				undoMgr = git.NewUndoManager(client)
			}

			layout.RegisterDefaults(reg, cfg, gc, th)

			// Create session manager and attempt to restore previous session.
			sessMgr := session.NewManager()
			preset := restoreSessionOrDefault(sessMgr, cfg, cwd, gc)

			// Create the layout engine
			engine, err := layout.NewEngine(reg, preset)
			if err != nil {
				return fmt.Errorf("initialize layout: %w", err)
			}

			// If session had additional tabs, add them now.
			if cfg.Session.Enabled {
				addRestoredTabs(sessMgr, engine, cwd)
			}

			// Apply persisted preview position from config AFTER restoring
			// tabs so it is not overwritten by addRestoredTabs → SwitchTab.
			if cfg.Preview.Position != "" {
				engine.SetPreviewPosition(layout.PreviewPositionFromString(cfg.Preview.Position))
			}

			// Create keymap from configured scheme
			km, err := keymap.NewKeymap(cfg.General.KeybindingScheme)
			if err != nil {
				return fmt.Errorf("load keymap: %w", err)
			}
			keymap.WarnConflicts(km.Bindings())

			// Create bookmark manager
			bmMgr := bookmarks.NewManager(cfg.Bookmarks)

			// Wire AI subsystem: register providers, wrap git client with
			// AI middleware, and attach the chat footer when enabled.
			var chatModel *chat.Model
			var aiRegistry *ai.Registry
			gitClient := gc // default to plain client

			if cfg.AI.Enabled && cfg.AI.Chat.Enabled && gc != nil {
				var cm *chat.Model
				cm, aiRegistry = initChat(cfg, gc, cwd, th)
				chatModel = cm

				if aiRegistry != nil {
					redactor := ai.NewRedactor(cfg.AI.RedactPatterns)
					builder := ai.NewBuilder(gc, redactor, cfg.AI.MaxContextTokens)
					audit, _ := ai.NewAuditLogger("")
					gitClient = middleware.NewAIGitClient(gc, aiRegistry, builder, audit, cfg.AI)
				}
			}

			if aiRegistry != nil {
				defer func() { _ = aiRegistry.Close() }()
			}

			// Create and run the TUI
			model := tui.New(engine, th, km, bmMgr).
				WithUndoManager(undoMgr).
				WithGitClient(gitClient).
				WithConfig(cfg).
				WithSessionManager(sessMgr)

			if chatModel != nil {
				model = model.WithChat(chatModel)
			}

			p := tea.NewProgram(model)
			if _, err := p.Run(); err != nil {
				fmt.Fprintf(origStderr, "Error: run TUI: %v\n", err)
				return fmt.Errorf("run TUI: %w", err)
			}

			// After TUI exits, show update notification if available.
			showUpdateNotification(origStderr, updateCh)

			return nil
		},
	}

	// Set version so that --version / -v flags work via Cobra.
	rootCmd.Version = config.AppVersion
	rootCmd.SetVersionTemplate("{{.Version}}\n")

	// ---------------------------------------------------------------------------
	// Performance profiling hooks (pprof)
	// ---------------------------------------------------------------------------

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		cpuPath, _ := cmd.Flags().GetString("cpu-profile")
		if cpuPath != "" {
			f, err := os.Create(cpuPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not create CPU profile %q: %v\n", cpuPath, err)
				return nil
			}
			if err := pprof.StartCPUProfile(f); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not start CPU profile: %v\n", err)
				f.Close()
				return nil
			}
			cpuProfileFile = f
		}
		return nil
	}

	// PersistentPostRunE is intentionally omitted: Cobra skips it when RunE
	// returns an error, so profiling cleanup would be lost on failure. The
	// returned cleanup function (deferred in Execute) handles it instead.

	// Global flags available to all subcommands.
	rootCmd.PersistentFlags().String("cpu-profile", "", "write CPU profile to `file`")
	rootCmd.PersistentFlags().String("mem-profile", "", "write memory profile to `file`")
	rootCmd.PersistentFlags().Bool("no-ai", false, "Disable AI features for this operation")
	rootCmd.PersistentFlags().Bool("demo", false, "Launch with a demo project to explore grut")
	rootCmd.PersistentFlags().Bool("reset-welcome", false, "Reset first-run state so the welcome screen shows on next launch")

	// Register subcommands via constructors (no init() side effects).
	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newUpdateCmd())
	rootCmd.AddCommand(newMCPCmd())
	rootCmd.AddCommand(newExtCmd())
	rootCmd.AddCommand(newRunCmd())
	rootCmd.AddCommand(newReportCmd())

	// cleanup releases profiling resources. It is idempotent — safe to call
	// multiple times (subsequent calls are no-ops).
	var cleanupDone bool
	cleanup = func() {
		if cleanupDone {
			return
		}
		cleanupDone = true

		if cpuProfileFile != nil {
			pprof.StopCPUProfile()
			cpuProfileFile.Close()
		}

		memPath, _ := rootCmd.Flags().GetString("mem-profile")
		if memPath != "" {
			f, err := os.Create(memPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not create memory profile %q: %v\n", memPath, err)
				return
			}
			defer f.Close()
			runtime.GC()
			if err := pprof.WriteHeapProfile(f); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not write memory profile: %v\n", err)
			}
		}
	}

	return rootCmd, cleanup
}

// restoreSessionOrDefault attempts to load a saved session for the given
// working directory and returns the first tab's preset. Falls back to
// ExplorerPreset when sessions are disabled or no saved session exists.
func restoreSessionOrDefault(mgr *session.Manager, cfg *config.Config, workDir string, _ git.GitClient) layout.Preset {
	defaultPreset := func() layout.Preset { //nolint:gocritic // closure needed for deferred evaluation
		// v1: always default to ExplorerPreset. The filetree panel
		// already shows git status indicators when .git exists.
		// GitPreset and other presets are preserved for v2 multi-tab.
		return layout.ExplorerPreset()
	}

	if !cfg.Session.Enabled {
		return defaultPreset()
	}

	state, err := mgr.Load(workDir)
	if err != nil {
		slog.Warn("failed to load session, using default layout", "err", err)
		return defaultPreset()
	}
	if state == nil || len(state.Tabs) == 0 {
		return defaultPreset()
	}

	// Use the first tab's preset to initialise the engine.
	presetName := state.Tabs[0].Preset
	if presetName == "" {
		presetName = state.Tabs[0].Name
	}
	if p, ok := layout.Presets()[presetName]; ok {
		return p
	}
	return defaultPreset()
}

// addRestoredTabs adds any additional tabs (beyond the first) from a
// saved session to the engine. The first tab was already used to
// create the engine, so we start from index 1.
func addRestoredTabs(mgr *session.Manager, engine *layout.Engine, workDir string) {
	// v1: single-tab only — do not restore additional tabs.
	// The first tab is already created by the engine; extra tabs are
	// suppressed until v2 re-enables multi-tab support.
	if layout.SingleTabMode {
		return
	}

	state, err := mgr.Load(workDir)
	if err != nil || state == nil || len(state.Tabs) <= 1 {
		return
	}

	presets := layout.Presets()
	for _, tab := range state.Tabs[1:] {
		presetName := tab.Preset
		if presetName == "" {
			presetName = tab.Name
		}
		p, ok := presets[presetName]
		if !ok {
			p = layout.ExplorerPreset()
		}
		if _, err := engine.AddTab(p); err != nil {
			slog.Warn("failed to restore tab", "tab", tab.Name, "err", err)
		}
	}

	// Restore the active tab index.
	if state.ActiveTab > 0 && state.ActiveTab < engine.TabManager().Count() {
		_ = engine.SwitchTab(state.ActiveTab)
	}
}

// Execute creates the root command and runs it. Returns an error on failure;
// the caller (main) is responsible for os.Exit. Profiling cleanup is deferred
// so it runs even when RunE returns an error.
func Execute() error {
	rootCmd, cleanup := buildRootCommand()
	defer cleanup()
	return rootCmd.Execute()
}

// showUpdateNotification prints an update notification to the given writer
// if the background version check found a newer release.
func showUpdateNotification(w io.Writer, ch <-chan *update.UpdateInfo) {
	if w == nil {
		w = os.Stderr
	}
	select {
	case info := <-ch:
		if info != nil {
			fmt.Fprintf(w, "\nA new version of grut is available: v%s → v%s\nRun \"grut update\" to install it.\n",
				info.CurrentVersion, info.LatestVersion)
		}
	default:
	}
}

// applyNoAIFlag checks the --no-ai persistent flag and, when set, disables
// all AI features in the configuration. This is the single point where the
// flag overrides config, ensuring consistent behaviour across all subcommands.
func applyNoAIFlag(cmd *cobra.Command, cfg *config.Config) {
	noAI, _ := cmd.Flags().GetBool("no-ai")
	if noAI {
		cfg.AI.Enabled = false
	}
}

// resetWelcomeState removes the first-run marker file and sets the
// show_first_run_help config back to true so the welcome screen
// auto-shows on next launch.
func resetWelcomeState() error {
	if err := session.ResetFirstRun(); err != nil {
		return fmt.Errorf("reset first-run marker: %w", err)
	}
	if err := config.SaveUserSettingBool("general.show_first_run_help", true); err != nil {
		return fmt.Errorf("reset show_first_run_help config: %w", err)
	}
	fmt.Fprintln(os.Stderr, "Welcome screen reset. It will show again on next launch.")
	return nil
}

// openLogPath validates and opens a log file path for GRUT_LOG.
// Returns nil if the path is invalid (relative, UNC) or cannot be opened.
func openLogPath(raw string) *os.File {
	cleaned := filepath.Clean(raw)
	if !filepath.IsAbs(cleaned) {
		return nil
	}
	if strings.HasPrefix(cleaned, `\\`) {
		return nil
	}
	// filepath.Clean normalises "//" to "/" on non-Windows, so check the
	// raw input to catch forward-slash UNC paths portably.
	if strings.HasPrefix(raw, "//") {
		return nil
	}
	f, err := os.OpenFile(cleaned, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil
	}
	return f
}

// initChat creates and returns a chat.Model and its AI Registry when all
// prerequisites are available. Returns (nil, nil) if setup fails so the
// app degrades gracefully.
func initChat(cfg *config.Config, gc git.GitClient, repoRoot string, th *theme.Theme) (*chat.Model, *ai.Registry) {
	registry := ai.NewRegistry(cfg.AI)

	// Register configured providers. Errors are non-fatal — the registry
	// falls back gracefully if a provider is unavailable at call-time.
	if p, err := ai.NewCopilotProvider(cfg.AI.Copilot.Model); err != nil {
		slog.Warn("chat: copilot provider unavailable", "err", err)
	} else {
		registry.Register("copilot", p)
	}
	registry.Register("claude", ai.NewClaudeProvider(cfg.AI.Claude.Model, cfg.AI.Claude.MaxTokens))

	redactor := ai.NewRedactor(cfg.AI.RedactPatterns)
	audit, err := ai.NewAuditLogger("")
	if err != nil {
		slog.Warn("chat: audit logger unavailable, continuing without audit", "err", err)
		audit = nil
	}

	jail, err := mcp.NewPathJail(repoRoot, false)
	if err != nil {
		slog.Warn("chat: path jail unavailable, chat disabled", "err", err)
		return nil, registry
	}
	limiter := mcp.NewRateLimiter(60, 30)
	toolReg := chat.NewToolRegistry()
	executor := chat.NewToolExecutor(gc, jail, limiter, toolReg)
	confirming := chat.NewConfirmationManager(toolReg)
	sysPrompt := chat.NewSystemPromptBuilder(gc, cfg.AI.Chat.SystemPrompt)
	chatModel := chat.New(chat.Deps{
		Registry:   registry,
		Executor:   executor,
		Confirming: confirming,
		SysPrompt:  sysPrompt,
		Redactor:   redactor,
		Audit:      audit,
		Theme:      th,
		ChatCfg:    cfg.AI.Chat,
	})
	return &chatModel, registry
}
