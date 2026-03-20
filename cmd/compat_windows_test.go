//go:build windows

package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckTerminalCompat_NoMSYSTEM(t *testing.T) {
	// When MSYSTEM is not set (PowerShell, cmd, etc.), the check should pass.
	t.Setenv("MSYSTEM", "")
	assert.Empty(t, checkTerminalCompat(), "should return empty when MSYSTEM is unset")
}

func TestCheckTerminalCompat_MinTTY(t *testing.T) {
	// Simulate standalone Git Bash (MinTTY): MSYSTEM is set but no
	// ConPTY terminal env vars are present.
	t.Setenv("MSYSTEM", "MINGW64")
	t.Setenv("WT_SESSION", "")
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("ConEmuPID", "")
	t.Setenv("GRUT_FORCE_TERMINAL", "")

	msg := checkTerminalCompat()
	require.NotEmpty(t, msg, "should return error message for MinTTY")

	// Verify the message contains key information.
	for _, want := range []string{
		"MinTTY",
		"MSYS2 pseudo-TTY",
		"PowerShell",
		"Windows Terminal",
		"winpty",
		"https://www.msys2.org/docs/terminals/",
	} {
		assert.Contains(t, msg, want)
	}
}

func TestCheckTerminalCompat_WindowsTerminal(t *testing.T) {
	// Git Bash shell inside Windows Terminal — WT_SESSION is set.
	t.Setenv("MSYSTEM", "MINGW64")
	t.Setenv("WT_SESSION", "some-guid")
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("ConEmuPID", "")

	assert.Empty(t, checkTerminalCompat(), "should pass for Windows Terminal")
}

func TestCheckTerminalCompat_VSCode(t *testing.T) {
	// Git Bash shell inside VS Code integrated terminal.
	t.Setenv("MSYSTEM", "MINGW64")
	t.Setenv("WT_SESSION", "")
	t.Setenv("TERM_PROGRAM", "vscode")
	t.Setenv("ConEmuPID", "")

	assert.Empty(t, checkTerminalCompat(), "should pass for VS Code terminal")
}

func TestCheckTerminalCompat_VSCodeCaseInsensitive(t *testing.T) {
	// TERM_PROGRAM comparison should be case-insensitive.
	t.Setenv("MSYSTEM", "MINGW64")
	t.Setenv("WT_SESSION", "")
	t.Setenv("TERM_PROGRAM", "VSCode")
	t.Setenv("ConEmuPID", "")

	assert.Empty(t, checkTerminalCompat(), "should be case-insensitive for TERM_PROGRAM=VSCode")
}

func TestCheckTerminalCompat_ConEmu(t *testing.T) {
	// Git Bash shell inside ConEmu/Cmder.
	t.Setenv("MSYSTEM", "MINGW64")
	t.Setenv("WT_SESSION", "")
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("ConEmuPID", "12345")

	assert.Empty(t, checkTerminalCompat(), "should pass for ConEmu")
}

func TestCheckTerminalCompat_UnlistedTerminal(t *testing.T) {
	// MSYSTEM set with a TERM_PROGRAM not in the ConPTY allowlist.
	// The check blocks because it cannot confirm ConPTY support.
	//
	// NOTE: this is a known limitation — ConPTY-capable terminals like
	// Alacritty or WezTerm that set TERM_PROGRAM but are not in the
	// allowlist will be falsely blocked. Users can work around this by
	// setting GRUT_FORCE_TERMINAL=1 (see TestCheckTerminalCompat_ForceTerminalBypass).
	t.Setenv("MSYSTEM", "MINGW64")
	t.Setenv("WT_SESSION", "")
	t.Setenv("TERM_PROGRAM", "some-unlisted-terminal")
	t.Setenv("ConEmuPID", "")

	assert.NotEmpty(t, checkTerminalCompat(), "should block for TERM_PROGRAM not in ConPTY allowlist")
}

func TestIsConPTYTerminal_AllUnset(t *testing.T) {
	t.Setenv("WT_SESSION", "")
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("ConEmuPID", "")

	assert.False(t, isConPTYTerminal(), "should return false when no ConPTY env vars are set")
}

func TestIsConPTYTerminal_WTSession(t *testing.T) {
	t.Setenv("WT_SESSION", "abc-123")
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("ConEmuPID", "")

	assert.True(t, isConPTYTerminal(), "should return true when WT_SESSION is set")
}

func TestIsConPTYTerminal_VSCode(t *testing.T) {
	// Direct test for TERM_PROGRAM=vscode in isConPTYTerminal.
	t.Setenv("WT_SESSION", "")
	t.Setenv("TERM_PROGRAM", "vscode")
	t.Setenv("ConEmuPID", "")

	assert.True(t, isConPTYTerminal(), "should return true when TERM_PROGRAM=vscode")
}

func TestIsConPTYTerminal_ConEmuPID(t *testing.T) {
	t.Setenv("WT_SESSION", "")
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("ConEmuPID", "9999")

	assert.True(t, isConPTYTerminal(), "should return true when ConEmuPID is set")
}

// TestCheckTerminalCompat_MSYSTEMVariants ensures detection works for
// different MSYSTEM values (MINGW32, MINGW64, UCRT64, CLANG64, etc.)
func TestCheckTerminalCompat_MSYSTEMVariants(t *testing.T) {
	variants := []string{"MINGW32", "MINGW64", "UCRT64", "CLANG64", "MSYS"}
	for _, v := range variants {
		t.Run(v, func(t *testing.T) {
			t.Setenv("MSYSTEM", v)
			t.Setenv("WT_SESSION", "")
			t.Setenv("TERM_PROGRAM", "")
			t.Setenv("ConEmuPID", "")

			assert.NotEmpty(t, checkTerminalCompat(), "should block for MSYSTEM=%s without ConPTY", v)
		})
	}
}

// TestCheckTerminalCompat_MessageMentionsGrut ensures the error message
// references grut (not dispatch or another tool).
func TestCheckTerminalCompat_MessageMentionsGrut(t *testing.T) {
	t.Setenv("MSYSTEM", "MINGW64")
	t.Setenv("WT_SESSION", "")
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("ConEmuPID", "")
	t.Setenv("GRUT_FORCE_TERMINAL", "")

	msg := checkTerminalCompat()
	assert.Contains(t, msg, "grut")
	assert.Contains(t, msg, "GRUT_FORCE_TERMINAL")
}

// TestCheckTerminalCompat_ForceTerminalBypass verifies the escape hatch:
// GRUT_FORCE_TERMINAL set to any non-empty value bypasses the MinTTY check,
// allowing users with ConPTY-capable terminals not in the allowlist
// (e.g. Alacritty, WezTerm) to run grut.
func TestCheckTerminalCompat_ForceTerminalBypass(t *testing.T) {
	t.Setenv("MSYSTEM", "MINGW64")
	t.Setenv("WT_SESSION", "")
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("ConEmuPID", "")
	t.Setenv("GRUT_FORCE_TERMINAL", "1")

	assert.Empty(t, checkTerminalCompat(), "GRUT_FORCE_TERMINAL=1 should bypass MinTTY check")
}

// ---------------------------------------------------------------------------
// isConPTYTerminal — additional direct unit tests
// ---------------------------------------------------------------------------

// TestIsConPTYTerminal_CaseInsensitive verifies that every capitalisation of
// "vscode" is accepted by isConPTYTerminal directly. strings.EqualFold must
// handle all variants.
func TestIsConPTYTerminal_CaseInsensitive(t *testing.T) {
	variants := []string{"vscode", "VSCODE", "VSCode", "VsCode", "vSCODE"}
	for _, v := range variants {
		t.Run(v, func(t *testing.T) {
			t.Setenv("WT_SESSION", "")
			t.Setenv("TERM_PROGRAM", v)
			t.Setenv("ConEmuPID", "")

			assert.True(t, isConPTYTerminal(), "isConPTYTerminal() should return true for TERM_PROGRAM=%q", v)
		})
	}
}

// TestIsConPTYTerminal_MultipleSignals verifies correct behaviour when more
// than one ConPTY indicator is present simultaneously (e.g. VS Code inside
// Windows Terminal with ConEmu integration active). Any single indicator is
// sufficient; this confirms the OR logic holds across all three branches.
func TestIsConPTYTerminal_MultipleSignals(t *testing.T) {
	t.Setenv("WT_SESSION", "some-guid")
	t.Setenv("TERM_PROGRAM", "vscode")
	t.Setenv("ConEmuPID", "1234")

	assert.True(t, isConPTYTerminal(), "isConPTYTerminal() should return true when multiple ConPTY env vars are set")
}

// ---------------------------------------------------------------------------
// Integration wiring: root.go RunE — subprocess test
// ---------------------------------------------------------------------------

// TestRootCmd_MinTTYRejected_Subprocess spawns a child "go run ." process
// with MSYSTEM=MINGW64 and no ConPTY vars, then asserts the process exits
// non-zero and emits the MinTTY diagnostic.
//
// A subprocess is required because root.go's RunE calls redirectStderr() which
// invokes windows.SetStdHandle(STD_ERROR_HANDLE, ...) — a process-wide mutation
// that would corrupt the test runner's own output capture if done in-process.
func TestRootCmd_MinTTYRejected_Subprocess(t *testing.T) {
	if testing.Short() {
		t.Skip("subprocess integration test skipped with -short")
	}

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve project root: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "main.go")); statErr != nil {
		t.Skipf("cannot locate main.go at %s: %v", root, statErr)
	}

	// Inherit env, but strip and override the four compat vars.
	env := make([]string, 0, len(os.Environ()))
	for _, kv := range os.Environ() {
		key := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			key = kv[:i]
		}
		switch key {
		case "MSYSTEM", "WT_SESSION", "TERM_PROGRAM", "ConEmuPID":
		// stripped — set explicitly below
		default:
			env = append(env, kv)
		}
	}
	env = append(env,
		"MSYSTEM=MINGW64",
		"WT_SESSION=",
		"TERM_PROGRAM=",
		"ConEmuPID=",
	)

	cmd := exec.Command("go", "run", ".")
	cmd.Dir = root
	cmd.Env = env
	out, runErr := cmd.CombinedOutput()

	require.Error(t, runErr, "grut should exit non-zero in MinTTY environment")
	assert.Contains(t, string(out), "MinTTY",
		"expected MinTTY diagnostic in combined output; got:\n%s", out)
	assert.Contains(t, string(out), "ALTERNATIVES",
		"expected ALTERNATIVES section in combined output; got:\n%s", out)
}
