package review

import (
	"fmt"
	"strings"
)

// GenerateSummary produces a structured plain-text summary of all review
// decisions. The output is designed for pasting into PR descriptions or
// review comments.
func GenerateSummary(files []ReviewFile) string {
	if len(files) == 0 {
		return "# Review Summary\n\nNo changes to review."
	}

	// Categorise files: a file appears in a section if it has any hunks
	// in that state (a mixed file can appear in multiple sections).
	type fileEntry struct {
		file  ReviewFile
		count int // number of hunks in the relevant state
	}

	var approvedFiles, rejectedFiles, pendingFiles []fileEntry
	totalApproved, totalRejected, totalPending := 0, 0, 0

	for _, f := range files {
		approved, rejected, pending := 0, 0, 0
		for _, s := range f.HunkStates {
			switch s {
			case HunkApproved:
				approved++
			case HunkRejected:
				rejected++
			default:
				pending++
			}
		}
		totalApproved += approved
		totalRejected += rejected
		totalPending += pending

		if approved > 0 {
			approvedFiles = append(approvedFiles, fileEntry{f, approved})
		}
		if rejected > 0 {
			rejectedFiles = append(rejectedFiles, fileEntry{f, rejected})
		}
		if pending > 0 {
			pendingFiles = append(pendingFiles, fileEntry{f, pending})
		}
	}

	var b strings.Builder
	b.WriteString("# Review Summary\n")

	// --- Approved ---
	_, _ = fmt.Fprintf(&b, "\n## Approved (%d files, %d hunks)\n",
		len(approvedFiles), totalApproved)
	if len(approvedFiles) == 0 {
		b.WriteString("(none)\n")
	}
	for _, fe := range approvedFiles {
		_, _ = fmt.Fprintf(&b, "- %s: %d/%d hunks approved\n",
			fe.file.Path, fe.count, len(fe.file.HunkStates))
	}

	// --- Rejected ---
	_, _ = fmt.Fprintf(&b, "\n## Rejected (%d files, %d hunks)\n",
		len(rejectedFiles), totalRejected)
	if len(rejectedFiles) == 0 {
		b.WriteString("(none)\n")
	}
	for _, fe := range rejectedFiles {
		var details []string
		for i, s := range fe.file.HunkStates {
			if s == HunkRejected && i < len(fe.file.Diff.Hunks) {
				hunk := fe.file.Diff.Hunks[i]
				endLine := hunk.NewStart + hunk.NewLines - 1
				if endLine < hunk.NewStart {
					endLine = hunk.NewStart
				}
				details = append(details, fmt.Sprintf("hunk %d: lines %d-%d",
					i+1, hunk.NewStart, endLine))
			}
		}
		line := fmt.Sprintf("- %s: %d/%d hunks rejected",
			fe.file.Path, fe.count, len(fe.file.HunkStates))
		if len(details) > 0 {
			line += " (" + strings.Join(details, ", ") + ")"
		}
		b.WriteString(line + "\n")
	}

	// --- Pending ---
	_, _ = fmt.Fprintf(&b, "\n## Pending (%d files, %d hunks)\n",
		len(pendingFiles), totalPending)
	if len(pendingFiles) == 0 {
		b.WriteString("(none)\n")
	}
	for _, fe := range pendingFiles {
		_, _ = fmt.Fprintf(&b, "- %s: %d hunks unreviewed\n",
			fe.file.Path, fe.count)
	}

	return b.String()
}
