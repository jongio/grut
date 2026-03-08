package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveIconMode_PassthroughNerd(t *testing.T) {
	assert.Equal(t, "nerd", ResolveIconMode("nerd"))
}

func TestResolveIconMode_PassthroughASCII(t *testing.T) {
	assert.Equal(t, "ascii", ResolveIconMode("ascii"))
}

func TestResolveIconMode_ExplicitEnvNerd(t *testing.T) {
	t.Setenv("GRUT_NERD_FONT", "1")
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("WT_SESSION", "")
	assert.Equal(t, "nerd", ResolveIconMode("auto"))
}

func TestResolveIconMode_ExplicitEnvASCII(t *testing.T) {
	t.Setenv("GRUT_NERD_FONT", "0")
	t.Setenv("TERM_PROGRAM", "WezTerm") // would match, but explicit opt-out wins
	assert.Equal(t, "ascii", ResolveIconMode("auto"))
}

func TestResolveIconMode_TermProgram(t *testing.T) {
	t.Setenv("GRUT_NERD_FONT", "")
	t.Setenv("WT_SESSION", "")

	for _, tp := range []string{"WezTerm", "kitty", "Alacritty", "iTerm.app", "Hyper", "warp", "rio", "Ghostty"} {
		t.Run(tp, func(t *testing.T) {
			t.Setenv("TERM_PROGRAM", tp)
			assert.Equal(t, "nerd", ResolveIconMode("auto"))
		})
	}
}

func TestResolveIconMode_WTSession(t *testing.T) {
	t.Setenv("GRUT_NERD_FONT", "")
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("WT_SESSION", "some-guid-value")
	assert.Equal(t, "nerd", ResolveIconMode("auto"))
}

func TestResolveIconMode_FallbackASCII(t *testing.T) {
	t.Setenv("GRUT_NERD_FONT", "")
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("WT_SESSION", "")
	assert.Equal(t, "ascii", ResolveIconMode("auto"))
}

func TestResolveIconMode_TrueYesVariants(t *testing.T) {
	for _, v := range []string{"true", "yes"} {
		t.Run(v, func(t *testing.T) {
			t.Setenv("GRUT_NERD_FONT", v)
			t.Setenv("TERM_PROGRAM", "")
			t.Setenv("WT_SESSION", "")
			assert.Equal(t, "nerd", ResolveIconMode("auto"))
		})
	}
}

func TestResolveIconMode_FalseNoVariants(t *testing.T) {
	for _, v := range []string{"false", "no"} {
		t.Run(v, func(t *testing.T) {
			t.Setenv("GRUT_NERD_FONT", v)
			assert.Equal(t, "ascii", ResolveIconMode("auto"))
		})
	}
}
