package git

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// StashList returns all stash entries.
func (c *Client) StashList(ctx context.Context) ([]StashEntry, error) {
	out, err := c.run(ctx, "stash", "list", "--format=%H"+FieldSep+"%gd"+FieldSep+"%gs"+FieldSep+"%cI")
	if err != nil {
		return nil, fmt.Errorf("stash list: %w", err)
	}
	return parseStashList(out), nil
}

// parseStashList parses "git stash list" output formatted as
// hash<FieldSep>ref<FieldSep>message[<FieldSep>date].
func parseStashList(output string) []StashEntry {
	entries := make([]StashEntry, 0)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}

		fields := strings.SplitN(line, FieldSep, 4)
		if len(fields) < 3 {
			continue
		}

		idx, idxErr := parseStashIndex(fields[1])
		if idxErr != nil {
			continue
		}
		var date time.Time
		if len(fields) >= 4 {
			date, _ = time.Parse(time.RFC3339, strings.TrimSpace(fields[3]))
		}
		msg := fields[2]
		var branch string
		// Extract branch from message: "On <branch>: ..." or "WIP on <branch>: ..."
		if onIdx := strings.Index(msg, " on "); onIdx >= 0 {
			rest := msg[onIdx+4:] // skip " on "
			if colonIdx := strings.Index(rest, ":"); colonIdx >= 0 {
				branch = strings.TrimSpace(rest[:colonIdx])
			}
		}
		// Also handle "On <branch>:" at the start
		if strings.HasPrefix(msg, "On ") {
			rest := msg[3:]
			if colonIdx := strings.Index(rest, ":"); colonIdx >= 0 {
				branch = strings.TrimSpace(rest[:colonIdx])
			}
		}
		entries = append(entries, StashEntry{
			Hash:    fields[0],
			Index:   idx,
			Message: msg,
			Branch:  branch,
			Date:    date,
		})
	}
	return entries
}

// parseStashIndex extracts the numeric index from "stash@{N}".
func parseStashIndex(ref string) (int, error) {
	// ref looks like "stash@{0}"
	start := strings.Index(ref, "{")
	end := strings.Index(ref, "}")
	if start < 0 || end < 0 || end <= start+1 {
		return -1, fmt.Errorf("invalid stash ref format: %q", ref)
	}
	n, err := strconv.Atoi(ref[start+1 : end])
	if err != nil {
		return -1, fmt.Errorf("parse stash index from %q: %w", ref, err)
	}
	return n, nil
}

// StashShow returns the diff output for the stash entry at the given index.
func (c *Client) StashShow(ctx context.Context, index int) (string, error) {
	ref := fmt.Sprintf("stash@{%d}", index)
	out, err := c.run(ctx, "stash", "show", "-p", ref)
	if err != nil {
		return "", fmt.Errorf("stash show: %w", err)
	}
	return out, nil
}

// StashPush creates a new stash entry.
func (c *Client) StashPush(ctx context.Context, opts StashOpts) error {
	return c.queue.Exec(ctx, func() error {
		args := []string{"stash", "push"}
		if opts.Message != "" {
			args = append(args, "-m", opts.Message)
		}
		if opts.KeepIndex {
			args = append(args, "--keep-index")
		}
		if opts.Staged {
			args = append(args, "--staged")
		}
		if len(opts.Paths) > 0 {
			for _, p := range opts.Paths {
				if err := ValidatePath(p); err != nil {
					return fmt.Errorf("stash push path: %w", err)
				}
			}
			args = append(args, "--")
			args = append(args, opts.Paths...)
		}

		_, err := c.run(ctx, args...)
		if err != nil {
			return fmt.Errorf("stash push: %w", err)
		}
		c.cache.Invalidate()
		return nil
	})
}

// StashPop applies and removes the stash entry at the given index.
func (c *Client) StashPop(ctx context.Context, index int) error {
	return c.queue.Exec(ctx, func() error {
		ref := fmt.Sprintf("stash@{%d}", index)
		_, err := c.run(ctx, "stash", "pop", ref)
		if err != nil {
			return fmt.Errorf("stash pop: %w", err)
		}
		c.cache.Invalidate()
		return nil
	})
}

// StashApply applies (but does not remove) the stash entry at the given index.
func (c *Client) StashApply(ctx context.Context, index int) error {
	return c.queue.Exec(ctx, func() error {
		ref := fmt.Sprintf("stash@{%d}", index)
		_, err := c.run(ctx, "stash", "apply", ref)
		if err != nil {
			return fmt.Errorf("stash apply: %w", err)
		}
		c.cache.Invalidate()
		return nil
	})
}

// StashDrop removes the stash entry at the given index without applying it.
func (c *Client) StashDrop(ctx context.Context, index int) error {
	return c.queue.Exec(ctx, func() error {
		ref := fmt.Sprintf("stash@{%d}", index)
		_, err := c.run(ctx, "stash", "drop", ref)
		if err != nil {
			return fmt.Errorf("stash drop: %w", err)
		}
		return nil
	})
}
