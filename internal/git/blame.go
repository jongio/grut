package git

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Blame returns line-by-line blame information for a file.
func (c *Client) Blame(ctx context.Context, path string) ([]BlameLine, error) {
	if err := ValidatePath(path); err != nil {
		return nil, fmt.Errorf("blame path: %w", err)
	}

	out, err := c.run(ctx, "blame", "--porcelain", "--", path)
	if err != nil {
		return nil, fmt.Errorf("blame: %w", err)
	}

	return parseBlame(out)
}

// parseBlame parses "git blame --porcelain" output.
//
// Format: blocks of headers followed by a tab-prefixed content line.
//
//	<hash> <orig_line> <final_line> [<num_lines>]
//	author <name>
//	author-time <timestamp>
//	...
//	\t<content>
func parseBlame(output string) ([]BlameLine, error) {
	result := make([]BlameLine, 0)
	lines := strings.Split(output, "\n")
	i := 0

	for i < len(lines) {
		line := strings.TrimRight(lines[i], "\r")
		if line == "" {
			i++
			continue
		}

		// Each block starts with: <hash> <orig_line> <final_line> [<num_lines>]
		fields := strings.Fields(line)
		if len(fields) < 3 {
			i++
			continue
		}

		bl := BlameLine{}
		bl.Hash = fields[0]

		lineNo, err := strconv.Atoi(fields[2])
		if err != nil {
			i++
			continue
		}
		bl.LineNo = lineNo
		i++

		// Read header lines until we hit the content line (starts with \t).
		for i < len(lines) {
			line = strings.TrimRight(lines[i], "\r")
			if strings.HasPrefix(line, "\t") {
				bl.Content = line[1:] // strip leading tab
				i++
				break
			}

			switch {
			case strings.HasPrefix(line, "author "):
				bl.Author = strings.TrimPrefix(line, "author ")
			case strings.HasPrefix(line, "author-time "):
				ts := strings.TrimPrefix(line, "author-time ")
				if unix, err := strconv.ParseInt(ts, 10, 64); err == nil {
					bl.Date = time.Unix(unix, 0)
				}
			}
			i++
		}

		result = append(result, bl)
	}
	return result, nil
}
