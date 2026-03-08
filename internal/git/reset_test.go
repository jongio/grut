package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResetFlagCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		mode        ResetMode
		want        string
		wantErr     string
		wantErrMode string
	}{
		{
			name: "ResetSoft returns soft flag",
			mode: ResetSoft,
			want: "--soft",
		},
		{
			name: "ResetMixed returns mixed flag",
			mode: ResetMixed,
			want: "--mixed",
		},
		{
			name: "ResetHard returns hard flag",
			mode: ResetHard,
			want: "--hard",
		},
		{
			name:        "invalid mode returns descriptive error",
			mode:        ResetMode("sideways"),
			wantErr:     "reset: invalid mode",
			wantErrMode: "sideways",
		},
		{
			name:        "empty string mode returns error",
			mode:        ResetMode(""),
			wantErr:     "reset: invalid mode",
			wantErrMode: "\"\"",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := resetFlag(tt.mode)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Empty(t, got)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Contains(t, err.Error(), tt.wantErrMode)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
