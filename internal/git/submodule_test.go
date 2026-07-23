package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSubmoduleStatus(t *testing.T) {
	t.Parallel()
	const sha1 = "0123456789abcdef0123456789abcdef01234567"
	const sha2 = "89abcdef0123456789abcdef0123456789abcdef"
	const sha3 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const sha4 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	tests := []struct {
		name string
		out  string
		want []Submodule
	}{
		{
			name: "empty output",
			out:  "",
			want: []Submodule{},
		},
		{
			name: "clean with describe",
			out:  " " + sha1 + " deps/lib (v1.2.3)",
			want: []Submodule{{Path: "deps/lib", Commit: sha1, Describe: "v1.2.3", Initialized: true}},
		},
		{
			name: "modified without describe",
			out:  "+" + sha2 + " vendor/changed",
			want: []Submodule{{Path: "vendor/changed", Commit: sha2, Initialized: true, Modified: true}},
		},
		{
			name: "not initialized path with spaces",
			out:  "-" + sha3 + " third party/module name (heads/main)",
			want: []Submodule{{Path: "third party/module name", Commit: sha3, Describe: "heads/main", Initialized: false}},
		},
		{
			name: "conflicted",
			out:  "U" + sha4 + " deps/conflicted (remotes/origin/main)",
			want: []Submodule{{Path: "deps/conflicted", Commit: sha4, Describe: "remotes/origin/main", Initialized: true, Conflicted: true}},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseSubmoduleStatus(tt.out)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSubmoduleState(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		sm   Submodule
		want string
	}{
		{name: "clean", sm: Submodule{Initialized: true}, want: submoduleStateClean},
		{name: "modified", sm: Submodule{Initialized: true, Modified: true}, want: submoduleStateModified},
		{name: "not initialized", sm: Submodule{}, want: submoduleStateNotInitialized},
		{name: "conflicted", sm: Submodule{Initialized: true, Conflicted: true}, want: submoduleStateConflicted},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.sm.State())
		})
	}
}
