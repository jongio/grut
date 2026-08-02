package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/proctree"
	"github.com/jongio/grut/internal/theme"
	"github.com/spf13/cobra"
)

type doctorStatus string

const (
	doctorPass doctorStatus = "pass"
	doctorWarn doctorStatus = "warn"
	doctorFail doctorStatus = "fail"
)

const (
	doctorCheckTheme         = "Theme"
	doctorCheckGitRepository = "Git repository"
	doctorCheckGitHubAuth    = "GitHub auth"
	doctorCheckAIProvider    = "AI provider"
	doctorCheckTerminal      = "Terminal"
	doctorAIProviderNone     = "none"
	doctorAIProviderCopilot  = "copilot"
	doctorAIProviderClaude   = "claude"
	doctorSummaryDisabled    = "disabled"
)

type doctorCheck struct {
	Name     string       `json:"name"`
	Status   doctorStatus `json:"status"`
	Required bool         `json:"required"`
	Summary  string       `json:"summary"`
	Detail   string       `json:"detail,omitempty"`
	Fix      string       `json:"fix,omitempty"`
}

type doctorReport struct {
	OK               bool          `json:"ok"`
	ConfigPath       string        `json:"config_path"`
	SelectedTheme    string        `json:"selected_theme,omitempty"`
	GitRepository    string        `json:"git_repository,omitempty"`
	GitHubAuthSource string        `json:"github_auth_source,omitempty"`
	AIProvider       string        `json:"ai_provider,omitempty"`
	Terminal         string        `json:"terminal,omitempty"`
	Checks           []doctorCheck `json:"checks"`
}

type doctorCommandDeps struct {
	loadConfig    configLoadFunc
	configPath    configPathFunc
	configDir     func() string
	dataDir       func() string
	loadTheme     func(string) (string, error)
	runCommand    func(context.Context, string, ...string) (string, string, error)
	getwd         func() (string, error)
	getenv        func(string) string
	checkTerminal func() string
	checkDir      func(string) (bool, bool, error)
}

func newDoctorCmd() *cobra.Command {
	return newDoctorCmdWithDeps(defaultDoctorDeps())
}

func newDoctorCmdWithDeps(deps doctorCommandDeps) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:          "doctor",
		Short:        "Check grut environment health",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			report := runDoctor(cmd.Context(), deps)
			check, _ := cmd.Flags().GetBool("check")
			var err error
			if asJSON {
				err = writeDoctorJSON(cmd.OutOrStdout(), report)
			} else if !check {
				writeDoctorText(cmd.OutOrStdout(), report)
			}
			if err != nil {
				return err
			}
			if !report.OK {
				return errDoctorRequiredChecksFailed
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output the doctor report as JSON")
	cmd.Flags().Bool("check", false, "Exit with an error when required doctor checks fail")
	return cmd
}

var errDoctorRequiredChecksFailed = errors.New("doctor found failed required checks")

func defaultDoctorDeps() doctorCommandDeps {
	return doctorCommandDeps{
		loadConfig: config.Load,
		configPath: config.UserConfigFilePath,
		configDir:  config.ConfigDir,
		dataDir:    config.DataDir,
		loadTheme: func(name string) (string, error) {
			th, err := theme.Load(name)
			if err != nil {
				return "", err
			}
			return th.Name, nil
		},
		runCommand:    runDoctorCommand,
		getwd:         os.Getwd,
		getenv:        os.Getenv,
		checkTerminal: checkTerminalCompat,
		checkDir:      checkDoctorDir,
	}
}

func runDoctor(ctx context.Context, deps doctorCommandDeps) doctorReport {
	deps = normalizeDoctorDeps(deps)
	report := doctorReport{ConfigPath: deps.configPath()}

	cfg, configCheck := checkDoctorConfig(deps)
	report.Checks = append(report.Checks, configCheck)
	report.Checks = append(report.Checks, checkDoctorTheme(deps, cfg))
	gitChecks := checkDoctorGit(ctx, deps)
	report.Checks = append(report.Checks, gitChecks...)
	ghCheck, ghSource := checkDoctorGitHub(ctx, deps)
	report.GitHubAuthSource = ghSource
	report.Checks = append(report.Checks, ghCheck)
	report.Checks = append(report.Checks, checkDoctorAI(deps, cfg, ghSource))
	report.Checks = append(report.Checks, checkDoctorTerminal(deps))
	report.Checks = append(
		report.Checks,
		checkDoctorDirectory(deps, "Config directory", deps.configDir()),
		checkDoctorDirectory(deps, "Data directory", deps.dataDir()),
	)

	if cfg != nil {
		report.SelectedTheme = cfg.Theme.Name
		report.AIProvider = cfg.AI.Provider
	}
	report.OK = !hasFailedRequiredDoctorCheck(report.Checks)
	report.GitRepository = gitRepositorySummary(gitChecks)
	report.Terminal = terminalSummary(deps)
	return report
}

func normalizeDoctorDeps(deps doctorCommandDeps) doctorCommandDeps {
	defaults := defaultDoctorDeps()
	if deps.loadConfig == nil {
		deps.loadConfig = defaults.loadConfig
	}
	if deps.configPath == nil {
		deps.configPath = defaults.configPath
	}
	if deps.configDir == nil {
		deps.configDir = defaults.configDir
	}
	if deps.dataDir == nil {
		deps.dataDir = defaults.dataDir
	}
	if deps.loadTheme == nil {
		deps.loadTheme = defaults.loadTheme
	}
	if deps.runCommand == nil {
		deps.runCommand = defaults.runCommand
	}
	if deps.getwd == nil {
		deps.getwd = defaults.getwd
	}
	if deps.getenv == nil {
		deps.getenv = defaults.getenv
	}
	if deps.checkTerminal == nil {
		deps.checkTerminal = defaults.checkTerminal
	}
	if deps.checkDir == nil {
		deps.checkDir = defaults.checkDir
	}
	return deps
}

func checkDoctorConfig(deps doctorCommandDeps) (*config.Config, doctorCheck) {
	cfg, err := deps.loadConfig()
	if err != nil {
		return nil, doctorCheck{
			Name:     "Config",
			Status:   doctorFail,
			Required: true,
			Summary:  "invalid configuration",
			Detail:   err.Error(),
			Fix:      fmt.Sprintf("Edit %s or run 'grut config defaults -o %s'.", deps.configPath(), deps.configPath()),
		}
	}
	return cfg, doctorCheck{
		Name:     "Config",
		Status:   doctorPass,
		Required: true,
		Summary:  "loaded and valid",
		Detail:   deps.configPath(),
	}
}

func checkDoctorTheme(deps doctorCommandDeps, cfg *config.Config) doctorCheck {
	if cfg == nil {
		return doctorCheck{
			Name:    "Theme",
			Status:  doctorWarn,
			Summary: "skipped because config did not load",
			Fix:     fmt.Sprintf("Fix configuration first: %s.", deps.configPath()),
		}
	}
	resolved, err := deps.loadTheme(cfg.Theme.Name)
	if err != nil {
		return doctorCheck{
			Name:     doctorCheckTheme,
			Status:   doctorFail,
			Required: true,
			Summary:  fmt.Sprintf("cannot load %q", cfg.Theme.Name),
			Detail:   err.Error(),
			Fix:      fmt.Sprintf("Set [theme].name in %s to a built-in theme such as \"default\".", deps.configPath()),
		}
	}
	return doctorCheck{
		Name:     doctorCheckTheme,
		Status:   doctorPass,
		Required: true,
		Summary:  fmt.Sprintf("%s ready", resolved),
		Detail:   fmt.Sprintf("configured: %s", cfg.Theme.Name),
	}
}

func checkDoctorGit(ctx context.Context, deps doctorCommandDeps) []doctorCheck {
	stdout, stderr, err := deps.runCommand(ctx, "git", "--version")
	if err != nil {
		return []doctorCheck{{
			Name:     "Git",
			Status:   doctorFail,
			Required: true,
			Summary:  "git not available",
			Detail:   commandDetail(stderr, err),
			Fix:      "Install Git from https://git-scm.com/downloads and ensure git is on PATH.",
		}}
	}
	checks := []doctorCheck{{
		Name:     "Git",
		Status:   doctorPass,
		Required: true,
		Summary:  strings.TrimSpace(stdout),
	}}

	cwd, err := deps.getwd()
	if err != nil {
		checks = append(checks, doctorCheck{
			Name:    doctorCheckGitRepository,
			Status:  doctorWarn,
			Summary: "cannot determine working directory",
			Detail:  err.Error(),
			Fix:     "Run grut doctor from a readable project directory.",
		})
		return checks
	}
	stdout, stderr, err = deps.runCommand(ctx, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		checks = append(checks, doctorCheck{
			Name:    doctorCheckGitRepository,
			Status:  doctorWarn,
			Summary: "not inside a git repository",
			Detail:  fmt.Sprintf("cwd: %s; %s", cwd, commandDetail(stderr, err)),
			Fix:     "Run from a git worktree or initialize one with 'git init'.",
		})
		return checks
	}
	checks = append(checks, doctorCheck{
		Name:    doctorCheckGitRepository,
		Status:  doctorPass,
		Summary: "inside worktree",
		Detail:  strings.TrimSpace(stdout),
	})
	return checks
}

func checkDoctorGitHub(ctx context.Context, deps doctorCommandDeps) (doctorCheck, string) {
	stdout, stderr, err := deps.runCommand(ctx, "gh", "auth", "token")
	if err == nil && strings.TrimSpace(stdout) != "" {
		return doctorCheck{
			Name:    doctorCheckGitHubAuth,
			Status:  doctorPass,
			Summary: "authenticated via gh CLI",
			Detail:  "source: gh auth token",
		}, "gh"
	}
	if deps.getenv("GITHUB_TOKEN") != "" {
		return doctorCheck{
			Name:    doctorCheckGitHubAuth,
			Status:  doctorPass,
			Summary: "authenticated via GITHUB_TOKEN",
			Detail:  "source: GITHUB_TOKEN",
		}, "GITHUB_TOKEN"
	}
	return doctorCheck{
		Name:    doctorCheckGitHubAuth,
		Status:  doctorWarn,
		Summary: "not authenticated",
		Detail:  commandDetail(stderr, err),
		Fix:     "Run 'gh auth login' or set GITHUB_TOKEN.",
	}, doctorAIProviderNone
}

func checkDoctorAI(deps doctorCommandDeps, cfg *config.Config, ghSource string) doctorCheck {
	if cfg == nil {
		return doctorCheck{
			Name:    doctorCheckAIProvider,
			Status:  doctorWarn,
			Summary: "skipped because config did not load",
			Fix:     fmt.Sprintf("Fix configuration first: %s.", deps.configPath()),
		}
	}
	if !cfg.AI.Enabled || cfg.AI.Provider == doctorAIProviderNone {
		return doctorCheck{
			Name:    doctorCheckAIProvider,
			Status:  doctorPass,
			Summary: doctorSummaryDisabled,
			Detail:  fmt.Sprintf("provider: %s", cfg.AI.Provider),
		}
	}
	switch cfg.AI.Provider {
	case doctorAIProviderCopilot:
		if ghSource != doctorAIProviderNone {
			return doctorCheck{
				Name:    doctorCheckAIProvider,
				Status:  doctorPass,
				Summary: "copilot auth source available",
				Detail:  fmt.Sprintf("source: %s", ghSource),
			}
		}
		return doctorCheck{
			Name:    doctorCheckAIProvider,
			Status:  doctorWarn,
			Summary: "copilot may need GitHub auth",
			Fix:     "Run 'gh auth login' or set GITHUB_TOKEN before using Copilot features.",
		}
	case doctorAIProviderClaude:
		if deps.getenv("ANTHROPIC_API_KEY") != "" {
			return doctorCheck{
				Name:    doctorCheckAIProvider,
				Status:  doctorPass,
				Summary: "claude API key present",
				Detail:  "source: ANTHROPIC_API_KEY",
			}
		}
		return doctorCheck{
			Name:    doctorCheckAIProvider,
			Status:  doctorWarn,
			Summary: "missing ANTHROPIC_API_KEY",
			Fix:     "Set ANTHROPIC_API_KEY or change [ai].provider to \"copilot\" or \"none\".",
		}
	default:
		return doctorCheck{
			Name:     doctorCheckAIProvider,
			Status:   doctorFail,
			Required: true,
			Summary:  fmt.Sprintf("unknown provider %q", cfg.AI.Provider),
			Fix:      fmt.Sprintf("Edit [ai].provider in %s.", deps.configPath()),
		}
	}
}

func checkDoctorTerminal(deps doctorCommandDeps) doctorCheck {
	if msg := deps.checkTerminal(); msg != "" {
		return doctorCheck{
			Name:     doctorCheckTerminal,
			Status:   doctorFail,
			Required: true,
			Summary:  "incompatible terminal",
			Detail:   firstLine(msg),
			Fix:      "Use PowerShell, Command Prompt, Windows Terminal, or set GRUT_FORCE_TERMINAL=1 if you know this terminal is compatible.",
		}
	}
	term := deps.getenv("TERM")
	if strings.EqualFold(term, "dumb") {
		return doctorCheck{
			Name:    doctorCheckTerminal,
			Status:  doctorWarn,
			Summary: "TERM=dumb may limit rendering",
			Fix:     "Use a modern terminal with color and alternate-screen support.",
		}
	}
	return doctorCheck{
		Name:    doctorCheckTerminal,
		Status:  doctorPass,
		Summary: "compatible",
		Detail:  terminalSummary(deps),
	}
}

func checkDoctorDirectory(deps doctorCommandDeps, name, path string) doctorCheck {
	exists, writable, err := deps.checkDir(path)
	if err != nil {
		return doctorCheck{
			Name:     name,
			Status:   doctorFail,
			Required: true,
			Summary:  "not writable",
			Detail:   fmt.Sprintf("%s: %v", path, err),
			Fix:      fmt.Sprintf("Create %s and grant your user write permission.", path),
		}
	}
	if !exists {
		return doctorCheck{
			Name:    name,
			Status:  doctorWarn,
			Summary: "directory does not exist yet",
			Detail:  path,
			Fix:     fmt.Sprintf("Create it with 'mkdir %s' if grut cannot create it automatically.", path),
		}
	}
	if !writable {
		return doctorCheck{
			Name:     name,
			Status:   doctorFail,
			Required: true,
			Summary:  "directory is not writable",
			Detail:   path,
			Fix:      fmt.Sprintf("Grant write permission to %s.", path),
		}
	}
	return doctorCheck{
		Name:    name,
		Status:  doctorPass,
		Summary: "exists and writable",
		Detail:  path,
	}
}

func runDoctorCommand(ctx context.Context, name string, args ...string) (string, string, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := proctree.Command(cmdCtx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := proctree.Run(cmd)
	return stdout.String(), stderr.String(), err
}

func checkDoctorDir(path string) (bool, bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, false, nil
		}
		return false, false, err
	}
	if !info.IsDir() {
		return true, false, fmt.Errorf("not a directory")
	}
	probe := filepath.Join(path, fmt.Sprintf(".grut-doctor-write-check-%d", os.Getpid()))
	f, err := os.OpenFile(probe, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return true, false, err
	}
	if closeErr := f.Close(); closeErr != nil {
		_ = os.Remove(probe)
		return true, false, closeErr
	}
	if err := os.Remove(probe); err != nil {
		return true, false, err
	}
	return true, true, nil
}

func hasFailedRequiredDoctorCheck(checks []doctorCheck) bool {
	for _, check := range checks {
		if check.Required && check.Status == doctorFail {
			return true
		}
	}
	return false
}

func writeDoctorJSON(w io.Writer, report doctorReport) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

func writeDoctorText(w io.Writer, report doctorReport) {
	fmt.Fprintln(w, "grut doctor")
	fmt.Fprintln(w)
	for _, check := range report.Checks {
		fmt.Fprintf(w, "%s %-18s %-4s %s\n", doctorIcon(check.Status), check.Name, strings.ToUpper(string(check.Status)), check.Summary)
		if check.Detail != "" {
			fmt.Fprintf(w, "  %s\n", check.Detail)
		}
		if check.Fix != "" {
			fmt.Fprintf(w, "  fix: %s\n", check.Fix)
		}
	}
	fmt.Fprintln(w)
	if report.OK {
		fmt.Fprintln(w, "All required checks passed.")
	} else {
		fmt.Fprintln(w, "One or more required checks failed.")
	}
}

func doctorIcon(status doctorStatus) string {
	switch status {
	case doctorPass:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Render("✓")
	case doctorWarn:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#F1FA8C")).Render("⚠")
	case doctorFail:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Render("✗")
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#8BE9FD")).Render("ℹ")
	}
}

func commandDetail(stderr string, err error) string {
	detail := strings.TrimSpace(stderr)
	if detail != "" {
		return detail
	}
	if err != nil {
		return err.Error()
	}
	return ""
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}

func gitRepositorySummary(checks []doctorCheck) string {
	for _, check := range checks {
		if check.Name == doctorCheckGitRepository {
			if check.Status == doctorPass {
				return check.Detail
			}
			return check.Summary
		}
	}
	return ""
}

func terminalSummary(deps doctorCommandDeps) string {
	term := deps.getenv("TERM")
	if term == "" {
		term = "unknown"
	}
	return fmt.Sprintf("%s/%s TERM=%s", runtime.GOOS, runtime.GOARCH, term)
}
