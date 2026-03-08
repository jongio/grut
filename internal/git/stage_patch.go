package git

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	stdpath "path"
	"path/filepath"
	"strings"
)

// StageHunk stages a single hunk from an unstaged file by building a patch
// and applying it to the index via git apply --cached.
func (c *Client) StageHunk(ctx context.Context, path string, hunk Hunk) error {
	if err := ValidateRepoRelativePath(path); err != nil {
		return fmt.Errorf("stage hunk path: %w", err)
	}
	patch := buildHunkPatch(path, hunk)
	return c.queue.Exec(ctx, func() error {
		if err := c.runWithStdin(ctx, []byte(patch), "apply", "--cached"); err != nil {
			return fmt.Errorf("stage hunk: %w", err)
		}
		c.cache.Invalidate()
		return nil
	})
}

// UnstageHunk unstages a single hunk from a staged file by building a patch
// and applying it in reverse via git apply --cached --reverse.
func (c *Client) UnstageHunk(ctx context.Context, path string, hunk Hunk) error {
	if err := ValidateRepoRelativePath(path); err != nil {
		return fmt.Errorf("unstage hunk path: %w", err)
	}
	patch := buildHunkPatch(path, hunk)
	return c.queue.Exec(ctx, func() error {
		if err := c.runWithStdin(ctx, []byte(patch), "apply", "--cached", "--reverse"); err != nil {
			return fmt.Errorf("unstage hunk: %w", err)
		}
		c.cache.Invalidate()
		return nil
	})
}

// StageLine stages a single diff line from an unstaged hunk by constructing
// a synthetic patch containing only that line's change.
func (c *Client) StageLine(ctx context.Context, path string, hunk Hunk, lineIdx int) error {
	if err := ValidateRepoRelativePath(path); err != nil {
		return fmt.Errorf("stage line path: %w", err)
	}
	if lineIdx < 0 || lineIdx >= len(hunk.Lines) {
		return fmt.Errorf("stage line: index %d out of range [0, %d)", lineIdx, len(hunk.Lines))
	}
	if hunk.Lines[lineIdx].Type == DiffLineContext {
		return fmt.Errorf("stage line: cannot stage a context line")
	}
	patch := buildLinePatch(path, hunk, lineIdx)
	return c.queue.Exec(ctx, func() error {
		if err := c.runWithStdin(ctx, []byte(patch), "apply", "--cached"); err != nil {
			return fmt.Errorf("stage line: %w", err)
		}
		c.cache.Invalidate()
		return nil
	})
}

// UnstageLine unstages a single diff line from a staged hunk by constructing
// a synthetic patch and applying it in reverse.
func (c *Client) UnstageLine(ctx context.Context, path string, hunk Hunk, lineIdx int) error {
	if err := ValidateRepoRelativePath(path); err != nil {
		return fmt.Errorf("unstage line path: %w", err)
	}
	if lineIdx < 0 || lineIdx >= len(hunk.Lines) {
		return fmt.Errorf("unstage line: index %d out of range [0, %d)", lineIdx, len(hunk.Lines))
	}
	if hunk.Lines[lineIdx].Type == DiffLineContext {
		return fmt.Errorf("unstage line: cannot unstage a context line")
	}
	patch := buildLinePatch(path, hunk, lineIdx)
	return c.queue.Exec(ctx, func() error {
		if err := c.runWithStdin(ctx, []byte(patch), "apply", "--cached", "--reverse"); err != nil {
			return fmt.Errorf("unstage line: %w", err)
		}
		c.cache.Invalidate()
		return nil
	})
}

// buildHunkPatch constructs a unified diff patch for a single hunk.
func buildHunkPatch(path string, hunk Hunk) string {
	p := sanitizePatchPath(path)
	header := sanitizePatchHeader(hunk.Header, hunk.OldStart, hunk.OldLines, hunk.NewStart, hunk.NewLines)

	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "diff --git a/%s b/%s\n", p, p)
	_, _ = fmt.Fprintf(&b, "--- a/%s\n", p)
	_, _ = fmt.Fprintf(&b, "+++ b/%s\n", p)
	b.WriteString(header + "\n")
	for _, line := range hunk.Lines {
		content := sanitizePatchLineContent(line.Content)
		switch line.Type {
		case DiffLineContext:
			b.WriteString(" " + content + "\n")
		case DiffLineAdded:
			b.WriteString("+" + content + "\n")
		case DiffLineRemoved:
			b.WriteString("-" + content + "\n")
		}
	}
	if hunk.NoNewlineEOF {
		b.WriteString("\\ No newline at end of file\n")
	}
	return b.String()
}

// buildLinePatch constructs a synthetic patch that includes only the change
// at lineIdx, with all other changes neutralized. Non-selected added lines
// are dropped (they don't exist in the old side), and non-selected removed
// lines are converted to context (they exist in both sides).
func buildLinePatch(path string, hunk Hunk, lineIdx int) string {
	var synthLines []DiffLine
	for i, line := range hunk.Lines {
		if i == lineIdx {
			synthLines = append(synthLines, line)
			continue
		}
		switch line.Type {
		case DiffLineContext:
			synthLines = append(synthLines, line)
		case DiffLineAdded:
			// Non-selected additions don't exist in the old side;
			// they cannot be context lines. Drop them.
		case DiffLineRemoved:
			// Non-selected removals exist in the old side;
			// convert to context so they remain unchanged.
			synthLines = append(synthLines, DiffLine{
				Type:    DiffLineContext,
				Content: line.Content,
			})
		}
	}

	oldCount, newCount := 0, 0
	for _, line := range synthLines {
		switch line.Type {
		case DiffLineContext:
			oldCount++
			newCount++
		case DiffLineAdded:
			newCount++
		case DiffLineRemoved:
			oldCount++
		}
	}

	p := sanitizePatchPath(path)
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "diff --git a/%s b/%s\n", p, p)
	_, _ = fmt.Fprintf(&b, "--- a/%s\n", p)
	_, _ = fmt.Fprintf(&b, "+++ b/%s\n", p)
	_, _ = fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@\n", hunk.OldStart, oldCount, hunk.NewStart, newCount)
	for _, line := range synthLines {
		content := sanitizePatchLineContent(line.Content)
		switch line.Type {
		case DiffLineContext:
			b.WriteString(" " + content + "\n")
		case DiffLineAdded:
			b.WriteString("+" + content + "\n")
		case DiffLineRemoved:
			b.WriteString("-" + content + "\n")
		}
	}
	if hunk.NoNewlineEOF {
		b.WriteString("\\ No newline at end of file\n")
	}
	return b.String()
}

func sanitizePatchPath(path string) string {
	// Normalise all backslashes to forward slashes regardless of OS so
	// that Windows-style paths produce valid git diff headers on Linux.
	p := strings.ReplaceAll(path, "\\", "/")
	if vol := filepath.VolumeName(path); vol != "" {
		p = strings.TrimPrefix(p, filepath.ToSlash(vol))
	}
	p = strings.NewReplacer("\x00", "", "\r", "", "\n", "").Replace(p)
	p = strings.TrimLeft(p, "/")
	p = stdpath.Clean("/" + p)
	p = strings.TrimPrefix(p, "/")
	if p == "" || p == "." {
		return "invalid-path"
	}
	return p
}

func sanitizePatchHeader(header string, oldStart, oldLines, newStart, newLines int) string {
	fallback := fmt.Sprintf("@@ -%d,%d +%d,%d @@", oldStart, oldLines, newStart, newLines)
	if header == "" {
		return fallback
	}
	clean := strings.NewReplacer("\x00", "", "\r", "", "\n", "\n").Replace(header)
	clean = strings.Split(clean, "\n")[0]
	clean = strings.TrimSpace(clean)
	if !strings.HasPrefix(clean, "@@") {
		return fallback
	}
	return clean
}

func sanitizePatchLineContent(content string) string {
	return strings.NewReplacer("\x00", "", "\r", "\\r", "\n", "\\n").Replace(content)
}

// runWithStdin executes a git command with data piped to stdin.
func (c *Client) runWithStdin(ctx context.Context, stdin []byte, args ...string) error {
	if err := validateArgs(args); err != nil {
		return fmt.Errorf("invalid git argument: %w", err)
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = c.repoDir
	cmd.Stdin = bytes.NewReader(stdin)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	slog.Debug("git exec (stdin)", "args", args, "dir", c.repoDir, "stdinLen", len(stdin))

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			return fmt.Errorf("git %s: %w", args[0], err)
		}
		return fmt.Errorf("git %s: %s: %w", args[0], errMsg, err)
	}

	return nil
}
