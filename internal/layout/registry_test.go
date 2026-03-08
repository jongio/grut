package layout

import (
	"testing"

	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	assert.NotNil(t, r)
	assert.Empty(t, r.Names())
}

func TestRegistryRegisterAndCreate(t *testing.T) {
	r := NewRegistry()
	r.Register("test", func() panels.Panel {
		return panels.NewPlaceholder("test")
	})

	assert.True(t, r.Has("test"))
	assert.False(t, r.Has("nonexistent"))

	p, err := r.Create("test")
	require.NoError(t, err)
	assert.Equal(t, "test", p.Title())
}

func TestRegistryCreateUnknown(t *testing.T) {
	r := NewRegistry()
	_, err := r.Create("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown panel")
}

func TestRegistryNames(t *testing.T) {
	r := NewRegistry()
	r.Register("alpha", func() panels.Panel { return panels.NewPlaceholder("alpha") })
	r.Register("beta", func() panels.Panel { return panels.NewPlaceholder("beta") })

	names := r.Names()
	assert.Len(t, names, 2)
	assert.Contains(t, names, "alpha")
	assert.Contains(t, names, "beta")
}

func TestRegistryOverwrite(t *testing.T) {
	r := NewRegistry()
	r.Register("test", func() panels.Panel {
		return panels.NewPlaceholder("original")
	})
	r.Register("test", func() panels.Panel {
		return panels.NewPlaceholder("replaced")
	})

	p, err := r.Create("test")
	require.NoError(t, err)
	assert.Equal(t, "replaced", p.Title())
}

func TestRegisterDefaults(t *testing.T) {
	r := NewRegistry()
	cfg, err := config.Load()
	require.NoError(t, err)
	RegisterDefaults(r, cfg, nil, nil)

	expectedPanels := []string{"filetree", "preview", "status", "gitdiff"}
	expectedTitles := map[string]string{
		"filetree": "Files",
		"preview":  "preview",
		"status":   "status",
		"gitdiff":  "gitdiff",
	}
	for _, name := range expectedPanels {
		assert.True(t, r.Has(name), "expected panel %q to be registered", name)

		p, err := r.Create(name)
		require.NoError(t, err, "creating panel %q", name)
		assert.Equal(t, expectedTitles[name], p.Title())
	}
}
