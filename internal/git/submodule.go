package git

import (
	"context"
	"fmt"
	"strings"
)

const (
	gitCleanCommand              = "clean"
	submoduleStateClean          = gitCleanCommand
	submoduleStateModified       = "modified"
	submoduleStateNotInitialized = "not initialized"
	submoduleStateConflicted     = "conflicted"
)

// Submodule describes one entry from git submodule status.
type Submodule struct {
	Path        string
	Commit      string
	Describe    string
	Initialized bool
	Modified    bool
	Conflicted  bool
}

// State returns a concise display state for the submodule.
func (s Submodule) State() string {
	switch {
	case s.Conflicted:
		return submoduleStateConflicted
	case !s.Initialized:
		return submoduleStateNotInitialized
	case s.Modified:
		return submoduleStateModified
	default:
		return submoduleStateClean
	}
}

// Submodules returns submodule status entries for the repository.
func (c *Client) Submodules(ctx context.Context) ([]Submodule, error) {
	out, err := c.run(ctx, "submodule", "status")
	if err != nil {
		return nil, fmt.Errorf("submodules: %w", err)
	}
	return parseSubmoduleStatus(out)
}

func parseSubmoduleStatus(out string) ([]Submodule, error) {
	lines := splitLines(out)
	if len(lines) == 0 {
		return []Submodule{}, nil
	}
	submodules := make([]Submodule, 0, len(lines))
	for _, line := range lines {
		if len(line) < 42 {
			return nil, fmt.Errorf("parse submodule status: malformed line %q", line)
		}
		status := line[0]
		commit := line[1:41]
		rest := strings.TrimSpace(line[41:])
		if rest == "" {
			return nil, fmt.Errorf("parse submodule status: missing path in line %q", line)
		}
		path := rest
		describe := ""
		if strings.HasSuffix(rest, ")") {
			if idx := strings.LastIndex(rest, " ("); idx >= 0 {
				path = rest[:idx]
				describe = strings.TrimSuffix(rest[idx+2:], ")")
			}
		}
		sm := Submodule{
			Path:        path,
			Commit:      commit,
			Describe:    describe,
			Initialized: status != '-',
			Modified:    status == '+',
			Conflicted:  status == 'U',
		}
		submodules = append(submodules, sm)
	}
	return submodules, nil
}
