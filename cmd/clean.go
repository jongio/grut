package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/jongio/grut/internal/config"
	"github.com/spf13/cobra"
)

// dataDirFunc resolves grut's data directory. It is injected so tests can
// point the clean command at a temporary directory instead of the real one.
type dataDirFunc func() string

// cleanTarget is a directory that "grut clean" is allowed to remove.
type cleanTarget struct {
	label string
	path  string
}

type cleanTargetReport struct {
	Label   string `json:"label"`
	Path    string `json:"path"`
	Exists  bool   `json:"exists"`
	Bytes   int64  `json:"bytes"`
	Removed bool   `json:"removed"`
}

type cleanReport struct {
	Forced       bool                `json:"forced"`
	TotalBytes   int64               `json:"total_bytes"`
	PresentCount int                 `json:"present_count"`
	Targets      []cleanTargetReport `json:"targets"`
}

// cleanTargets returns the transient directories clean manages, rooted at
// dataDir. Only regenerable state is included: saved sessions and watchdog
// diagnostics. The user config file, installed extensions, crash reports
// (owned by "grut report --clear"), the MCP audit log, and the first-run
// marker all live elsewhere under the data or config directory and are left
// untouched.
func cleanTargets(dataDir string) []cleanTarget {
	return []cleanTarget{
		{label: "sessions", path: filepath.Join(dataDir, "sessions")},
		{label: "diagnostics", path: filepath.Join(dataDir, "diagnostics")},
	}
}

func newCleanCmd() *cobra.Command {
	return newCleanCmdWithDeps(config.DataDir)
}

func newCleanCmdWithDeps(dataDir dataDirFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Remove grut's transient session and diagnostic data",
		Long: `Remove grut's transient data: saved session state and watchdog
diagnostic logs.

Without --force, clean previews what would be removed and how much disk
space it would reclaim. Pass --force to delete.

clean never touches your config file, installed extensions, crash reports
(use "grut report --clear" for those), or the MCP audit log.`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			force, _ := cmd.Flags().GetBool("force")
			asJSON, _ := cmd.Flags().GetBool("json")
			check, _ := cmd.Flags().GetBool("check")
			return runClean(cmd.OutOrStdout(), cleanTargets(dataDir()), force, asJSON, check)
		},
	}
	cmd.Flags().Bool("force", false, "Delete the transient data (without this flag, clean only previews)")
	cmd.Flags().Bool("json", false, "Print the clean report as JSON")
	cmd.Flags().Bool("check", false, "Exit with an error when transient data is present")
	return cmd
}

// runClean scans each target, then either previews it or removes it. A
// missing target is reported and skipped rather than treated as an error, so
// running clean on a fresh machine is a no-op.
func runClean(out io.Writer, targets []cleanTarget, force, asJSON, check bool) error {
	force = force && !check
	report := cleanReport{
		Forced:  force,
		Targets: make([]cleanTargetReport, 0, len(targets)),
	}
	for _, t := range targets {
		size, exists, err := dirSize(t.path)
		if err != nil {
			return fmt.Errorf("scanning %s: %w", t.label, err)
		}
		targetReport := cleanTargetReport{
			Label:  t.label,
			Path:   t.path,
			Exists: exists,
			Bytes:  size,
		}
		if !exists {
			report.Targets = append(report.Targets, targetReport)
			if !asJSON {
				fmt.Fprintf(out, "  %-12s not present\n", t.label)
			}
			continue
		}
		report.PresentCount++
		report.TotalBytes += size
		if force {
			if err := os.RemoveAll(t.path); err != nil {
				return fmt.Errorf("removing %s: %w", t.label, err)
			}
			targetReport.Removed = true
			report.Targets = append(report.Targets, targetReport)
			if !asJSON {
				fmt.Fprintf(out, "  %-12s removed  %s\n", t.label, humanizeBytes(size))
			}
			continue
		}
		report.Targets = append(report.Targets, targetReport)
		if !asJSON {
			fmt.Fprintf(out, "  %-12s %-10s %s\n", t.label, humanizeBytes(size), t.path)
		}
	}

	if asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return err
		}
		if check && report.PresentCount > 0 {
			return errCleanTargetsPresent
		}
		return nil
	}

	fmt.Fprintln(out)
	if force {
		fmt.Fprintf(out, "Reclaimed %s.\n", humanizeBytes(report.TotalBytes))
		return nil
	}
	if report.PresentCount == 0 {
		fmt.Fprintln(out, "Nothing to clean.")
		return nil
	}
	fmt.Fprintf(out, "%s across %d location(s). Run with --force to delete.\n", humanizeBytes(report.TotalBytes), report.PresentCount)
	if check {
		return errCleanTargetsPresent
	}
	return nil
}

var errCleanTargetsPresent = errors.New("transient data is present")

// dirSize returns the total size of all files under path. The second return
// value reports whether path exists; a missing path yields (0, false, nil).
func dirSize(path string) (int64, bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, false, nil
		}
		return 0, false, err
	}
	if !info.IsDir() {
		return info.Size(), true, nil
	}

	var total int64
	walkErr := filepath.WalkDir(path, func(_ string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		total += fi.Size()
		return nil
	})
	if walkErr != nil {
		return 0, true, walkErr
	}
	return total, true, nil
}

// humanizeBytes formats a byte count using binary units (KiB, MiB, ...).
func humanizeBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for size := n / unit; size >= unit; size /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
