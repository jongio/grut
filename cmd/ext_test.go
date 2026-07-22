package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jongio/grut/internal/extension"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	extValidateCommand = "validate"
	extValidateJSONArg = "--json"
	extTestManifest    = "extension.toml"
	extTestName        = "valid-ext"
)

// ---------------------------------------------------------------------------
// newExtCmd — subcommand structure (see root_test.go for registration test)
// ---------------------------------------------------------------------------

func TestExtListCmd_NoExtensions(t *testing.T) {
	// Running ext list should succeed when no extensions are installed.
	cmd := newExtCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"list"})
	err := cmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "No extensions installed")
}

func TestExtCreateCmd_ListTemplates(t *testing.T) {
	// ext create --list should list available templates.
	cmd := newExtCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"create", "--list"})
	err := cmd.Execute()
	assert.NoError(t, err)
	// Should have at least one template output.
	assert.NotEmpty(t, buf.String())
}

func TestExtCreateCmd_NoName(t *testing.T) {
	// ext create with no args should error.
	cmd := newExtCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"create"})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "extension name is required")
}

func TestExtRemoveCmd_NonexistentExtension(t *testing.T) {
	cmd := newExtCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"remove", "nonexistent-extension"})
	err := cmd.Execute()
	assert.Error(t, err)
}

func TestExtEnableCmd_NonexistentExtension(t *testing.T) {
	cmd := newExtCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"enable", "nonexistent-extension"})
	err := cmd.Execute()
	assert.Error(t, err)
}

func TestExtDisableCmd_NonexistentExtension(t *testing.T) {
	cmd := newExtCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"disable", "nonexistent-extension"})
	err := cmd.Execute()
	assert.Error(t, err)
}

func TestExtInfoCmd_NonexistentExtension(t *testing.T) {
	cmd := newExtCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"info", "nonexistent-extension"})
	err := cmd.Execute()
	assert.Error(t, err)
}

func TestExtInstallCmd_MissingArgs(t *testing.T) {
	// install requires exactly 1 arg.
	cmd := newExtCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"install"})
	err := cmd.Execute()
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// newExtCreateCmd — flag registration
// ---------------------------------------------------------------------------

func TestNewExtCreateCmd_FlagsRegistered(t *testing.T) {
	cmd := newExtCreateCmd()
	templateFlag := cmd.Flags().Lookup("template")
	require.NotNil(t, templateFlag, "--template flag must be registered")
	assert.Equal(t, "lua", templateFlag.DefValue)

	listFlag := cmd.Flags().Lookup("list")
	require.NotNil(t, listFlag, "--list flag must be registered")
	assert.Equal(t, "false", listFlag.DefValue)
}

func TestExtListCmd_JSONFlagRegistered(t *testing.T) {
	cmd := newExtListCmd()
	flag := cmd.Flags().Lookup("json")
	require.NotNil(t, flag, "--json flag must be registered")
	assert.Equal(t, "bool", flag.Value.Type())
	assert.Equal(t, "false", flag.DefValue)
}

func TestExtInfoCmd_JSONFlagRegistered(t *testing.T) {
	cmd := newExtInfoCmd()
	flag := cmd.Flags().Lookup("json")
	require.NotNil(t, flag, "--json flag must be registered")
	assert.Equal(t, "bool", flag.Value.Type())
	assert.Equal(t, "false", flag.DefValue)
}

func TestExtValidateCmd_JSONFlagRegistered(t *testing.T) {
	cmd := newExtValidateCmd()
	flag := cmd.Flags().Lookup("json")
	require.NotNil(t, flag, "--json flag must be registered")
	assert.Equal(t, "bool", flag.Value.Type())
	assert.Equal(t, "false", flag.DefValue)
}

func TestExtValidateCmd_ValidatesScaffoldedTemplates(t *testing.T) {
	for _, tmpl := range extension.ListTemplates() {
		t.Run(tmpl.Name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, extension.Scaffold(dir, extTestName, tmpl.Name))

			cmd := newExtCmd()
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)
			cmd.SetArgs([]string{extValidateCommand, filepath.Join(dir, extTestName)})

			require.NoError(t, cmd.Execute())
			assert.Contains(t, buf.String(), "Extension project is valid")
			assert.Contains(t, buf.String(), tmpl.Runtime)
		})
	}
}

func TestExtValidateCmd_DefaultsToCurrentDirectory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, extension.Scaffold(dir, extTestName, "lua"))
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(filepath.Join(dir, extTestName)))
	t.Cleanup(func() { require.NoError(t, os.Chdir(origDir)) })

	cmd := newExtCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{extValidateCommand})

	require.NoError(t, cmd.Execute())
	assert.Contains(t, buf.String(), "Extension project is valid")
}

func TestExtValidateCmd_InvalidProjects(t *testing.T) {
	tests := []struct {
		name          string
		manifest      string
		fileName      string
		path          string
		wantErr       string
		wantOutput    string
		createProject bool
	}{
		{
			name: "invalid manifest field",
			manifest: `
name = "Bad Name"
version = "1.0.0"
runtime = "lua"
entry_point = "init.lua"
`,
			fileName:      "init.lua",
			wantErr:       "manifest validation failed",
			wantOutput:    "name",
			createProject: true,
		},
		{
			name: "unknown permission",
			manifest: `
name = "bad-permission"
version = "1.0.0"
runtime = "lua"
entry_point = "init.lua"
permissions = ["secrets"]
`,
			fileName:      "init.lua",
			wantErr:       "unknown permission",
			wantOutput:    "secrets",
			createProject: true,
		},
		{
			name: "missing entry point",
			manifest: `
name = "missing-entry"
version = "1.0.0"
runtime = "lua"
entry_point = "init.lua"
`,
			wantErr:       "does not exist",
			wantOutput:    "entry_point",
			createProject: true,
		},
		{
			name: "path traversal",
			manifest: `
name = "traversal"
version = "1.0.0"
runtime = "lua"
entry_point = "../init.lua"
`,
			wantErr:       "must not contain '..'",
			wantOutput:    "manifest validation failed",
			createProject: true,
		},
		{
			name:       "nonexistent path",
			path:       filepath.Join(t.TempDir(), "missing"),
			wantErr:    "not accessible",
			wantOutput: "not accessible",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectDir := tt.path
			if tt.createProject {
				projectDir = t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(projectDir, extTestManifest), []byte(tt.manifest), 0o600))
				if tt.fileName != "" {
					require.NoError(t, os.WriteFile(filepath.Join(projectDir, tt.fileName), []byte("-- ok\n"), 0o600))
				}
			}

			cmd := newExtCmd()
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)
			cmd.SetArgs([]string{extValidateCommand, projectDir})

			err := cmd.Execute()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Contains(t, buf.String(), "Extension project is invalid")
			assert.Contains(t, buf.String(), tt.wantOutput)
		})
	}
}

func TestExtValidateCmd_JSONOutput(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, extension.Scaffold(dir, extTestName, "lua"))

	cmd := newExtCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{extValidateCommand, filepath.Join(dir, extTestName), extValidateJSONArg})

	require.NoError(t, cmd.Execute())

	var out extensionValidationJSON
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	require.NotNil(t, out.Manifest)
	assert.Equal(t, extValidationStatusValid, out.Status)
	assert.Equal(t, extTestName, out.Manifest.Name)
	assert.Equal(t, "lua", out.Manifest.Runtime)
	assert.Empty(t, out.Errors)
}

func TestExtValidateCmd_JSONOutputForInvalidProject(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, extTestManifest), []byte(`
name = "json-invalid"
version = "1.0.0"
runtime = "lua"
entry_point = "missing.lua"
`), 0o600))

	cmd := newExtCmd()
	var buf bytes.Buffer
	var errBuf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{extValidateCommand, dir, extValidateJSONArg})

	err := cmd.Execute()
	require.Error(t, err)

	var out extensionValidationJSON
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, extValidationStatusInvalid, out.Status)
	assert.NotEmpty(t, out.Errors)
	assert.Contains(t, out.Errors[0], "entry_point")
}

func TestPrintExtensionListJSON_EmptyList(t *testing.T) {
	cmd := newExtListCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := printExtensionListJSON(cmd, nil)
	require.NoError(t, err)

	var out []extensionInventoryJSON
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Empty(t, out)
}

func TestPrintExtensionListJSON_SortedInventory(t *testing.T) {
	cmd := newExtListCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	installed := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)

	err := printExtensionListJSON(cmd, []extension.ExtensionInfo{
		{
			Manifest:    extension.Manifest{Name: "zeta", Version: "1.0.0", Runtime: "lua"},
			Dir:         "z-dir",
			Enabled:     false,
			InstalledAt: installed,
		},
		{
			Manifest:    extension.Manifest{Name: "alpha", Version: "2.0.0", Runtime: "wasm", Permissions: []string{"notify"}},
			Dir:         "a-dir",
			Enabled:     true,
			InstalledAt: installed,
			SourceURL:   "https://github.com/example/alpha",
			CommitHash:  "abc123",
		},
	})
	require.NoError(t, err)

	var out []extensionInventoryJSON
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	require.Len(t, out, 2)
	assert.Equal(t, "alpha", out[0].Name)
	assert.Equal(t, "wasm", out[0].Runtime)
	assert.Equal(t, []string{"notify"}, out[0].Permissions)
	assert.True(t, out[0].Enabled)
	assert.Equal(t, "https://github.com/example/alpha", out[0].SourceURL)
	assert.Equal(t, "abc123", out[0].CommitHash)
	assert.Equal(t, "a-dir", out[0].Directory)
	assert.Equal(t, "zeta", out[1].Name)
}

func TestPrintExtensionJSON_IncludesManifestAndState(t *testing.T) {
	cmd := newExtInfoCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	installed := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)

	err := printExtensionJSON(cmd, extension.ExtensionInfo{
		Manifest: extension.Manifest{
			Name:        "demo",
			Version:     "1.2.3",
			Description: "Demo extension",
			Author:      "Grut",
			License:     "MIT",
			Runtime:     "lua",
			EntryPoint:  "init.lua",
			MinGrut:     "0.1.0",
			Permissions: []string{"notify", "git_read"},
		},
		Dir:         "demo-dir",
		Enabled:     true,
		InstalledAt: installed,
		SourceURL:   "https://github.com/example/demo",
		CommitHash:  "def456",
	})
	require.NoError(t, err)

	var out extensionInventoryJSON
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "demo", out.Name)
	assert.Equal(t, "1.2.3", out.Version)
	assert.Equal(t, "lua", out.Runtime)
	assert.Equal(t, "init.lua", out.EntryPoint)
	assert.Equal(t, []string{"notify", "git_read"}, out.Permissions)
	assert.True(t, out.Enabled)
	assert.Equal(t, "demo-dir", out.Directory)
	assert.Equal(t, installed, out.InstalledAt)
}
