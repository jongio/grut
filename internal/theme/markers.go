package theme

// StatusState identifies a state that needs a non-color text marker.
type StatusState string

const (
	StatusStaged    StatusState = "staged"
	StatusUnstaged  StatusState = "unstaged"
	StatusUntracked StatusState = "untracked"
	StatusConflict  StatusState = "conflict"
	StatusAdded     StatusState = "added"
	StatusRemoved   StatusState = "removed"
	StatusInfo      StatusState = "info"
	StatusWarning   StatusState = "warning"
	StatusError     StatusState = "error"
	StatusSuccess   StatusState = "success"
)

// StatusMarker returns the stable marker for a status state.
func StatusMarker(state StatusState) string {
	switch state {
	case StatusStaged:
		return "A/M/D"
	case StatusUnstaged:
		return "M/D"
	case StatusUntracked:
		return "?"
	case StatusConflict:
		return "U"
	case StatusAdded:
		return "+"
	case StatusRemoved:
		return "-"
	case StatusInfo:
		return "ℹ"
	case StatusWarning:
		return "⚠"
	case StatusError:
		return "✗"
	case StatusSuccess:
		return "✓"
	default:
		return "•"
	}
}

// StatusLegendEntry is one row in the compact status legend.
type StatusLegendEntry struct {
	Label  string
	Marker string
}

// StatusLegend returns marker rows shared by help and tests.
func StatusLegend() []StatusLegendEntry {
	return []StatusLegendEntry{
		{Label: "staged", Marker: StatusMarker(StatusStaged)},
		{Label: "unstaged", Marker: StatusMarker(StatusUnstaged)},
		{Label: "untracked", Marker: StatusMarker(StatusUntracked)},
		{Label: "conflict", Marker: StatusMarker(StatusConflict)},
		{Label: "added", Marker: StatusMarker(StatusAdded)},
		{Label: "removed", Marker: StatusMarker(StatusRemoved)},
		{Label: "warning", Marker: StatusMarker(StatusWarning)},
		{Label: "error", Marker: StatusMarker(StatusError)},
	}
}
