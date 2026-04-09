package config

import "testing"

func TestParseGitHubRemote(t *testing.T) {
	tests := []struct {
		url       string
		wantOwner string
		wantRepo  string
	}{
		{"https://github.com/jongio/grut.git", "jongio", "grut"},
		{"https://github.com/jongio/grut", "jongio", "grut"},
		{"git@github.com:jongio/grut.git", "jongio", "grut"},
		{"git@github.com:jongio/grut", "jongio", "grut"},
		{"https://gitlab.com/foo/bar.git", "", ""},
		{"", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			owner, repo := parseGitHubRemote(tt.url)
			if owner != tt.wantOwner || repo != tt.wantRepo {
				t.Errorf("parseGitHubRemote(%q) = (%q, %q), want (%q, %q)",
					tt.url, owner, repo, tt.wantOwner, tt.wantRepo)
			}
		})
	}
}

func TestEffectivePageSize(t *testing.T) {
	tests := []struct {
		name     string
		pageSize int
		want     int
	}{
		{"zero returns default", 0, 30},
		{"negative returns default", -1, 30},
		{"positive returns configured", 50, 50},
		{"one returns configured", 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &GitHubConfig{PageSize: tt.pageSize}
			if got := c.EffectivePageSize(); got != tt.want {
				t.Errorf("EffectivePageSize() = %d, want %d", got, tt.want)
			}
		})
	}
}
