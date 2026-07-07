package git

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Log returns commits from git log.
func (c *Client) Log(ctx context.Context, opts LogOpts) ([]Commit, error) {
	bodyFormat := "%b"
	if opts.OmitBody {
		bodyFormat = ""
	}

	// Build the format string: fields separated by \x1e (RS).
	// Each commit record is terminated by \x1f (US) so that multi-line
	// body text (%b) does not break record boundaries.
	format := strings.Join([]string{
		"%H",  // full hash
		"%h",  // short hash
		"%an", // author name
		"%ae", // author email
		"%aI", // author date ISO 8601
		"%s",  // subject
		bodyFormat,
		"%P",  // parent hashes
		"%D",  // ref names
		"%G?", // signature verification status
		"%GS", // signer identity
	}, FieldSep) + RecordEnd

	args := []string{"log", "--format=" + format}

	if opts.MaxCount > 0 {
		args = append(args, fmt.Sprintf("--max-count=%d", opts.MaxCount))
	}
	if opts.Skip > 0 {
		args = append(args, fmt.Sprintf("--skip=%d", opts.Skip))
	}
	if opts.Since != "" {
		if err := ValidateArg(opts.Since); err != nil {
			return nil, fmt.Errorf("log since: %w", err)
		}
		args = append(args, "--since="+opts.Since)
	}
	if opts.Until != "" {
		if err := ValidateArg(opts.Until); err != nil {
			return nil, fmt.Errorf("log until: %w", err)
		}
		args = append(args, "--until="+opts.Until)
	}
	if opts.Author != "" {
		if err := ValidateArg(opts.Author); err != nil {
			return nil, fmt.Errorf("log author: %w", err)
		}
		args = append(args, "--author="+opts.Author)
	}
	if opts.Grep != "" {
		if err := ValidateArg(opts.Grep); err != nil {
			return nil, fmt.Errorf("log grep: %w", err)
		}
		args = append(args, "--grep="+opts.Grep)
	}
	if opts.All {
		args = append(args, "--all")
	}
	if opts.Ref != "" {
		if err := ValidateRef(opts.Ref); err != nil {
			return nil, fmt.Errorf("log ref: %w", err)
		}
		args = append(args, opts.Ref)
	}
	if opts.Path != "" {
		if err := ValidatePath(opts.Path); err != nil {
			return nil, fmt.Errorf("log path: %w", err)
		}
		args = append(args, "--", opts.Path)
	}

	out, err := c.run(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("log: %w", err)
	}
	return parseLogOutput(out, FieldSep)
}

// parseLogOutput parses the custom-formatted git log output.
// Each commit record is delimited by \x1f (US). Within each record,
// fields are separated by \x1e (RS). This allows body text containing
// newlines to be parsed correctly.
func parseLogOutput(output, sep string) ([]Commit, error) { //nolint:unparam // separator kept as parameter for testability
	if strings.TrimSpace(output) == "" {
		return []Commit{}, nil
	}

	commits := make([]Commit, 0)

	// Split by \x1f (US) to isolate individual commit records.
	// Each record may span multiple lines when the body (%b) contains
	// newlines, so we must NOT split by \n first.
	records := strings.Split(output, "\x1f")

	for _, record := range records {
		record = strings.TrimSpace(record)
		if record == "" {
			continue
		}

		fields := strings.Split(record, sep)
		if len(fields) < 9 {
			continue // skip malformed records
		}

		date, err := time.Parse(time.RFC3339, strings.TrimSpace(fields[4]))
		if err != nil {
			date = time.Time{}
		}

		parents := make([]string, 0)
		if p := strings.TrimSpace(fields[7]); p != "" {
			parents = strings.Fields(p)
		}

		refs := make([]string, 0)
		if r := strings.TrimSpace(fields[8]); r != "" {
			for _, ref := range strings.Split(r, ", ") {
				ref = strings.TrimSpace(ref)
				if ref != "" {
					refs = append(refs, ref)
				}
			}
		}

		// Signature fields are appended after the ref names. Older callers
		// (and unit tests) may pass records without them, so read defensively.
		var sig SignatureStatus
		var signer string
		if len(fields) >= 11 {
			sig = ParseSignatureStatus(strings.TrimSpace(fields[9]))
			signer = strings.TrimSpace(fields[10])
		}

		commits = append(commits, Commit{
			Hash:        strings.TrimSpace(fields[0]),
			ShortHash:   strings.TrimSpace(fields[1]),
			Author:      strings.TrimSpace(fields[2]),
			AuthorEmail: strings.TrimSpace(fields[3]),
			Date:        date,
			Subject:     strings.TrimSpace(fields[5]),
			Body:        strings.TrimSpace(fields[6]),
			Parents:     parents,
			Refs:        refs,
			Signature:   sig,
			Signer:      signer,
		})
	}
	return commits, nil
}
