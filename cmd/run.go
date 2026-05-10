package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/git"
	"github.com/jongio/grut/internal/shortcuts"
	"github.com/spf13/cobra"
)

// newRunCmd creates the "run" subcommand for executing git workflow shortcuts.
func newRunCmd() *cobra.Command {
	var (
		listFlag     bool
		describeFlag string
		dryRunFlag   bool
		noConfirm    bool
	)

	runCmd := &cobra.Command{
		Use:   "run <shortcut> [args...]",
		Short: "Execute an AI-powered git workflow shortcut",
		Long: `Run a named git workflow shortcut. Shortcuts are concise aliases for
multi-step git operations that execute through the AI git client middleware.

Use --list to see all available shortcuts, or --describe <name> to see details.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			applyNoAIFlag(cmd, cfg)

			if !cfg.Shortcuts.Enabled {
				return fmt.Errorf("shortcuts are disabled in config (set shortcuts.enabled = true)")
			}

			// Create git client.
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}
			gc, err := git.NewClient(cwd)
			if err != nil {
				return fmt.Errorf("git client: %w", err)
			}

			engine := shortcuts.NewEngine(gc)

			// Register custom shortcuts from config.
			registerCustomShortcuts(engine, cfg.Shortcuts.Custom)

			w := cmd.OutOrStdout()

			// Handle --list.
			if listFlag {
				all := engine.List()
				if len(all) == 0 {
					_, _ = fmt.Fprintln(w, "No shortcuts available.")
					return nil
				}
				_, _ = fmt.Fprintln(w, "Available shortcuts:")
				_, _ = fmt.Fprintln(w)
				for _, s := range all {
					src := "builtin"
					if !s.Builtin {
						src = "custom"
					}
					_, _ = fmt.Fprintf(w, "  %-12s %-8s %s\n", s.Name, "["+src+"]", s.Description)
				}
				return nil
			}

			// Handle --describe.
			if describeFlag != "" {
				s, ok := engine.Resolve(describeFlag)
				if !ok {
					return fmt.Errorf("unknown shortcut %q", describeFlag)
				}
				printShortcutDetails(cmd, s)
				return nil
			}

			// Normal execution requires a shortcut name.
			if len(args) == 0 {
				return fmt.Errorf("shortcut name required (use --list to see available shortcuts)")
			}

			name := args[0]
			scArgs := parseShortcutArgs(args[1:])

			// Handle --dry-run.
			if dryRunFlag {
				steps, err := engine.Plan(name, scArgs)
				if err != nil {
					return fmt.Errorf("plan shortcut %q: %w", name, err)
				}
				fmt.Fprintf(w, "Dry run for %q:\n\n", name)
				for i, step := range steps {
					params := formatParams(step.Params)
					fmt.Fprintf(w, "  %d. %s %s\n", i+1, step.Op, params)
				}
				return nil
			}

			// Confirmation check.
			sc, ok := engine.Resolve(name)
			if !ok {
				return fmt.Errorf("unknown shortcut %q", name)
			}
			if sc.Confirm && !noConfirm {
				fmt.Fprintf(w, "Will execute shortcut %q: %s\n", sc.Name, sc.Description)
				steps, err := engine.Plan(name, scArgs)
				if err != nil {
					return fmt.Errorf("plan shortcut %q: %w", name, err)
				}
				for i, step := range steps {
					fmt.Fprintf(w, "  %d. %s %s\n", i+1, step.Op, formatParams(step.Params))
				}
				_, _ = fmt.Fprintln(w, "\nSkipping confirmation (use --no-confirm to silence this).")
			}

			// Execute.
			result, err := engine.Execute(cmd.Context(), name, scArgs)
			if err != nil {
				return fmt.Errorf("execute shortcut %q: %w", name, err)
			}

			// Print results.
			for i, sr := range result.StepResults {
				status := "✓"
				if sr.Err != nil {
					status = "✗"
				}
				detail := ""
				if sr.Output != "" {
					detail = " → " + sr.Output
				}
				if sr.Err != nil {
					detail = " → " + sr.Err.Error()
				}
				fmt.Fprintf(w, "  %s %d. %s%s\n", status, i+1, sr.Step.Op, detail)
			}

			if result.Err != nil {
				return result.Err
			}

			_, _ = fmt.Fprintln(w, "\nDone.")
			return nil
		},
	}

	runCmd.Flags().BoolVar(&listFlag, "list", false, "List available shortcuts")
	runCmd.Flags().StringVar(&describeFlag, "describe", "", "Show details for a shortcut")
	runCmd.Flags().BoolVar(&dryRunFlag, "dry-run", false, "Show execution plan without running")
	runCmd.Flags().BoolVar(&noConfirm, "no-confirm", false, "Skip confirmation prompts")

	return runCmd
}

// registerCustomShortcuts converts config CustomShortcut definitions into
// engine-registered Shortcut values.
func registerCustomShortcuts(engine *shortcuts.Engine, customs []config.CustomShortcut) {
	for _, c := range customs {
		steps := make([]shortcuts.Step, 0, len(c.Steps))
		for _, raw := range c.Steps {
			parts := strings.Fields(raw)
			if len(parts) == 0 {
				continue
			}
			step := shortcuts.Step{
				Op:     parts[0],
				Params: make(map[string]string),
				OnFail: shortcuts.OnFailStop,
			}
			// Parse remaining "key=value" pairs as params.
			for _, p := range parts[1:] {
				if k, v, ok := strings.Cut(p, "="); ok {
					step.Params[k] = v
				}
			}
			steps = append(steps, step)
		}
		engine.RegisterCustom(shortcuts.Shortcut{
			Name:        c.Name,
			Description: c.Description,
			Steps:       steps,
			Confirm:     true,
		})
	}
}

// parseShortcutArgs parses "key=value" arguments from the command line.
func parseShortcutArgs(raw []string) map[string]string {
	args := make(map[string]string)
	for _, r := range raw {
		if k, v, ok := strings.Cut(r, "="); ok {
			args[k] = v
		}
	}
	return args
}

// printShortcutDetails prints full details of a shortcut.
func printShortcutDetails(cmd *cobra.Command, s shortcuts.Shortcut) {
	w := cmd.OutOrStdout()
	src := "builtin"
	if !s.Builtin {
		src = "custom"
	}
	fmt.Fprintf(w, "Name:        %s\n", s.Name)
	fmt.Fprintf(w, "Source:      %s\n", src)
	fmt.Fprintf(w, "Description: %s\n", s.Description)
	fmt.Fprintf(w, "Confirm:     %v\n", s.Confirm)

	if len(s.Args) > 0 {
		_, _ = fmt.Fprintln(w, "\nArguments:")
		for _, a := range s.Args {
			req := ""
			if a.Required {
				req = " (required)"
			}
			def := ""
			if a.Default != "" {
				def = fmt.Sprintf(" [default: %s]", a.Default)
			}
			fmt.Fprintf(w, "  %-12s %s%s%s\n", a.Name, a.Prompt, def, req)
		}
	}

	fmt.Fprintln(w, "\nSteps:")
	for i, step := range s.Steps {
		params := formatParams(step.Params)
		fmt.Fprintf(w, "  %d. %s %s (on-fail: %s)\n", i+1, step.Op, params, stepOnFail(step))
	}
}

// formatParams formats step params as a compact string.
func formatParams(params map[string]string) string {
	if len(params) == 0 {
		return ""
	}
	var parts []string
	for k, v := range params {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, " ")
}

// stepOnFail returns the on-fail policy string, defaulting to "stop".
func stepOnFail(s shortcuts.Step) string {
	if s.OnFail == "" {
		return shortcuts.OnFailStop
	}
	return s.OnFail
}
