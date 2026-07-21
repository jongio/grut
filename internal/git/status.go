package git

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Status returns the working tree status using porcelain v2 format.
func (c *Client) Status(ctx context.Context) ([]FileStatus, error) {
	out, err := c.run(ctx, "status", "--porcelain=v2", "--branch", "-uall")
	if err != nil {
		return nil, fmt.Errorf("status: %w", err)
	}
	return parseStatusV2(out)
}

// StatusWithBranch returns branch tracking metadata together with the working
// tree file statuses from a single git invocation. It is a convenience for
// callers that need both the branch header (name, upstream, ahead, behind) and
// the changed entries without running git status twice.
func (c *Client) StatusWithBranch(ctx context.Context) (StatusBranch, []FileStatus, error) {
	out, err := c.run(ctx, "status", "--porcelain=v2", "--branch", "-uall")
	if err != nil {
		return StatusBranch{}, nil, fmt.Errorf("status: %w", err)
	}
	files, err := parseStatusV2(out)
	if err != nil {
		return StatusBranch{}, nil, err
	}
	return ParseStatusBranch(out), files, nil
}

// parseStatusV2 parses git status --porcelain=v2 output into FileStatus entries.
//
// Porcelain v2 format:
//
//	# branch.oid <hash>
//	# branch.head <name>
//	# branch.upstream <upstream>
//	# branch.ab +<ahead> -<behind>
//	1 <XY> <sub> <mH> <mI> <mW> <hH> <hI> <path>
//	2 <XY> <sub> <mH> <mI> <mW> <hH> <hI> <X><score> <path>\t<origPath>
//	? <path>
//	! <path>
//	u <XY> <sub> <m1> <m2> <m3> <mW> <h1> <h2> <h3> <path>
func parseStatusV2(output string) ([]FileStatus, error) {
	result := make([]FileStatus, 0)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}

		switch {
		case strings.HasPrefix(line, "#"):
			// Header line — skip (branch info).
			continue

		case strings.HasPrefix(line, "1 "):
			// Ordinary changed entry.
			fs, err := parseOrdinaryEntry(line)
			if err != nil {
				return nil, fmt.Errorf("parse ordinary entry: %w", err)
			}
			result = append(result, fs)

		case strings.HasPrefix(line, "2 "):
			// Renamed or copied entry.
			fs, err := parseRenamedEntry(line)
			if err != nil {
				return nil, fmt.Errorf("parse renamed entry: %w", err)
			}
			result = append(result, fs)

		case strings.HasPrefix(line, "u "):
			// Unmerged entry (conflict).
			fs, err := parseUnmergedEntry(line)
			if err != nil {
				return nil, fmt.Errorf("parse unmerged entry: %w", err)
			}
			result = append(result, fs)

		case strings.HasPrefix(line, "? "):
			// Untracked file.
			path := line[2:]
			result = append(result, FileStatus{
				Path:           path,
				StagedStatus:   StatusUntracked,
				WorktreeStatus: StatusUntracked,
			})

		case strings.HasPrefix(line, "! "):
			// Ignored file.
			path := line[2:]
			result = append(result, FileStatus{
				Path:           path,
				StagedStatus:   StatusIgnored,
				WorktreeStatus: StatusIgnored,
			})
		}
	}
	return result, nil
}

// parseOrdinaryEntry parses a "1 <XY> ..." line.
// Format: 1 <XY> <sub> <mH> <mI> <mW> <hH> <hI> <path>
func parseOrdinaryEntry(line string) (FileStatus, error) {
	// "1 XY N... <remaining fields> <path>"
	// Fields are space-separated; the path is the last field.
	fields := strings.SplitN(line, " ", 9)
	if len(fields) < 9 {
		return FileStatus{}, fmt.Errorf("expected 9 fields, got %d: %q", len(fields), line)
	}
	xy := fields[1]
	if len(xy) < 2 {
		return FileStatus{}, fmt.Errorf("expected 2-char XY, got %q", xy)
	}
	return FileStatus{
		Path:           fields[8],
		StagedStatus:   mapStatusCode(xy[0]),
		WorktreeStatus: mapStatusCode(xy[1]),
	}, nil
}

// parseRenamedEntry parses a "2 <XY> ..." line (rename/copy).
// Format: 2 <XY> <sub> <mH> <mI> <mW> <hH> <hI> <X><score> <path>\t<origPath>
func parseRenamedEntry(line string) (FileStatus, error) {
	fields := strings.SplitN(line, " ", 10)
	if len(fields) < 10 {
		return FileStatus{}, fmt.Errorf("expected 10 fields, got %d: %q", len(fields), line)
	}
	xy := fields[1]
	if len(xy) < 2 {
		return FileStatus{}, fmt.Errorf("expected 2-char XY, got %q", xy)
	}
	// The last field contains "path\torigPath".
	pathField := fields[9]
	parts := strings.SplitN(pathField, "\t", 2)
	path := parts[0]
	origPath := ""
	if len(parts) == 2 {
		origPath = parts[1]
	}
	return FileStatus{
		Path:           path,
		StagedStatus:   mapStatusCode(xy[0]),
		WorktreeStatus: mapStatusCode(xy[1]),
		OrigPath:       origPath,
	}, nil
}

// mapStatusCode maps a porcelain v2 status byte to our StatusCode type.
// Porcelain v2 uses '.' to mean "not modified" whereas the standard
// representation uses ' ' (space). This function normalizes that.
func mapStatusCode(b byte) StatusCode {
	if b == '.' {
		return StatusUnmodified
	}
	return StatusCode(b)
}

// parseUnmergedEntry parses a "u <XY> ..." line (conflict).
// Format: u <XY> <sub> <m1> <m2> <m3> <mW> <h1> <h2> <h3> <path>
func parseUnmergedEntry(line string) (FileStatus, error) {
	fields := strings.SplitN(line, " ", 11)
	if len(fields) < 11 {
		return FileStatus{}, fmt.Errorf("expected 11 fields, got %d: %q", len(fields), line)
	}
	xy := fields[1]
	if len(xy) < 2 {
		return FileStatus{}, fmt.Errorf("expected 2-char XY, got %q", xy)
	}
	return FileStatus{
		Path:           fields[10],
		StagedStatus:   StatusConflict,
		WorktreeStatus: StatusConflict,
	}, nil
}

// IgnoredPaths returns paths ignored by .gitignore rules.
// It uses --ignored=matching so that ignored directories appear as single
// entries rather than recursing into every contained file.
func (c *Client) IgnoredPaths(ctx context.Context) ([]string, error) {
	out, err := c.run(ctx, "status", "--porcelain=v2", "--ignored=matching")
	if err != nil {
		return nil, fmt.Errorf("ignored paths: %w", err)
	}
	paths := make([]string, 0)
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "! ") {
			paths = append(paths, line[2:])
		}
	}
	return paths, nil
}

// StatusBranch holds parsed branch info from porcelain v2 header lines.
type StatusBranch struct {
	OID      string
	Head     string
	Upstream string
	Ahead    int
	Behind   int
}

// ParseStatusBranch extracts branch metadata from porcelain v2 output.
func ParseStatusBranch(output string) StatusBranch {
	var sb StatusBranch
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case strings.HasPrefix(line, "# branch.oid "):
			sb.OID = strings.TrimPrefix(line, "# branch.oid ")
		case strings.HasPrefix(line, "# branch.head "):
			sb.Head = strings.TrimPrefix(line, "# branch.head ")
		case strings.HasPrefix(line, "# branch.upstream "):
			sb.Upstream = strings.TrimPrefix(line, "# branch.upstream ")
		case strings.HasPrefix(line, "# branch.ab "):
			ab := strings.TrimPrefix(line, "# branch.ab ")
			parts := strings.Fields(ab)
			if len(parts) == 2 {
				sb.Ahead, _ = strconv.Atoi(strings.TrimPrefix(parts[0], "+"))
				behind := strings.TrimPrefix(parts[1], "-")
				sb.Behind, _ = strconv.Atoi(behind)
			}
		}
	}
	return sb
}
