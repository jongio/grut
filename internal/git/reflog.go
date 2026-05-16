package git

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Reflog returns reflog entries for the given ref. If ref is empty, defaults
// to HEAD. Limit controls the maximum number of entries (0 = unlimited).
func (c *Client) Reflog(ctx context.Context, ref string, limit int) ([]ReflogEntry, error) {
	if ref == "" {
		ref = refHEAD
	}
	if err := ValidateRef(ref); err != nil {
		return nil, fmt.Errorf("reflog ref: %w", err)
	}

	format := strings.Join([]string{
		"%H",  // hash
		"%gs", // reflog subject
		"%gD", // reflog selector (e.g. HEAD@{0})
		"%gd", // reflog selector short
		"%aI", // author date ISO 8601
	}, FieldSep)

	args := []string{"reflog", cmdShow, ref, "--format=" + format}
	if limit > 0 {
		args = append(args, fmt.Sprintf("-n%d", limit))
	}

	out, err := c.run(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("reflog: %w", err)
	}

	return parseReflog(out, FieldSep), nil
}

// parseReflog parses custom-formatted reflog output.
func parseReflog(output, sep string) []ReflogEntry { //nolint:unparam // separator kept as parameter for testability
	entries := make([]ReflogEntry, 0)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}

		fields := strings.Split(line, sep)
		if len(fields) < 5 {
			continue
		}

		entry := ReflogEntry{
			Hash:    fields[0],
			Message: fields[1],
			Action:  fields[3],
		}

		if d, err := time.Parse(time.RFC3339, fields[4]); err == nil {
			entry.Date = d
		}

		entries = append(entries, entry)
	}
	return entries
}
