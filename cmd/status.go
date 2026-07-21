package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/jongio/grut/internal/git"
	"github.com/spf13/cobra"
)

// statusEntry is a single changed file in the JSON output.
type statusEntry struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

// statusCounts summarizes how many entries fall into each category.
type statusCounts struct {
	Staged     int `json:"staged"`
	Unstaged   int `json:"unstaged"`
	Untracked  int `json:"untracked"`
	Conflicted int `json:"conflicted"`
}

// statusReport is the full machine-readable status document.
type statusReport struct {
	Branch     string        `json:"branch"`
	Upstream   string        `json:"upstream"`
	Ahead      int           `json:"ahead"`
	Behind     int           `json:"behind"`
	Detached   bool          `json:"detached"`
	Clean      bool          `json:"clean"`
	Staged     []statusEntry `json:"staged"`
	Unstaged   []statusEntry `json:"unstaged"`
	Untracked  []string      `json:"untracked"`
	Conflicted []string      `json:"conflicted"`
	Counts     statusCounts  `json:"counts"`
}

// newStatusCmd creates the status command, which prints a summary of the
// current repository state for humans or, with --json, for scripts.
func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Print a summary of the working tree status",
		Long: `Print a summary of the current repository: branch, tracking
information (ahead/behind), and the staged, unstaged, untracked, and
conflicted files.

Use --json for a stable, machine-readable document that scripts and other
tools can parse.`,
		Args: cobra.NoArgs,
		RunE: runStatus,
	}
	cmd.Flags().Bool("json", false, "Output the status as JSON")
	return cmd
}

func runStatus(cmd *cobra.Command, _ []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	gc, err := git.NewClient(cwd)
	if err != nil {
		return fmt.Errorf("git client: %w", err)
	}

	ctx := context.Background()
	isRepo, err := gc.IsRepo(ctx)
	if err != nil {
		return fmt.Errorf("check repository: %w", err)
	}
	if !isRepo {
		return fmt.Errorf("not a git repository: %s", cwd)
	}

	branch, files, err := gc.StatusWithBranch(ctx)
	if err != nil {
		return fmt.Errorf("read status: %w", err)
	}

	report := buildStatusReport(branch, files)

	asJSON, _ := cmd.Flags().GetBool("json")
	if asJSON {
		return writeStatusJSON(cmd.OutOrStdout(), report)
	}
	writeStatusText(cmd.OutOrStdout(), report)
	return nil
}

// buildStatusReport classifies file entries into staged, unstaged, untracked,
// and conflicted buckets. A file may be both staged and unstaged (for example
// an index change plus a later worktree edit), in which case it appears in both.
const detachedHead = "(detached)"

func buildStatusReport(branch git.StatusBranch, files []git.FileStatus) statusReport {
	head := branch.Head
	detached := head == "" || head == detachedHead

	report := statusReport{
		Branch:     head,
		Upstream:   branch.Upstream,
		Ahead:      branch.Ahead,
		Behind:     branch.Behind,
		Detached:   detached,
		Staged:     []statusEntry{},
		Unstaged:   []statusEntry{},
		Untracked:  []string{},
		Conflicted: []string{},
	}

	for _, f := range files {
		switch f.StagedStatus {
		case git.StatusUntracked:
			report.Untracked = append(report.Untracked, f.Path)
		case git.StatusConflict:
			report.Conflicted = append(report.Conflicted, f.Path)
		default:
			if isChange(f.StagedStatus) {
				report.Staged = append(report.Staged, statusEntry{Path: f.Path, Status: f.StagedStatus.String()})
			}
			if isChange(f.WorktreeStatus) {
				report.Unstaged = append(report.Unstaged, statusEntry{Path: f.Path, Status: f.WorktreeStatus.String()})
			}
		}
	}

	report.Counts = statusCounts{
		Staged:     len(report.Staged),
		Unstaged:   len(report.Unstaged),
		Untracked:  len(report.Untracked),
		Conflicted: len(report.Conflicted),
	}
	report.Clean = report.Counts.Staged == 0 &&
		report.Counts.Unstaged == 0 &&
		report.Counts.Untracked == 0 &&
		report.Counts.Conflicted == 0

	return report
}

// isChange reports whether a status code represents an actual change to the
// index or worktree (as opposed to unmodified).
func isChange(code git.StatusCode) bool {
	return code != git.StatusUnmodified
}

func writeStatusJSON(w io.Writer, report statusReport) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		return fmt.Errorf("encode status: %w", err)
	}
	return nil
}

func writeStatusText(w io.Writer, report statusReport) {
	if report.Detached {
		fmt.Fprintln(w, "HEAD detached")
	} else {
		fmt.Fprintf(w, "On branch %s\n", report.Branch)
	}

	if report.Upstream != "" {
		track := ""
		switch {
		case report.Ahead > 0 && report.Behind > 0:
			track = fmt.Sprintf(" [ahead %d, behind %d]", report.Ahead, report.Behind)
		case report.Ahead > 0:
			track = fmt.Sprintf(" [ahead %d]", report.Ahead)
		case report.Behind > 0:
			track = fmt.Sprintf(" [behind %d]", report.Behind)
		}
		fmt.Fprintf(w, "Tracking %s%s\n", report.Upstream, track)
	}

	if report.Clean {
		fmt.Fprintln(w, "Working tree clean")
		return
	}

	writeStatusSection(w, "Staged", report.Staged)
	writeStatusSection(w, "Unstaged", report.Unstaged)
	if len(report.Conflicted) > 0 {
		fmt.Fprintf(w, "Conflicted (%d):\n", len(report.Conflicted))
		for _, p := range report.Conflicted {
			fmt.Fprintf(w, "  U %s\n", p)
		}
	}
	if len(report.Untracked) > 0 {
		fmt.Fprintf(w, "Untracked (%d):\n", len(report.Untracked))
		for _, p := range report.Untracked {
			fmt.Fprintf(w, "  ? %s\n", p)
		}
	}
}

func writeStatusSection(w io.Writer, label string, entries []statusEntry) {
	if len(entries) == 0 {
		return
	}
	fmt.Fprintf(w, "%s (%d):\n", label, len(entries))
	for _, e := range entries {
		fmt.Fprintf(w, "  %s %s\n", e.Status, e.Path)
	}
}
