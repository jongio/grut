package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/jongio/grut/internal/crashlog"
	"github.com/jongio/grut/internal/panels"
	"github.com/spf13/cobra"
)

const (
	githubRepo = "jongio/grut"
	issueURL   = "https://github.com/" + githubRepo + "/issues/new"
	maxURLLen  = 4000

	reportJSONFlag = "json"
)

type crashReportSummary struct {
	ID         string `json:"id"`
	Timestamp  string `json:"timestamp"`
	Version    string `json:"version"`
	GoVersion  string `json:"go_version"`
	OS         string `json:"os"`
	Arch       string `json:"arch"`
	Terminal   string `json:"terminal"`
	PanicValue string `json:"panic_value"`
	Context    string `json:"context"`
}

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
	cmd.Flags().Bool(reportJSONFlag, false, "Print list output as JSON")

	return cmd
}

func runReport(cmd *cobra.Command, _ []string) error {
	doClear, _ := cmd.Flags().GetBool("clear")
	if doClear {
		return runReportClear()
	}

	list, _ := cmd.Flags().GetBool("list")
	jsonOutput, _ := cmd.Flags().GetBool(reportJSONFlag)
	if list {
		return runReportList(cmd, jsonOutput)
	}

	showID, _ := cmd.Flags().GetString("show")
	if showID != "" {
		return runReportShow(showID)
	}

	if jsonOutput {
		return fmt.Errorf("--json can only be used with --list")
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

func runReportList(cmd *cobra.Command, jsonOutput bool) error {
	reports, err := crashlog.List()
	if err != nil {
		return fmt.Errorf("listing crash reports: %w", err)
	}
	if jsonOutput {
		return writeCrashReportListJSON(cmd.OutOrStdout(), reports)
	}

	w := cmd.OutOrStdout()
	if len(reports) == 0 {
		fmt.Fprintln(w, "No crash reports found.")
		return nil
	}

	fmt.Fprintf(w, "Crash Reports (%d found)\n\n", len(reports))
	fmt.Fprintf(w, "  %-12s %-22s %-42s %s\n", "ID", "Timestamp", "Panic", "Context")
	for _, r := range reports {
		id := r.ID
		if runes := []rune(id); len(runes) > 8 {
			id = string(runes[:8]) + ".."
		}
		ts := r.Timestamp.Format("2006-01-02 15:04:05")
		pv := truncate(r.PanicValue, 50)
		fmt.Fprintf(w, "  %-12s %-22s %-42s %s\n", id, ts, pv, r.Context)
	}
	return nil
}

func writeCrashReportListJSON(w io.Writer, reports []*crashlog.CrashReport) error {
	summaries := make([]crashReportSummary, 0, len(reports))
	for _, r := range reports {
		summaries = append(summaries, crashReportSummary{
			ID:         r.ID,
			Timestamp:  r.Timestamp.Format(time.RFC3339),
			Version:    r.Version,
			GoVersion:  r.GoVersion,
			OS:         r.OS,
			Arch:       r.Arch,
			Terminal:   r.Terminal,
			PanicValue: r.PanicValue,
			Context:    r.Context,
		})
	}
	return json.NewEncoder(w).Encode(summaries)
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
		return panels.OpenInBrowser(context.Background(), minimalURL)
	}

	if err := panels.OpenInBrowser(context.Background(), fullURL); err != nil {
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
