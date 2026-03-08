package git

import "testing"

func TestRemoteToHTTPS(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"empty", "", ""},
		{"ssh colon", "git@github.com:user/repo.git", "https://github.com/user/repo"},
		{"ssh colon no suffix", "git@github.com:user/repo", "https://github.com/user/repo"},
		{"https", "https://github.com/user/repo.git", "https://github.com/user/repo"},
		{"https no suffix", "https://github.com/user/repo", "https://github.com/user/repo"},
		{"http", "http://github.com/user/repo.git", "http://github.com/user/repo"},
		{"ssh scheme", "ssh://git@github.com/user/repo.git", "https://github.com/user/repo"},
		{"unknown", "ftp://example.com/repo", ""},
		{"whitespace", "  https://github.com/user/repo.git  ", "https://github.com/user/repo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := RemoteToHTTPS(tt.raw)
			if got != tt.want {
				t.Errorf("RemoteToHTTPS(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
