package panelreg

import (
	"testing"

	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterAndBuilders(t *testing.T) {
	defer func() { Reset() }()
	Reset()

	Register("alpha", func(deps Deps) panels.Panel {
		return panels.NewPlaceholder("alpha", deps.Theme)
	})
	Register("beta", func(deps Deps) panels.Panel {
		return panels.NewPlaceholder("beta", deps.Theme)
	})

	bs := Builders()
	require.Len(t, bs, 2)
	assert.Contains(t, bs, "alpha")
	assert.Contains(t, bs, "beta")

	// Verify builder produces correct panel.
	p := bs["alpha"](Deps{})
	assert.Equal(t, "alpha", p.Title())
}

func TestRegisterOverwrite(t *testing.T) {
	defer func() { Reset() }()
	Reset()

	Register("x", func(deps Deps) panels.Panel {
		return panels.NewPlaceholder("first", deps.Theme)
	})
	Register("x", func(deps Deps) panels.Panel {
		return panels.NewPlaceholder("second", deps.Theme)
	})

	bs := Builders()
	require.Len(t, bs, 1)
	p := bs["x"](Deps{})
	assert.Equal(t, "second", p.Title())
}

func TestBuildersSnapshot(t *testing.T) {
	defer func() { Reset() }()
	Reset()

	Register("a", func(deps Deps) panels.Panel {
		return panels.NewPlaceholder("a", deps.Theme)
	})

	snap := Builders()
	// Mutating the snapshot doesn't affect the registry.
	delete(snap, "a")

	bs := Builders()
	assert.Len(t, bs, 1)
	assert.Contains(t, bs, "a")
}

func TestReset(t *testing.T) {
	defer func() { Reset() }()

	Register("z", func(deps Deps) panels.Panel {
		return panels.NewPlaceholder("z", deps.Theme)
	})
	Reset()
	assert.Empty(t, Builders())
}
