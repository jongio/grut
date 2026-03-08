package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/jongio/grut/internal/crashlog"
	"github.com/jongio/grut/internal/panels"
	"github.com/spf13/cobra"
)

const (
	githubRepo = "jongio/grut"
	issueURL   = "https://github.com/" + githubRepo + "/issues/new"
	maxURLLen  = 4000
)

// newReportCmd creates the report command for viewing crash reports
// and filing GitHub issues.
func newReportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report",
		Short: "View crash reports and file GitHub issues",
		Long: `View, list, and manage crash reports captured by grut.

When run without flags (or with --latest), opens a browser to file a
pre-filled GitHub issue for the most recent crash. Use --list to see
all stored reports, --show to inspect one, or --clear to remove them all.`,
		RunE: runReport,
	}

	cmd.Flags().Bool("latest", false, "Open a GitHub issue for the most recent crash")
	cmd.Flags().Bool("list", false, "List all crash reports")
	cmd.Flags().String("show", "", "Show a specific crash report by ID")
	cmd.Flags().Bool("clear", false, "Clear all crash reports")
	cmd.Flags().Bool("no-browser", false, "Print the GitHub issue URL instead of opening the browser")

	return cmd
}

func runReport(cmd *cobra.Command, _ []string) error {
	doClear, _ := cmd.Flags().GetBool("clear")
	if doClear {
		return runReportClear()
	}

	list, _ := cmd.Flags().GetBool("list")
	if list {
		return runReportList()
	}

	showID, _ := cmd.Flags().GetString("show")
	if showID != "" {
		return runReportShow(showID)
	}

	// Default / --latest: open issue for most recent crash.
	noBrowser, _ := cmd.Flags().GetBool("no-browser")
	return runReportLatest(noBrowser)
}

func runReportClear() error {
	n, err := crashlog.Clear()
	if err != nil {
		return fmt.Errorf("clearing crash reports: %w", err)
	}
	fmt.Printf("Cleared %d crash report(s).\n", n)
	return nil
}

func runReportList() error {
	reports, err := crashlog.List()
	if err != nil {
		return fmt.Errorf("listing crash reports: %w", err)
	}
	if len(reports) == 0 {
		fmt.Println("No crash reports found.")
		return nil
	}

	fmt.Printf("Crash Reports (%d found)\n\n", len(reports))
	fmt.Printf("  %-12s %-22s %-42s %s\n", "ID", "Timestamp", "Panic", "Context")
	for _, r := range reports {
		id := r.ID
		if runes := []rune(id); len(runes) > 8 {
			id = string(runes[:8]) + ".."
		}
		ts := r.Timestamp.Format("2006-01-02 15:04:05")
		pv := truncate(r.PanicValue, 50)
		fmt.Printf("  %-12s %-22s %-42s %s\n", id, ts, pv, r.Context)
	}
	return nil
}

func runReportShow(id string) error {
	report, err := crashlog.Read(id)
	if err != nil {
		return fmt.Errorf("reading crash report %q: %w", id, err)
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("formatting crash report: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

func runReportLatest(noBrowser bool) error {
	reports, err := crashlog.List()
	if err != nil {
		return fmt.Errorf("listing crash reports: %w", err)
	}
	if len(reports) == 0 {
		fmt.Println("No crash reports found.")
		return nil
	}

	report := reports[0]
	fullURL := buildIssueURL(report)

	if noBrowser {
		fmt.Println(fullURL)
		return nil
	}

	shortID := truncate(report.ID, 8)
	fmt.Printf("Opening GitHub issue for crash report %s...\n\n", shortID)
	fmt.Printf("Title: %s\n\n", crashlog.FormatGitHubIssueTitle(report))

	if len(fullURL) >= maxURLLen {
		// URL too long for browsers; open a minimal URL and print the body.
		minimalURL := issueURL + "?labels=" + url.QueryEscape("crash")
		body := crashlog.FormatGitHubIssueBody(report)
		fmt.Println(body)
		fmt.Println()
		fmt.Println("The crash report is too long for a URL. The report body has been printed above -- copy and paste it into the issue form.")
		return panels.OpenInBrowser(minimalURL)
	}

	if err := panels.OpenInBrowser(fullURL); err != nil {
		return fmt.Errorf("opening browser: %w", err)
	}
	fmt.Println("Browser opened. Review the issue details before submitting.")
	return nil
}

func buildIssueURL(r *crashlog.CrashReport) string {
	title := url.QueryEscape(crashlog.FormatGitHubIssueTitle(r))
	body := url.QueryEscape(crashlog.FormatGitHubIssueBody(r))
	labels := url.QueryEscape("crash")
	return issueURL + "?title=" + title + "&body=" + body + "&labels=" + labels
}

func truncate(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes])
}
