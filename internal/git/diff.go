package git

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Diff returns file diffs for the repository.
func (c *Client) Diff(ctx context.Context, opts DiffOpts) ([]FileDiff, error) {
	args := []string{"diff"}

	if opts.Staged {
		args = append(args, "--cached")
	}

	if opts.IgnoreAll {
		args = append(args, "-w")
	}

	if opts.Context > 0 {
		args = append(args, fmt.Sprintf("-U%d", opts.Context))
	}

	if opts.NameOnly {
		args = append(args, "--name-only")
	}

	if opts.StatOnly {
		args = append(args, "--stat")
	}

	if opts.CommitA != "" {
		if err := ValidateRef(opts.CommitA); err != nil {
			return nil, fmt.Errorf("diff commitA: %w", err)
		}
		args = append(args, opts.CommitA)
	}
	if opts.CommitB != "" {
		if err := ValidateRef(opts.CommitB); err != nil {
			return nil, fmt.Errorf("diff commitB: %w", err)
		}
		args = append(args, opts.CommitB)
	}

	if opts.Path != "" {
		if err := ValidatePath(opts.Path); err != nil {
			return nil, fmt.Errorf("diff path: %w", err)
		}
		args = append(args, "--", opts.Path)
	}

	out, err := c.run(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("diff: %w", err)
	}

	return parseDiff(out)
}

// parseDiff parses unified diff output into FileDiff entries.
func parseDiff(output string) ([]FileDiff, error) {
	if strings.TrimSpace(output) == "" {
		return []FileDiff{}, nil
	}

	diffs := make([]FileDiff, 0)
	lines := strings.Split(output, "\n")
	i := 0

	for i < len(lines) {
		line := strings.TrimRight(lines[i], "\r")

		// Look for diff header: "diff --git a/... b/..."
		if !strings.HasPrefix(line, "diff --git ") {
			i++
			continue
		}

		fd := FileDiff{}

		// Extract paths from diff header.
		fd.Path, fd.OldPath = parseDiffHeader(line)
		i++

		// Consume extended headers (index, old mode, new mode, similarity, etc.)
		for i < len(lines) {
			line = strings.TrimRight(lines[i], "\r")
			switch {
			case strings.HasPrefix(line, "index "),
				strings.HasPrefix(line, "old mode"),
				strings.HasPrefix(line, "new mode"),
				strings.HasPrefix(line, "new file"),
				strings.HasPrefix(line, "deleted file"),
				strings.HasPrefix(line, "similarity index"),
				strings.HasPrefix(line, "rename from"),
				strings.HasPrefix(line, "rename to"),
				strings.HasPrefix(line, "copy from"),
				strings.HasPrefix(line, "copy to"):
				if strings.HasPrefix(line, "rename from ") {
					fd.OldPath = strings.TrimPrefix(line, "rename from ")
				}
				if strings.HasPrefix(line, "rename to ") {
					fd.Path = strings.TrimPrefix(line, "rename to ")
				}
				i++
				continue
			case line == "Binary files differ" ||
				strings.HasPrefix(line, "Binary files"):
				fd.IsBinary = true
				i++
				continue
			default:
				// Not an extended header — stop.
			}
			break
		}

		// Parse --- and +++ lines.
		if i < len(lines) && strings.HasPrefix(strings.TrimRight(lines[i], "\r"), "--- ") {
			i++
		}
		if i < len(lines) && strings.HasPrefix(strings.TrimRight(lines[i], "\r"), "+++ ") {
			i++
		}

		// Parse hunks.
		for i < len(lines) {
			line = strings.TrimRight(lines[i], "\r")
			if strings.HasPrefix(line, "diff --git ") {
				break // next file
			}
			if strings.HasPrefix(line, "@@ ") {
				hunk, nextI := parseHunk(lines, i)
				fd.Hunks = append(fd.Hunks, hunk)
				i = nextI
			} else {
				i++
			}
		}

		diffs = append(diffs, fd)
	}

	return diffs, nil
}

// parseDiffHeader extracts file paths from "diff --git a/path b/path".
func parseDiffHeader(line string) (path, oldPath string) {
	// Remove "diff --git " prefix.
	rest := strings.TrimPrefix(line, "diff --git ")

	// Find the split point: "a/<path> b/<path>".
	// The tricky part is paths with spaces. We look for " b/" as separator.
	idx := strings.Index(rest, " b/")
	if idx < 0 {
		// Fallback: split on space.
		parts := strings.SplitN(rest, " ", 2)
		if len(parts) == 2 {
			return strings.TrimPrefix(parts[1], "b/"), strings.TrimPrefix(parts[0], "a/")
		}
		return rest, ""
	}

	aPath := strings.TrimPrefix(rest[:idx], "a/")
	bPath := strings.TrimPrefix(rest[idx+1:], "b/")
	return bPath, aPath
}

// parseHunk parses a single diff hunk starting at the @@ line.
// Returns the parsed hunk and the index of the line following the hunk.
func parseHunk(lines []string, startIdx int) (Hunk, int) {
	line := strings.TrimRight(lines[startIdx], "\r")

	h := Hunk{Header: line}
	h.OldStart, h.OldLines, h.NewStart, h.NewLines = parseHunkHeader(line)

	i := startIdx + 1
	oldLine := h.OldStart
	newLine := h.NewStart

	for i < len(lines) {
		line = strings.TrimRight(lines[i], "\r")

		// Stop at next hunk, next file, or empty line at EOF.
		if strings.HasPrefix(line, "@@") ||
			strings.HasPrefix(line, "diff --git ") {
			break
		}

		switch {
		case strings.HasPrefix(line, "+"):
			h.Lines = append(h.Lines, DiffLine{
				Type:    DiffLineAdded,
				Content: line[1:],
				NewLine: newLine,
			})
			newLine++
		case strings.HasPrefix(line, "-"):
			h.Lines = append(h.Lines, DiffLine{
				Type:    DiffLineRemoved,
				Content: line[1:],
				OldLine: oldLine,
			})
			oldLine++
		case strings.HasPrefix(line, " "):
			h.Lines = append(h.Lines, DiffLine{
				Type:    DiffLineContext,
				Content: line[1:],
				OldLine: oldLine,
				NewLine: newLine,
			})
			oldLine++
			newLine++
		case line == "\\ No newline at end of file":
			h.NoNewlineEOF = true
		default:
			// Unknown line format — stop parsing this hunk.
			i++
			continue
		}

		i++
	}

	return h, i
}

// parseHunkHeader extracts line numbers from "@@ -old,count +new,count @@ ...".
func parseHunkHeader(header string) (oldStart, oldLines, newStart, newLines int) {
	// Find the range between @@ markers.
	start := strings.Index(header, "@@")
	if start < 0 {
		return
	}
	rest := header[start+2:]
	end := strings.Index(rest, "@@")
	if end < 0 {
		end = len(rest)
	}
	rangeStr := strings.TrimSpace(rest[:end])

	// Split into old and new ranges.
	parts := strings.Fields(rangeStr)
	for _, p := range parts {
		switch {
		case strings.HasPrefix(p, "-"):
			oldStart, oldLines = parseRange(p[1:])
		case strings.HasPrefix(p, "+"):
			newStart, newLines = parseRange(p[1:])
		}
	}
	return
}

// parseRange parses "start,count" or "start" into two integers.
func parseRange(s string) (int, int) {
	if idx := strings.Index(s, ","); idx >= 0 {
		start, _ := strconv.Atoi(s[:idx])
		count, _ := strconv.Atoi(s[idx+1:])
		return start, count
	}
	start, _ := strconv.Atoi(s)
	return start, 1
}
