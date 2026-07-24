package theme

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveColorMode(t *testing.T) {
	tests := []struct {
		name       string
		configMode string
		noColorSet bool
		want       Mode
	}{
		{name: "auto color without NO_COLOR", configMode: configModeAuto, noColorSet: false, want: ModeColor},
		{name: "auto mono with NO_COLOR", configMode: configModeAuto, noColorSet: true, want: ModeMono},
		{name: "empty acts like auto", configMode: "", noColorSet: true, want: ModeMono},
		{name: "color overrides NO_COLOR", configMode: configModeColor, noColorSet: true, want: ModeColor},
		{name: "mono overrides environment", configMode: configModeMono, noColorSet: false, want: ModeMono},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ResolveColorMode(tt.configMode, tt.noColorSet))
		})
	}
}

func TestResolveEnvironmentColorModeDetectsNoColorPresence(t *testing.T) {
	original, hadOriginal := os.LookupEnv(noColorEnv)
	t.Cleanup(func() {
		if hadOriginal {
			require.NoError(t, os.Setenv(noColorEnv, original))
		} else {
			require.NoError(t, os.Unsetenv(noColorEnv))
		}
	})

	require.NoError(t, os.Unsetenv(noColorEnv))
	assert.Equal(t, ModeColor, ResolveEnvironmentColorMode(configModeAuto))

	require.NoError(t, os.Setenv(noColorEnv, ""))
	assert.Equal(t, ModeMono, ResolveEnvironmentColorMode(configModeAuto))
	assert.Equal(t, ModeColor, ResolveEnvironmentColorMode(configModeColor))
}

func TestApplyColorModeMonoVariantCollapsesDecorativeColors(t *testing.T) {
	th, err := Load("default")
	require.NoError(t, err)

	mono := ApplyColorMode(*th, ModeMono)

	assert.Equal(t, ModeMono, mono.Mode)
	assert.Equal(t, mono.Colors.Foreground, mono.Colors.GitStaged)
	assert.Equal(t, mono.Colors.Foreground, mono.Colors.GitConflict)
	assert.Equal(t, mono.Colors.Foreground, mono.Colors.DiffAdded)
	assert.Equal(t, mono.Colors.Foreground, mono.Colors.DiffRemoved)
	assert.Equal(t, mono.Colors.Foreground, mono.Colors.NotifyWarn)
	assert.Equal(t, mono.Colors.Foreground, mono.Colors.NotifyError)
	assert.Equal(t, th.Colors.Background, mono.Colors.Background)
	assert.Contains(t, mono.Styles.Selection.Render("selected"), "selected")
	assert.Equal(t, "U", StatusMarker(StatusConflict))
}

func TestStatusMarker(t *testing.T) {
	tests := []struct {
		state StatusState
		want  string
	}{
		{state: StatusStaged, want: "A/M/D"},
		{state: StatusUnstaged, want: "M/D"},
		{state: StatusUntracked, want: "?"},
		{state: StatusConflict, want: "U"},
		{state: StatusAdded, want: "+"},
		{state: StatusRemoved, want: "-"},
		{state: StatusWarning, want: "⚠"},
		{state: StatusError, want: "✗"},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			assert.Equal(t, tt.want, StatusMarker(tt.state))
		})
	}
}
