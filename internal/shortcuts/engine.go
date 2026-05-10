package shortcuts

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jongio/grut/internal/git"
)

// Sentinel errors for testable error checking via errors.Is().
var (
	ErrUnknownShortcut      = errors.New("unknown shortcut")
	ErrUnknownOperation     = errors.New("unknown operation")
	ErrUnsupportedResetMode = errors.New("unsupported reset mode")
)

// Engine resolves and executes shortcut workflows.
type Engine struct {
	git      git.GitClient
	builtins map[string]Shortcut
	custom   map[string]Shortcut
}

// NewEngine creates a new shortcut engine backed by the given git client.
func NewEngine(gc git.GitClient) *Engine {
	builtinMap := make(map[string]Shortcut)
	for _, s := range Builtins() {
		builtinMap[s.Name] = s
	}
	return &Engine{
		git:      gc,
		builtins: builtinMap,
		custom:   make(map[string]Shortcut),
	}
}

// RegisterCustom adds a custom shortcut, overriding any built-in with the
// same name.
func (e *Engine) RegisterCustom(s Shortcut) {
	s.Builtin = false
	e.custom[s.Name] = s
}

// Resolve looks up a shortcut by name. Custom shortcuts take precedence
// over built-ins.
func (e *Engine) Resolve(name string) (Shortcut, bool) {
	if s, ok := e.custom[name]; ok {
		return s, true
	}
	if s, ok := e.builtins[name]; ok {
		return s, true
	}
	return Shortcut{}, false
}

// List returns all available shortcuts (custom overrides merged with built-ins).
func (e *Engine) List() []Shortcut {
	seen := make(map[string]bool)
	var result []Shortcut

	// Custom shortcuts first.
	for _, s := range e.custom {
		seen[s.Name] = true
		result = append(result, s)
	}
	// Built-ins that are not overridden.
	for _, s := range Builtins() {
		if !seen[s.Name] {
			result = append(result, s)
		}
	}
	return result
}

// Plan returns the execution steps for a shortcut after resolving
// argument placeholders. This is useful for --dry-run previews.
func (e *Engine) Plan(name string, args map[string]string) ([]Step, error) {
	sc, ok := e.Resolve(name)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownShortcut, name)
	}

	// Resolve argument defaults and validate required args.
	resolved, err := resolveArgs(sc.Args, args)
	if err != nil {
		return nil, fmt.Errorf("resolving args for shortcut %q: %w", name, err)
	}

	// Substitute placeholders in step params.
	steps := make([]Step, len(sc.Steps))
	for i, s := range sc.Steps {
		steps[i] = substituteStep(s, resolved)
	}
	return steps, nil
}

// Execute runs the shortcut with the given arguments. Steps are executed
// sequentially through the git client.
func (e *Engine) Execute(ctx context.Context, name string, args map[string]string) (*ExecutionResult, error) {
	sc, ok := e.Resolve(name)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownShortcut, name)
	}

	resolved, err := resolveArgs(sc.Args, args)
	if err != nil {
		return nil, fmt.Errorf("resolve args for %q: %w", name, err)
	}

	result := &ExecutionResult{Shortcut: sc}

	for _, step := range sc.Steps {
		step = substituteStep(step, resolved)
		sr := e.executeStep(ctx, step)
		result.StepResults = append(result.StepResults, sr)

		if sr.Err != nil {
			switch step.OnFail {
			case OnFailStop, "":
				result.Err = fmt.Errorf("step %q failed: %w", step.Op, sr.Err)
				return result, nil
			case OnFailContinue:
				// Log and continue.
			case OnFailAsk:
				// In non-interactive mode, treat as stop.
				result.Err = fmt.Errorf("step %q failed (would ask in interactive mode): %w", step.Op, sr.Err)
				return result, nil
			}
		}
	}
	return result, nil
}

func (e *Engine) executeStep(ctx context.Context, step Step) StepResult {
	sr := StepResult{Step: step}

	var err error
	switch step.Op {
	case OpStage:
		paths := parsePaths(step.Params["paths"])
		err = e.git.Stage(ctx, paths)

	case OpUnstage:
		paths := parsePaths(step.Params["paths"])
		err = e.git.Unstage(ctx, paths)

	case OpCommit:
		msg := step.Params["message"]
		opts := git.CommitOpts{
			Amend: step.Params["amend"] == "true", //nolint:goconst // inline string is more readable here
			Fixup: step.Params["fixup"],
		}
		if msg == "" && step.Params["ai_message"] != "true" && opts.Fixup == "" {
			msg = "auto-commit"
		}
		var hash string
		hash, err = e.git.Commit(ctx, msg, opts)
		if err == nil {
			sr.Output = hash
		}

	case OpPush:
		opts := git.PushOpts{
			Remote:      step.Params["remote"],
			Branch:      step.Params["branch"],
			Force:       step.Params["force"] == "true",
			SetUpstream: step.Params["set_upstream"] == "true",
		}
		err = e.git.Push(ctx, opts)

	case OpPull:
		opts := git.PullOpts{
			Remote: step.Params["remote"],
			Branch: step.Params["branch"],
			Rebase: step.Params["rebase"] == "true",
		}
		err = e.git.Pull(ctx, opts)

	case OpFetch:
		opts := git.FetchOpts{
			Remote: step.Params["remote"],
			All:    step.Params["all"] == "true",
			Prune:  step.Params["prune"] == "true",
		}
		err = e.git.Fetch(ctx, opts)

	case OpRebase:
		onto := step.Params["onto"]
		err = e.git.Rebase(ctx, onto, git.RebaseOpts{})

	case OpMerge:
		branch := step.Params["branch"]
		opts := git.MergeOpts{
			Squash:  step.Params["squash"] == "true",
			NoFF:    step.Params["no_ff"] == "true",
			Message: step.Params["message"],
		}
		err = e.git.Merge(ctx, branch, opts)

	case OpCheckout:
		ref := step.Params["ref"]
		err = e.git.Checkout(ctx, ref)

	case OpBranch:
		name := step.Params["name"]
		base := step.Params["base"]
		err = e.git.BranchCreate(ctx, name, base)

	case OpReset:
		// Reset is implemented via Unstage for mixed, or requires
		// checkout for soft. We map to available git client ops.
		mode := step.Params["mode"]
		ref := step.Params["ref"]
		switch mode {
		case "soft":
			err = e.git.Checkout(ctx, ref)
		case "hard":
			// Hard reset: checkout the ref and unstage everything.
			err = e.git.Checkout(ctx, ref)
			if err == nil {
				err = e.git.Unstage(ctx, []string{"."})
			}
		case "mixed", "":
			err = e.git.Unstage(ctx, []string{"."})
		default:
			err = fmt.Errorf("%w: %q", ErrUnsupportedResetMode, mode)
		}

	case OpDelete:
		if step.Params["merged"] == "true" {
			err = e.deletemergedBranches(ctx)
		} else {
			branch := step.Params["branch"]
			err = e.git.BranchDelete(ctx, branch, false)
		}

	case OpStash:
		opts := git.StashOpts{
			Message: step.Params["message"],
		}
		err = e.git.StashPush(ctx, opts)

	case OpStashPop:
		err = e.git.StashPop(ctx, 0)

	case OpBranchRename:
		newName := step.Params["new_name"]
		err = e.git.BranchRename(ctx, "", newName)

	default:
		err = fmt.Errorf("%w: %q", ErrUnknownOperation, step.Op)
	}

	sr.Err = err
	return sr
}

// deletemergedBranches deletes local branches that have been merged into
// the current branch.
func (e *Engine) deletemergedBranches(ctx context.Context) error {
	branches, err := e.git.BranchList(ctx)
	if err != nil {
		return fmt.Errorf("list branches: %w", err)
	}

	var errs []error
	for _, b := range branches {
		// Skip current, remote, and protected branches.
		if b.IsCurrent || b.IsRemote {
			continue
		}
		if b.Name == "main" || b.Name == "master" || b.Name == "develop" {
			continue
		}
		if err := e.git.BranchDelete(ctx, b.Name, false); err != nil {
			errs = append(errs, fmt.Errorf("delete %s: %w", b.Name, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("some branches could not be deleted: %w", errors.Join(errs...))
	}
	return nil
}

// resolveArgs validates required arguments and fills in defaults.
func resolveArgs(defs []Arg, provided map[string]string) (map[string]string, error) {
	resolved := make(map[string]string)
	for _, a := range defs {
		if v, ok := provided[a.Name]; ok && v != "" {
			resolved[a.Name] = v
		} else if a.Default != "" {
			resolved[a.Name] = a.Default
		} else if a.Required {
			return nil, fmt.Errorf("required argument %q not provided", a.Name)
		}
	}
	// Pass through any extra args.
	for k, v := range provided {
		if _, ok := resolved[k]; !ok {
			resolved[k] = v
		}
	}
	return resolved, nil
}

// substituteStep replaces {{key}} placeholders in step params with resolved
// argument values.
func substituteStep(s Step, args map[string]string) Step {
	out := Step{
		Op:       s.Op,
		OnFail:   s.OnFail,
		AIAssist: s.AIAssist,
		Params:   make(map[string]string, len(s.Params)),
	}
	for k, v := range s.Params {
		for argName, argVal := range args {
			v = strings.ReplaceAll(v, "{{"+argName+"}}", argVal)
		}
		out.Params[k] = v
	}
	return out
}

// parsePaths splits a comma-or-space separated path list. A single "."
// is treated as stage-all.
func parsePaths(raw string) []string {
	if raw == "" || raw == "." {
		return []string{"."}
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' '
	})
	if len(parts) == 0 {
		return []string{"."}
	}
	return parts
}
