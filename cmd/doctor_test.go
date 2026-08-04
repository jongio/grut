package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jongio/grut/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoctorPassingConfig(t *testing.T) {
	out, err := runDoctorTest(t, doctorTestDeps(&config.Config{
		Theme: config.ThemeConfig{Name: "default"},
		AI:    config.AIConfig{Enabled: true, Provider: "copilot"},
	}), nil)

	require.NoError(t, err)
	assert.Contains(t, out, "grut doctor")
	assert.Contains(t, out, "Config")
	assert.Contains(t, out, "default ready")
	assert.Contains(t, out, "GitHub auth")
	assert.Contains(t, out, "All required checks passed.")
}

func TestNewDoctorCmd_Wiring(t *testing.T) {
	cmd := newDoctorCmd()
	assert.Equal(t, "doctor", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	require.NotNil(t, cmd.Flags().Lookup("json"), "--json flag should be registered")
	require.NotNil(t, cmd.Flags().Lookup("check"), "--check flag should be registered")
}

func TestDoctorInvalidConfigFailsRequiredCheck(t *testing.T) {
	deps := doctorTestDeps(nil)
	deps.loadConfig = func() (*config.Config, error) {
		return nil, errors.New("config validation: preview.width must be 1-100")
	}

	out, err := runDoctorTest(t, deps, []string{"--json"})

	require.NoError(t, err)
	var report doctorReport
	require.NoError(t, json.Unmarshal([]byte(out), &report))
	assert.False(t, report.OK)
	require.NotEmpty(t, report.Checks)
	assert.Equal(t, doctorFail, report.Checks[0].Status)
	assert.Contains(t, report.Checks[0].Fix, "grut config defaults")
}

func TestDoctorMissingGitHubAuthWarnsWithoutFailing(t *testing.T) {
	deps := doctorTestDeps(&config.Config{
		Theme: config.ThemeConfig{Name: "default"},
		AI:    config.AIConfig{Enabled: true, Provider: "copilot"},
	})
	deps.runCommand = func(_ context.Context, name string, args ...string) (string, string, error) {
		if name == "gh" {
			return "", "not logged in", errors.New("exit status 1")
		}
		return fakeDoctorCommand(name, args...)
	}
	deps.getenv = func(key string) string {
		if key == "TERM" {
			return "xterm-256color"
		}
		return ""
	}

	out, err := runDoctorTest(t, deps, nil)

	require.NoError(t, err)
	assert.Contains(t, out, "GitHub auth")
	assert.Contains(t, out, "WARN")
	assert.Contains(t, out, "gh auth login")
	assert.Contains(t, out, "All required checks passed.")
}

func TestDoctorRequiredFailureReportsFailure(t *testing.T) {
	deps := doctorTestDeps(&config.Config{
		Theme: config.ThemeConfig{Name: "default"},
		AI:    config.AIConfig{Enabled: false, Provider: "none"},
	})
	deps.runCommand = func(_ context.Context, name string, args ...string) (string, string, error) {
		if name == "git" && len(args) == 1 && args[0] == "--version" {
			return "", "git missing", errors.New("not found")
		}
		return fakeDoctorCommand(name, args...)
	}

	out, err := runDoctorTest(t, deps, nil)

	require.NoError(t, err)
	assert.Contains(t, out, "Git")
	assert.Contains(t, out, "FAIL")
	assert.Contains(t, out, "One or more required checks failed.")
}

// --check is an exit-code gate, matching "status --check" and "clean --check".
// The report still prints; only the exit code changes.
func TestDoctorCheckPassingRequiredChecksPrintsReport(t *testing.T) {
	out, err := runDoctorTest(t, doctorTestDeps(&config.Config{
		Theme: config.ThemeConfig{Name: "default"},
		AI:    config.AIConfig{Enabled: false, Provider: "none"},
	}), []string{"--check"})

	require.NoError(t, err)
	assert.Contains(t, out, "All required checks passed.")
}

func TestDoctorCheckRequiredFailureExitsNonZeroAndStillPrints(t *testing.T) {
	deps := doctorTestDeps(&config.Config{
		Theme: config.ThemeConfig{Name: "default"},
		AI:    config.AIConfig{Enabled: false, Provider: "none"},
	})
	deps.runCommand = func(_ context.Context, name string, args ...string) (string, string, error) {
		if name == "git" && len(args) == 1 && args[0] == "--version" {
			return "", "git missing", errors.New("not found")
		}
		return fakeDoctorCommand(name, args...)
	}

	out, err := runDoctorTest(t, deps, []string{"--check"})

	require.ErrorIs(t, err, errDoctorRequiredChecksFailed)
	assert.Contains(t, out, "One or more required checks failed.")
}

// Without --check, doctor is a diagnostic: it reports failures but exits 0 so
// it stays usable mid-pipeline. Covered by
// TestDoctorRequiredFailureReportsFailure above.

func TestDoctorCheckJSONStillPrintsJSON(t *testing.T) {
	out, err := runDoctorTest(t, doctorTestDeps(&config.Config{
		Theme: config.ThemeConfig{Name: "default"},
		AI:    config.AIConfig{Enabled: false, Provider: "none"},
	}), []string{"--check", "--json"})

	require.NoError(t, err)
	var report doctorReport
	require.NoError(t, json.Unmarshal([]byte(out), &report))
	assert.True(t, report.OK)
	assert.NotEmpty(t, report.Checks)
}

func TestDoctorJSONStableFields(t *testing.T) {
	out, err := runDoctorTest(t, doctorTestDeps(&config.Config{
		Theme: config.ThemeConfig{Name: "default"},
		AI:    config.AIConfig{Enabled: true, Provider: "claude"},
	}), []string{"--json"})

	require.NoError(t, err)
	var report doctorReport
	require.NoError(t, json.Unmarshal([]byte(out), &report))
	assert.True(t, report.OK)
	assert.Equal(t, `C:\Users\me\AppData\Roaming\grut\config.toml`, report.ConfigPath)
	assert.Equal(t, "default", report.SelectedTheme)
	assert.Equal(t, "gh", report.GitHubAuthSource)
	assert.Equal(t, "claude", report.AIProvider)
	assert.NotEmpty(t, report.Checks)
}

func TestHasFailedRequiredDoctorCheck(t *testing.T) {
	tests := []struct {
		name   string
		checks []doctorCheck
		want   bool
	}{
		{name: "pass", checks: []doctorCheck{{Status: doctorPass, Required: true}}, want: false},
		{name: "warn", checks: []doctorCheck{{Status: doctorWarn, Required: true}}, want: false},
		{name: "optional fail", checks: []doctorCheck{{Status: doctorFail}}, want: false},
		{name: "required fail", checks: []doctorCheck{{Status: doctorFail, Required: true}}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, hasFailedRequiredDoctorCheck(tt.checks))
		})
	}
}

func TestRootRegistersDoctor(t *testing.T) {
	root, cleanup := newRootCommand()
	defer cleanup()

	doctorCmd, _, err := root.Find([]string{"doctor"})
	require.NoError(t, err)
	require.NotNil(t, doctorCmd)
	assert.Equal(t, "doctor", doctorCmd.Name())
}

func runDoctorTest(t *testing.T, deps doctorCommandDeps, args []string) (string, error) {
	t.Helper()
	cmd := newDoctorCmdWithDeps(deps)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func doctorTestDeps(cfg *config.Config) doctorCommandDeps {
	if cfg == nil {
		cfg = &config.Config{}
	}
	if cfg.Theme.Name == "" {
		cfg.Theme.Name = "default"
	}
	return doctorCommandDeps{
		loadConfig: func() (*config.Config, error) { return cfg, nil },
		configPath: func() string {
			return `C:\Users\me\AppData\Roaming\grut\config.toml`
		},
		configDir: func() string { return `C:\Users\me\AppData\Roaming\grut` },
		dataDir:   func() string { return `C:\Users\me\AppData\Local\grut` },
		loadTheme: func(name string) (string, error) {
			if name == "broken" {
				return "", errors.New("theme missing")
			}
			return name, nil
		},
		runCommand: func(_ context.Context, name string, args ...string) (string, string, error) {
			return fakeDoctorCommand(name, args...)
		},
		getwd: func() (string, error) { return `C:\code\repo`, nil },
		getenv: func(key string) string {
			switch key {
			case "GITHUB_TOKEN":
				return "token"
			case "ANTHROPIC_API_KEY":
				return "token"
			case "TERM":
				return "xterm-256color"
			default:
				return ""
			}
		},
		checkTerminal: func() string { return "" },
		checkDir:      func(string) (bool, bool, error) { return true, true, nil },
	}
}

func fakeDoctorCommand(name string, args ...string) (string, string, error) {
	if name == "git" && len(args) == 1 && args[0] == "--version" {
		return "git version 2.50.0\n", "", nil
	}
	if name == "git" && len(args) == 2 && args[0] == "rev-parse" {
		return `C:\code\repo` + "\n", "", nil
	}
	if name == "gh" && len(args) == 2 && args[0] == "auth" && args[1] == "token" {
		return "gh-token\n", "", nil
	}
	return "", "", errors.New("unexpected command")
}
