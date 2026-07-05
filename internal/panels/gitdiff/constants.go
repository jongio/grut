package gitdiff

const (
	severityError   = "error"
	severityWarning = "warning"
	severityInfo    = "info"
	severityHint    = "hint"
)

const (
	// defaultDiffContext matches git's default -U value (3 lines).
	defaultDiffContext = 3
	// contextStep is how many context lines each +/- keypress adjusts by.
	contextStep = 3
	// maxDiffContext caps context expansion to keep reloads reasonable.
	maxDiffContext = 100
)
