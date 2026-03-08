package panels

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBasePanel_Focus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		panel BasePanel
		want  bool
	}{
		{
			name:  "sets focused true",
			panel: BasePanel{},
			want:  true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			panel := tt.panel
			panel.Focus()

			assert.Equal(t, tt.want, panel.Focused)
		})
	}
}

func TestBasePanel_Blur(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		panel BasePanel
		want  bool
	}{
		{
			name:  "sets focused false",
			panel: BasePanel{Focused: true},
			want:  false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			panel := tt.panel
			panel.Blur()

			assert.Equal(t, tt.want, panel.Focused)
		})
	}
}

func TestBasePanel_FocusBlurCycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "focus blur focus"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			panel := BasePanel{}

			panel.Focus()
			assert.True(t, panel.Focused)

			panel.Blur()
			assert.False(t, panel.Focused)

			panel.Focus()
			assert.True(t, panel.Focused)
		})
	}
}

func TestBasePanel_SetSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		width  int
		height int
	}{
		{
			name:   "sets width and height",
			width:  80,
			height: 24,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			panel := BasePanel{}
			panel.SetSize(tt.width, tt.height)

			assert.Equal(t, tt.width, panel.Width)
			assert.Equal(t, tt.height, panel.Height)
		})
	}
}

func TestBasePanel_SetSize_Zero(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		width  int
		height int
	}{
		{
			name:   "accepts zero values",
			width:  0,
			height: 0,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			panel := BasePanel{}
			panel.SetSize(tt.width, tt.height)

			assert.Equal(t, tt.width, panel.Width)
			assert.Equal(t, tt.height, panel.Height)
		})
	}
}

func TestBasePanel_SetSize_Large(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		width  int
		height int
	}{
		{
			name:   "accepts large values",
			width:  4096,
			height: 2160,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			panel := BasePanel{}
			panel.SetSize(tt.width, tt.height)

			assert.Equal(t, tt.width, panel.Width)
			assert.Equal(t, tt.height, panel.Height)
		})
	}
}

func TestBasePanel_Title(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		title string
	}{
		{
			name:  "returns panel title",
			title: "Status",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			panel := BasePanel{PanelTitle: tt.title}

			assert.Equal(t, tt.title, panel.Title())
		})
	}
}

func TestBasePanel_Title_Empty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "returns empty title"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			panel := BasePanel{}

			assert.Equal(t, "", panel.Title())
		})
	}
}

func TestBasePanel_KeyBindings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "returns nil by default"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			panel := BasePanel{}

			assert.Nil(t, panel.KeyBindings())
		})
	}
}

func TestBasePanel_DefaultState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "zero value defaults"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			panel := BasePanel{}

			assert.False(t, panel.Focused)
			assert.Equal(t, 0, panel.Width)
			assert.Equal(t, 0, panel.Height)
			assert.Equal(t, "", panel.PanelTitle)
		})
	}
}

func TestBasePanel_ConstructedWithTitle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		title string
	}{
		{
			name:  "constructed title is returned",
			title: "Files",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			panel := BasePanel{PanelTitle: tt.title}

			assert.Equal(t, tt.title, panel.Title())
		})
	}
}

func TestKeyBinding_Struct(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		key         string
		description string
		action      string
	}{
		{
			name:        "fields are assigned",
			key:         "ctrl+s",
			description: "Save file",
			action:      "save",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			binding := KeyBinding{
				Key:         tt.key,
				Description: tt.description,
				Action:      tt.action,
			}

			assert.Equal(t, tt.key, binding.Key)
			assert.Equal(t, tt.description, binding.Description)
			assert.Equal(t, tt.action, binding.Action)
		})
	}
}
