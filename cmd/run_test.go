package cmd

import (
	"bytes"
	"sort"
	"strings"
	"testing"

	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/shortcuts"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

// ---------------------------------------------------------------------------
// parseShortcutArgs
// ---------------------------------------------------------------------------

func TestParseShortcutArgs_EmptySlice(t *testing.T) {
	result := parseShortcutArgs([]string{})
	assert.Empty(t, result)
}

func TestParseShortcutArgs_NilSlice(t *testing.T) {
	result := parseShortcutArgs(nil)
	assert.Empty(t, result)
}

func TestParseShortcutArgs_SingleKeyValue(t *testing.T) {
	result := parseShortcutArgs([]string{"branch=main"})
	assert.Equal(t, map[string]string{"branch": "main"}, result)
}

func TestParseShortcutArgs_MultipleKeyValues(t *testing.T) {
	result := parseShortcutArgs([]string{"branch=main", "remote=origin"})
	assert.Equal(t, "main", result["branch"])
	assert.Equal(t, "origin", result["remote"])
	assert.Len(t, result, 2)
}

func TestParseShortcutArgs_NoEquals(t *testing.T) {
	// Entries without "=" are skipped by strings.Cut.
	result := parseShortcutArgs([]string{"noequalssign"})
	assert.Empty(t, result)
}

func TestParseShortcutArgs_MultipleEquals(t *testing.T) {
	// strings.Cut splits on the first "="; the rest stays in the value.
	result := parseShortcutArgs([]string{"msg=hello=world"})
	assert.Equal(t, "hello=world", result["msg"])
}

func TestParseShortcutArgs_EmptyKey(t *testing.T) {
	result := parseShortcutArgs([]string{"=value"})
	assert.Equal(t, "value", result[""])
}

func TestParseShortcutArgs_EmptyValue(t *testing.T) {
	result := parseShortcutArgs([]string{"key="})
	assert.Equal(t, "", result["key"])
}

func TestParseShortcutArgs_MixedValidAndInvalid(t *testing.T) {
	result := parseShortcutArgs([]string{"branch=main", "ignored", "msg=test"})
	assert.Len(t, result, 2)
	assert.Equal(t, "main", result["branch"])
	assert.Equal(t, "test", result["msg"])
}

func TestParseShortcutArgs_DuplicateKeysLastWins(t *testing.T) {
	result := parseShortcutArgs([]string{"key=first", "key=second"})
	assert.Equal(t, "second", result["key"])
}

func TestParseShortcutArgs_ValueWithSpaces(t *testing.T) {
	// In practice, shell quoting handles this; the function gets a single string.
	result := parseShortcutArgs([]string{"msg=fix the bug"})
	assert.Equal(t, "fix the bug", result["msg"])
}

// ---------------------------------------------------------------------------
// formatParams
// ---------------------------------------------------------------------------

func TestFormatParams_NilMap(t *testing.T) {
	assert.Equal(t, "", formatParams(nil))
}

func TestFormatParams_EmptyMap(t *testing.T) {
	assert.Equal(t, "", formatParams(map[string]string{}))
}

func TestFormatParams_SingleEntry(t *testing.T) {
	result := formatParams(map[string]string{"branch": "main"})
	assert.Equal(t, "branch=main", result)
}

func TestFormatParams_MultipleEntries(t *testing.T) {
	params := map[string]string{"branch": "main", "msg": "fix"}
	result := formatParams(params)
	// Map iteration order is non-deterministic; sort parts to compare.
	parts := strings.Split(result, " ")
	sort.Strings(parts)
	assert.Equal(t, []string{"branch=main", "msg=fix"}, parts)
}

func TestFormatParams_EntryWithEmptyValue(t *testing.T) {
	result := formatParams(map[string]string{"flag": ""})
	assert.Equal(t, "flag=", result)
}

func TestFormatParams_ThreeEntries(t *testing.T) {
	params := map[string]string{"a": "1", "b": "2", "c": "3"}
	result := formatParams(params)
	parts := strings.Split(result, " ")
	assert.Len(t, parts, 3)
	sort.Strings(parts)
	assert.Equal(t, []string{"a=1", "b=2", "c=3"}, parts)
}

// ---------------------------------------------------------------------------
// stepOnFail
// ---------------------------------------------------------------------------

func TestStepOnFail_Stop(t *testing.T) {
	s := shortcuts.Step{OnFail: shortcuts.OnFailStop}
	assert.Equal(t, "stop", stepOnFail(s))
}

func TestStepOnFail_Continue(t *testing.T) {
	s := shortcuts.Step{OnFail: shortcuts.OnFailContinue}
	assert.Equal(t, "continue", stepOnFail(s))
}

func TestStepOnFail_Ask(t *testing.T) {
	s := shortcuts.Step{OnFail: shortcuts.OnFailAsk}
	assert.Equal(t, "ask", stepOnFail(s))
}

func TestStepOnFail_EmptyDefaultsToStop(t *testing.T) {
	s := shortcuts.Step{OnFail: ""}
	assert.Equal(t, shortcuts.OnFailStop, stepOnFail(s))
}

func TestStepOnFail_CustomValue(t *testing.T) {
	// If someone sets a non-standard value, it passes through.
	s := shortcuts.Step{OnFail: "retry"}
	assert.Equal(t, "retry", stepOnFail(s))
}

// ---------------------------------------------------------------------------
// printShortcutDetails
// ---------------------------------------------------------------------------

func TestPrintShortcutDetails_Builtin(t *testing.T) {
	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	s := shortcuts.Shortcut{
		Name:        "quick-commit",
		Description: "Stage and commit all changes",
		Builtin:     true,
		Confirm:     true,
		Steps: []shortcuts.Step{
			{Op: "stage", Params: map[string]string{"path": "."}, OnFail: "stop"},
			{Op: "commit", Params: map[string]string{"msg": "wip"}, OnFail: "stop"},
		},
	}
	printShortcutDetails(cmd, s)

	output := buf.String()
	assert.Contains(t, output, "quick-commit")
	assert.Contains(t, output, "builtin")
	assert.Contains(t, output, "Stage and commit all changes")
	assert.Contains(t, output, "true") // Confirm
	assert.Contains(t, output, "Steps:")
	assert.Contains(t, output, "stage")
	assert.Contains(t, output, "commit")
}

func TestPrintShortcutDetails_Custom(t *testing.T) {
	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	s := shortcuts.Shortcut{
		Name:        "deploy",
		Description: "Deploy to production",
		Builtin:     false,
		Confirm:     false,
		Steps: []shortcuts.Step{
			{Op: "push", Params: map[string]string{"remote": "origin"}, OnFail: "continue"},
		},
	}
	printShortcutDetails(cmd, s)

	output := buf.String()
	assert.Contains(t, output, "deploy")
	assert.Contains(t, output, "custom")
	assert.Contains(t, output, "Deploy to production")
}

func TestPrintShortcutDetails_WithArgs(t *testing.T) {
	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	s := shortcuts.Shortcut{
		Name:        "release",
		Description: "Create a release",
		Builtin:     true,
		Steps:       []shortcuts.Step{{Op: "tag", OnFail: "stop"}},
		Args: []shortcuts.Arg{
			{Name: "version", Prompt: "Release version", Required: true, Default: ""},
			{Name: "channel", Prompt: "Release channel", Required: false, Default: "stable"},
		},
	}
	printShortcutDetails(cmd, s)

	output := buf.String()
	assert.Contains(t, output, "Arguments:")
	assert.Contains(t, output, "version")
	assert.Contains(t, output, "Release version")
	assert.Contains(t, output, "(required)")
	assert.Contains(t, output, "channel")
	assert.Contains(t, output, "[default: stable]")
}

func TestPrintShortcutDetails_NoSteps(t *testing.T) {
	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	s := shortcuts.Shortcut{
		Name:        "empty",
		Description: "An empty shortcut",
		Steps:       nil,
	}
	printShortcutDetails(cmd, s)

	output := buf.String()
	assert.Contains(t, output, "empty")
	assert.Contains(t, output, "Steps:")
}

// ---------------------------------------------------------------------------
// newRunCmd — flag registration
// ---------------------------------------------------------------------------

func TestNewRunCmd_FlagsRegistered(t *testing.T) {
	cmd := newRunCmd()

	tests := []struct {
		name     string
		flagType string
	}{
		{"list", "bool"},
		{"describe", "string"},
		{"dry-run", "bool"},
		{"no-confirm", "bool"},
	}

	for _, tt := range tests {
		f := cmd.Flags().Lookup(tt.name)
		if assert.NotNil(t, f, "flag --%s must be registered", tt.name) {
			assert.Equal(t, tt.flagType, f.Value.Type(), "flag --%s type mismatch", tt.name)
		}
	}
}

func TestNewRunCmd_UseAndShort(t *testing.T) {
	cmd := newRunCmd()
	assert.Equal(t, "run <shortcut> [args...]", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
}

// ---------------------------------------------------------------------------
// registerCustomShortcuts
// ---------------------------------------------------------------------------

func TestRegisterCustomShortcuts_EmptyList(t *testing.T) {
	// nil git client is safe — we only call RegisterCustom, not Execute.
	engine := shortcuts.NewEngine(nil)
	registerCustomShortcuts(engine, nil)
	// Builtins still exist, but no customs were added.
	// Verify no panic and engine is usable.
	all := engine.List()
	for _, s := range all {
		assert.True(t, s.Builtin, "with no customs, all shortcuts should be builtin")
	}
}

func TestRegisterCustomShortcuts_SingleCustom(t *testing.T) {
	engine := shortcuts.NewEngine(nil)
	customs := []config.CustomShortcut{
		{
			Name:        "deploy",
			Description: "Deploy to production",
			Steps:       []string{"push remote=origin"},
		},
	}
	registerCustomShortcuts(engine, customs)

	sc, ok := engine.Resolve("deploy")
	assert.True(t, ok, "custom shortcut 'deploy' should be registered")
	assert.Equal(t, "Deploy to production", sc.Description)
	assert.True(t, sc.Confirm, "custom shortcuts default to Confirm=true")
	assert.False(t, sc.Builtin)
	assert.Len(t, sc.Steps, 1)
	assert.Equal(t, "push", sc.Steps[0].Op)
	assert.Equal(t, "origin", sc.Steps[0].Params["remote"])
}

func TestRegisterCustomShortcuts_MultipleSteps(t *testing.T) {
	engine := shortcuts.NewEngine(nil)
	customs := []config.CustomShortcut{
		{
			Name:        "release",
			Description: "Tag and push",
			Steps:       []string{"tag name=v1.0", "push remote=origin branch=main"},
		},
	}
	registerCustomShortcuts(engine, customs)

	sc, ok := engine.Resolve("release")
	assert.True(t, ok)
	assert.Len(t, sc.Steps, 2)
	assert.Equal(t, "tag", sc.Steps[0].Op)
	assert.Equal(t, "v1.0", sc.Steps[0].Params["name"])
	assert.Equal(t, "push", sc.Steps[1].Op)
	assert.Equal(t, "origin", sc.Steps[1].Params["remote"])
	assert.Equal(t, "main", sc.Steps[1].Params["branch"])
}

func TestRegisterCustomShortcuts_EmptyStep(t *testing.T) {
	engine := shortcuts.NewEngine(nil)
	customs := []config.CustomShortcut{
		{
			Name:  "empty-step",
			Steps: []string{"", "push"},
		},
	}
	registerCustomShortcuts(engine, customs)

	sc, ok := engine.Resolve("empty-step")
	assert.True(t, ok)
	// Empty step string is skipped (strings.Fields("") returns empty slice).
	assert.Len(t, sc.Steps, 1)
	assert.Equal(t, "push", sc.Steps[0].Op)
}

func TestRegisterCustomShortcuts_StepWithoutParams(t *testing.T) {
	engine := shortcuts.NewEngine(nil)
	customs := []config.CustomShortcut{
		{
			Name:  "simple",
			Steps: []string{"stage"},
		},
	}
	registerCustomShortcuts(engine, customs)

	sc, ok := engine.Resolve("simple")
	assert.True(t, ok)
	assert.Len(t, sc.Steps, 1)
	assert.Equal(t, "stage", sc.Steps[0].Op)
	assert.Empty(t, sc.Steps[0].Params)
}

func TestRegisterCustomShortcuts_OnFailDefaultsToStop(t *testing.T) {
	engine := shortcuts.NewEngine(nil)
	customs := []config.CustomShortcut{
		{Name: "test-sc", Steps: []string{"push"}},
	}
	registerCustomShortcuts(engine, customs)

	sc, ok := engine.Resolve("test-sc")
	assert.True(t, ok)
	assert.Equal(t, shortcuts.OnFailStop, sc.Steps[0].OnFail)
}
