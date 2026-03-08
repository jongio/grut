package git

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseWorktreeListCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  []Worktree
	}{
		{
			name:  "empty input returns empty slice",
			input: "",
			want:  []Worktree{},
		},
		{
			name: "single worktree with head and branch",
			input: "worktree /path/to/main\n" +
				"HEAD abc1234\n" +
				"branch refs/heads/main\n\n",
			want: []Worktree{{
				Path:   filepath.FromSlash("/path/to/main"),
				Head:   "abc1234",
				Branch: "main",
			}},
		},
		{
			name: "multiple worktrees separated by blank lines",
			input: "worktree /path/to/main\n" +
				"HEAD abc1234\n" +
				"branch refs/heads/main\n\n" +
				"worktree /path/to/feature\n" +
				"HEAD def5678\n" +
				"branch refs/heads/feature-branch\n\n",
			want: []Worktree{
				{
					Path:   filepath.FromSlash("/path/to/main"),
					Head:   "abc1234",
					Branch: "main",
				},
				{
					Path:   filepath.FromSlash("/path/to/feature"),
					Head:   "def5678",
					Branch: "feature-branch",
				},
			},
		},
		{
			name: "bare worktree without branch",
			input: "worktree /path/to/bare\n" +
				"HEAD abc1234\n" +
				"bare\n\n",
			want: []Worktree{{
				Path: filepath.FromSlash("/path/to/bare"),
				Head: "abc1234",
				Bare: true,
			}},
		},
		{
			name: "windows line endings are handled",
			input: "worktree /path/to/main\r\n" +
				"HEAD abc1234\r\n" +
				"branch refs/heads/main\r\n\r\n",
			want: []Worktree{{
				Path:   filepath.FromSlash("/path/to/main"),
				Head:   "abc1234",
				Branch: "main",
			}},
		},
		{
			name: "no trailing blank line still captures last worktree",
			input: "worktree /path/to/feature\n" +
				"HEAD def5678\n" +
				"branch refs/heads/feature-branch\n",
			want: []Worktree{{
				Path:   filepath.FromSlash("/path/to/feature"),
				Head:   "def5678",
				Branch: "feature-branch",
			}},
		},
		{
			name:  "worktree with only path",
			input: "worktree /path/to/path-only\n\n",
			want: []Worktree{{
				Path: filepath.FromSlash("/path/to/path-only"),
			}},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := parseWorktreeList(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}
