//go:build mage

// This magefile handles building, testing, and installing grut.
//
// On Windows, Go's temp binaries in %TEMP% can be blocked by Defender.
// The init() function below automatically sets GOTMPDIR to a project-local
// directory (bin/.tmp) so `mage install` works without manual env setup.

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var (
	binName    = "grut-dev"
	mainPkg    = "."
	versionVar = "github.com/jongio/grut/internal/config.AppVersion"
)

const (
	windowsRaceToolchainVersion         = "2.8.0"
	windowsRaceToolchainSHA256          = "6252bf34fe2231a55ac7f03d482b36d2c7c58697990551bba508102cfb3f342e"
	windowsRaceToolchainDownloadTimeout = 10 * time.Minute
	windowsRaceToolchainExtractTimeout  = 10 * time.Minute
)

var validGoVersionRE = regexp.MustCompile(`^go\d+\.\d+(\.\d+)?(rc\d+|beta\d+)?$`)

func init() {
	// On Windows, Defender may block Go temp binaries compiled into %TEMP%.
	// Redirect GOTMPDIR to a project-local directory to avoid this.
	if os.Getenv("GOTMPDIR") != "" {
		return
	}
	tmpDir := filepath.Join(projectDir(), "bin", ".tmp")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to create GOTMPDIR %s: %v\n", tmpDir, err)
		return
	}
	os.Setenv("GOTMPDIR", tmpDir)
}

// deadcodeAllowlist contains functions reported by deadcode that are not
// genuinely dead: interface implementations dispatched via panels.Panel or
// extension.Runtime, factory-constructed runtimes, build-tag stubs, WASM
// host-function callbacks, and test-only accessors providing public API surface.
var deadcodeAllowlist = []string{
	// config — test-only accessor (used in app_test.go, engine_test.go)
	"LoadDefaults",

	// preview — test-only helper (used in editor_render_test.go)
	"highlightLine",

	// cmd — test-only accessor (returns root command + cleanup for tests)
	"newRootCommand",

	// crashlog — test-only accessors
	"CollectDiagnostics",
	"RecoverAndReport",

	// filetree — test accessor methods (not wired to key bindings yet)
	"FileTree.bookmarkCurrent",
	"FileTree.addToContext",

	// actions registry — public API getter
	"Description",

	// extension permissions — public API + error interface impl
	"CheckPermission",
	"AllPermissions",
	"ManifestHasPermission",
	"PermissionDeniedError.Error",

	// extension/runtime/lua — loaded via runtime factory; interface impls
	"NewLuaRuntime",
	"LuaRuntime.SetTimeout",
	"LuaRuntime.Load",
	"LuaRuntime.Close",
	"LuaRuntime.Name",
	"LuaRuntime.sandbox",
	"LuaRuntime.registerHostAPI",
	"LuaRuntime.luaToast",
	"LuaRuntime.luaRegisterCommand",
	"LuaRuntime.luaSetStatus",
	"LuaRuntime.execWithTimeout",
	"ValidateEntryPoint",

	// extension/runtime/mcp — loaded via runtime factory; interface impls
	"ringBuffer.Write",  // io.Writer
	"ringBuffer.String", // fmt.Stringer
	"NewMCPRuntime",
	"MCPRuntime.Name",
	"MCPRuntime.Load",
	"MCPRuntime.Close",
	"MCPRuntime.Running",
	"MCPRuntime.SendRequest",
	"resolveCommand",
	"filterEnvForSubprocess",
	// mcp_procattr_windows.go — build-tag no-op stubs; called from Load/Close
	// but unreachable under deadcode's Windows call-graph analysis because
	// MCPRuntime.Load itself is factory-dispatched (also allowlisted above).
	"setProcGroup",
	"killProcGroup",
	// mcp_procattr_unix.go — same pattern, unreachable under Linux analysis
	"postStartProcGroup",

	// extension/runtime/wasm — loaded via runtime factory; interface impls
	"NewWASMRuntime",
	"WASMRuntime.Name",
	"WASMRuntime.Load",
	"WASMRuntime.loadBytes",
	"WASMRuntime.Close",
	"WASMRuntime.registerHostFunctions",
	"readString",    // WASM host-function callback helper
	"levelToString", // WASM host-function callback helper

	// keymap — fmt.Stringer interface impl
	"Conflict.String",

	// layout — tab bar rendering (v2 multi-tab support)
	"RenderTabBar",

	// mcp — interface impls / public API
	"AuditLogger.Close", // io.Closer
	"Server.MCPServer",

	// notify — boundary helpers, public API
	"SafeUpdate",
	"SafeView",
	"renderErrorState",
	"truncateUTF8",

	// panels/aiconflict — panels.Panel interface impl + internal methods
	"New",
	"Panel.Init",
	"Panel.Update",
	"Panel.handleSetConflicts",
	"Panel.handleKey",
	"Panel.regionCount",
	"Panel.nextRegion",
	"Panel.prevRegion",
	"Panel.storeChoice",
	"Panel.acceptAI",
	"Panel.chooseOurs",
	"Panel.chooseTheirs",
	"Panel.hasAIResolution",
	"Panel.View",
	"Panel.buildLines",
	"Panel.renderSectionHeader",
	"Panel.renderCodeBlock",
	"Panel.renderKeyHints",
	"Panel.headerHex",
	"Panel.regionHex",
	"Panel.oursHex",
	"Panel.theirsHex",
	"Panel.aiHex",
	"Panel.dimHex",
	"Panel.successHex",
	"Panel.hintHex",
	"Panel.hintBgHex",
	"Panel.KeyBindings",
	"Panel.initStyles",
	"Panel.sectionHeaderStyle",
	"Panel.codeBlockStyle",
	"Panel.themeColor",
	"colorOrDefault",

	// panel accessor methods — public API / test accessors across panels
	"Panel.cursorIndex",
	"Panel.itemCount",
	"Panel.fileCount",
	"Panel.currentPosition",
	"Panel.currentThemeName",
	"Panel.currentActionOverride",
	"Panel.currentRightClickOverride",

	// filetree — accessor methods + build-tag/platform stub
	"normalizeVolume",
	"FileTree.cursorIndex",
	"FileTree.visibleCount",
	"FileTree.visibleName",
	"FileTree.visiblePath",
	"FileTree.isPathSelected",
	"FileTree.showHiddenState",

	// fuzzyfinder — accessor methods
	"FuzzyFinder.matchCount",
	"FuzzyFinder.cursorIndex",
	"FuzzyFinder.selectedItem",
	"FuzzyFinder.queryValue",

	// tui — tab management, called via key dispatch
	"Model.handleNewTab",
	"Model.handleSwitchOrCreateTab",
	"Model.handleCloseTab",

	// ai — token estimation (public API / test accessors)
	"tokenBudget.remaining",
	"estimateTokensForStatus",

	// bookmarks — public factory for testing
	"NewManagerWithDir",

	// chat — public API
	"FormatConfirmationPrompt",

	// git — public API / helpers
	"NewClientWithCache",
	"splitLines",
	"ParseStatusBranch",
	"JoinPaths",

	// github — exec helper
	"ghExec",

	// notify — public API + test accessors
	"ShowConfirmWithCheckbox",
	"Manager.toastDuration",
	"Manager.toastID",
	"Manager.toastIDs",
	"Manager.toastMessage",
	"Manager.inlineMessage",
	"Manager.inlineLevel",

	// commits — test accessors + helpers
	"Panel.showDetail",
	"Panel.signatureDetail",
	"relativeDate",

	// filetree — test accessors
	"FileTree.goToTop",
	"watcher.removeDir",
	"watcher.isPolling",

	// gitinfo — internal helpers
	"Panel.rebuildFromCurrent",

	// help — test accessors
	"Panel.scrollOffset",
	"Panel.lineCount",

	// session — public test-support API
	"IsFirstRunIn",
	"MarkFirstRunDoneIn",
	"ResetFirstRunIn",

	// theme — public API
	"LoadFromFile",
	"BuiltinNames",

	// mcp/server — test-only helper for validating path arrays
	"validateGitPaths",

	// fuzzyfinder/source — test-support cache invalidation API
	"fileCache.invalidate",
	"InvalidateFileCache",

	// panelreg — test-only reset helper
	"Reset",

	// update — internal checksum verification
	"verifyChecksum",

	// preview/editor — test-only save handler
	"handleFileSaved",

	// gitinfo — test-only color/icon helpers (used in gitinfo_test.go)
	"prColor",
	"prActionIcon",

	// gitstatus — test-only accessor (used in gitstatus_extra_test.go)
	"GitStatus.fileColor",

	// git/gittest — shared mock implementing full GitClient interface;
	// many methods are interface stubs not yet exercised by tests
	"MockClient.Status",
	"MockClient.Diff",
	"MockClient.Log",
	"MockClient.Blame",
	"MockClient.RepoRoot",
	"MockClient.IsRepo",
	"MockClient.DiffTreeFiles",
	"MockClient.DiffFileNames",
	"MockClient.Stage",
	"MockClient.Unstage",
	"MockClient.StageHunk",
	"MockClient.UnstageHunk",
	"MockClient.StageLine",
	"MockClient.UnstageLine",
	"MockClient.Commit",
	"MockClient.BranchList",
	"MockClient.CurrentBranch",
	"MockClient.BranchCreate",
	"MockClient.BranchDelete",
	"MockClient.BranchRename",
	"MockClient.Checkout",
	"MockClient.Push",
	"MockClient.Pull",
	"MockClient.Fetch",
	"MockClient.RemoteList",
	"MockClient.RemoteAdd",
	"MockClient.RemoteRemove",
	"MockClient.WorktreeList",
	"MockClient.WorktreeAdd",
	"MockClient.WorktreeRemove",
	"MockClient.StashList",
	"MockClient.StashShow",
	"MockClient.StashPush",
	"MockClient.StashPop",
	"MockClient.StashApply",
	"MockClient.StashDrop",
	"MockClient.TagList",
	"MockClient.TagCreate",
	"MockClient.TagDelete",
	"MockClient.TagListRemote",
	"MockClient.TagPush",
	"MockClient.TagPushAll",
	"MockClient.Merge",
	"MockClient.MergeAbort",
	"MockClient.Rebase",
	"MockClient.RebaseContinue",
	"MockClient.RebaseAbort",
	"MockClient.CherryPick",
	"MockClient.BisectStart",
	"MockClient.BisectGood",
	"MockClient.BisectBad",
	"MockClient.BisectReset",
	"MockClient.Reflog",
	"MockClient.DiscardFile",
	"MockClient.DiscardAllUnstaged",
	"MockClient.Revert",
	"MockClient.RevertContinue",
	"MockClient.RevertAbort",
	"MockClient.Reset",
	"MockClient.Clean",
	"MockClient.CleanPreview",

	// git — exported plain-text summary method on DiffStat (public package API;
	// the preview renders a colored variant, so this is unused internally)
	"DiffStat.Summary",
}

// Default target when running `mage` with no args.
var Default = Install

// Install runs tests, kills stale processes, builds the dev binary, and ensures it's in PATH.
func Install() error {
	if err := Test(); err != nil {
		return err
	}
	killStale()
	if err := Build(); err != nil {
		return err
	}
	if err := ensurePath(); err != nil {
		return err
	}
	return verify()
}

// Test runs all unit tests.
func Test() error {
	fmt.Println("\n=== Running tests ===")
	if err := run("go", "test", "./...", "-count=1"); err != nil {
		return err
	}
	return runMagefileTests()
}

func runMagefileTests() error {
	fmt.Println("\n=== Running magefile tests ===")
	return run("go", "test", "-tags", "mage", "magefile.go", "magefile_test.go")
}

// CoverageReport generates an HTML coverage report and opens it in a browser.
func CoverageReport() error {
	fmt.Println("\n=== Generating coverage report ===")
	binDir := filepath.Join(projectDir(), "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return fmt.Errorf("creating bin directory: %w", err)
	}

	coverFile := filepath.Join(binDir, "coverage.out")
	if err := run("go", "test", "-coverprofile="+coverFile, "-covermode=atomic", "./..."); err != nil {
		return fmt.Errorf("coverage: %w", err)
	}

	htmlFile := filepath.Join(binDir, "coverage.html")
	if err := run("go", "tool", "cover", "-html="+coverFile, "-o", htmlFile); err != nil {
		return fmt.Errorf("coverage html: %w", err)
	}

	fmt.Printf("   Report: %s\n", htmlFile)

	// Open in browser on supported platforms (best-effort).
	switch runtime.GOOS {
	case "windows":
		_ = exec.Command("cmd", "/c", "start", htmlFile).Start()
	case "darwin":
		_ = exec.Command("open", htmlFile).Start()
	default:
		_ = exec.Command("xdg-open", htmlFile).Start()
	}

	return nil
}

// TestWSL runs the test suite under WSL Linux to exercise Unix-specific code
// paths (build-tag stubs, path handling, etc.). Requires WSL to be installed.
func TestWSL() error {
	fmt.Println("\n=== Testing under WSL ===")
	if runtime.GOOS != "windows" {
		fmt.Println("   Skipped (only available on Windows with WSL)")
		return nil
	}

	if _, err := exec.LookPath("wsl"); err != nil {
		fmt.Println("   Skipped (WSL not installed)")
		return nil
	}

	// Convert the Windows project directory to a WSL-compatible path.
	// filepath.ToSlash converts backslashes to forward slashes, which is
	// required for wslpath on WSL 1 — passing single-backslash Windows paths
	// causes wslpath to strip the backslashes and return exit status 1.
	// Use a short timeout: if WSL doesn't respond in 30s it is not usable.
	wslPath, err := cmdOutputTimeout(30*time.Second, "wsl", "wslpath", "-u", filepath.ToSlash(projectDir()))
	if err != nil {
		if os.Getenv("WSL_SKIP") != "" {
			fmt.Println("   Skipped (WSL did not respond; WSL_SKIP set)")
			return nil
		}
		return fmt.Errorf("wslpath: WSL did not respond within 30s — is WSL running? (set WSL_SKIP=1 to skip) (%w)", err)
	}
	wslPath = strings.TrimSpace(wslPath)

	fmt.Printf("   Project (WSL): %s\n", wslPath)
	if err := ensureWSLGoToolchain(); err != nil {
		return fmt.Errorf("wsl go toolchain: %w", err)
	}

	// Single-quote the path for bash to prevent injection via directory names
	// containing $(), backticks, or other shell metacharacters.
	escapedPath := "'" + strings.ReplaceAll(wslPath, "'", "'\\''") + "'"

	// In a git worktree the .git entry is a file containing a Windows-format
	// "gitdir: <path>" pointer.  WSL cannot resolve that Windows path, which
	// causes any git command (and tests that call git) to fail with "not a git
	// repository".  Detect this case, temporarily rewrite .git with the
	// WSL-compatible path, run the tests, then restore the original file.
	// This avoids setting GIT_DIR globally (which would break tests that
	// create their own temporary git repos and inherit the env var).
	gitFile := filepath.Join(projectDir(), ".git")
	var origGitContent []byte
	needsRestore := false
	if content, readErr := os.ReadFile(gitFile); readErr == nil {
		line := strings.TrimSpace(string(content))
		if strings.HasPrefix(line, "gitdir:") {
			winGitDir := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
			wslGitDir, convErr := cmdOutputTimeout(30*time.Second, "wsl", "wslpath", "-u", filepath.ToSlash(winGitDir))
			if convErr == nil {
				wslGitDir = strings.TrimSpace(wslGitDir)
				newContent := "gitdir: " + wslGitDir + "\n"
				if writeErr := os.WriteFile(gitFile, []byte(newContent), 0o644); writeErr == nil {
					origGitContent = content
					needsRestore = true
					fmt.Printf("   .git patched:   gitdir: %s\n", wslGitDir)
				}
			}
		}
	}

	const wslTestTimeout = 10 * time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), wslTestTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "wsl", "bash", "-s")
	cmd.Stdin = strings.NewReader(fmt.Sprintf("cd %s && %sgo test ./... -count=1", escapedPath, wslGoEnvPrefix()))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = projectDir()
	runErr := cmd.Run()

	// Always restore the original .git file, even on test failure.
	if needsRestore {
		if restoreErr := os.WriteFile(gitFile, origGitContent, 0o644); restoreErr != nil {
			fmt.Printf("   WARNING: failed to restore .git: %v\n", restoreErr)
		} else {
			fmt.Println("   .git restored")
		}
	}

	if runErr != nil {
		return fmt.Errorf("wsl test: %w", runErr)
	}

	fmt.Println("   WSL tests passed")
	return nil
}

type wslGoRunner struct {
	run          func(time.Duration, string) error
	output       func(time.Duration, string) (string, error)
	localVersion func() (string, error)
}

func defaultWSLGoRunner() wslGoRunner {
	return wslGoRunner{
		run: func(timeout time.Duration, script string) error {
			return wslBashRunTimeout(timeout, script)
		},
		output: func(timeout time.Duration, script string) (string, error) {
			return wslBashOutputTimeout(timeout, script)
		},
		localVersion: localGoVersion,
	}
}

func ensureWSLGoToolchain() error {
	return defaultWSLGoRunner().ensureGoToolchain()
}

func (r wslGoRunner) ensureGoToolchain() error {
	checkScript := wslGoEnvPrefix() + "command -v go >/dev/null 2>&1"
	if err := r.run(30*time.Second, checkScript); err == nil {
		if out, versionErr := r.output(30*time.Second, wslGoEnvPrefix()+"go version"); versionErr == nil {
			fmt.Printf("   WSL Go: %s\n", strings.TrimSpace(out))
		}
		return nil
	}

	uname, err := r.output(30*time.Second, "uname -m")
	if err != nil {
		return fmt.Errorf("detect WSL architecture: %w", err)
	}
	arch, err := linuxGoArchFromUname(uname)
	if err != nil {
		return err
	}
	version, err := r.localVersion()
	if err != nil {
		return err
	}
	script, err := wslGoInstallScript(version, arch)
	if err != nil {
		return err
	}

	fmt.Printf("   Go missing in WSL; installing %s for linux/%s under ~/.local/share/grut/go\n", version, arch)
	if err := r.run(15*time.Minute, script); err != nil {
		return fmt.Errorf("install Go in WSL: %w", err)
	}
	if err := r.run(30*time.Second, checkScript); err != nil {
		return fmt.Errorf("verify installed Go in WSL: %w", err)
	}
	return nil
}

func wslBashRunTimeout(timeout time.Duration, script string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "wsl", "bash", "-s")
	cmd.Stdin = strings.NewReader(script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = projectDir()
	err := cmd.Run()
	if ctx.Err() != nil {
		return fmt.Errorf("timed out after %s: %w", timeout, ctx.Err())
	}
	return err
}

func wslBashOutputTimeout(timeout time.Duration, script string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "wsl", "bash", "-s")
	cmd.Stdin = strings.NewReader(script)
	cmd.Dir = projectDir()
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return "", fmt.Errorf("timed out after %s: %w", timeout, ctx.Err())
	}
	return string(out), err
}

func localGoVersion() (string, error) {
	out, err := cmdOutput("go", "env", "GOVERSION")
	if err != nil {
		return "", fmt.Errorf("detect local Go version: %w", err)
	}
	version := strings.TrimSpace(out)
	if !validGoVersion(version) {
		return "", fmt.Errorf("unsupported local Go version %q", version)
	}
	return version, nil
}

func linuxGoArchFromUname(uname string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(uname)) {
	case "x86_64", "amd64":
		return "amd64", nil
	case "aarch64", "arm64":
		return "arm64", nil
	case "armv6l", "armv7l":
		return "armv6l", nil
	case "i386", "i686", "386":
		return "386", nil
	default:
		return "", fmt.Errorf("unsupported WSL architecture %q", strings.TrimSpace(uname))
	}
}

func wslGoEnvPrefix() string {
	return `export PATH="$HOME/.local/share/grut/go/bin:$PATH"; `
}

func wslGoInstallScript(version, arch string) (string, error) {
	if !validGoVersion(version) {
		return "", fmt.Errorf("unsupported Go version %q", version)
	}
	if !validGoArch(arch) {
		return "", fmt.Errorf("unsupported Go architecture %q", arch)
	}
	archiveName := fmt.Sprintf("%s.linux-%s.tar.gz", version, arch)
	downloadURL := "https://go.dev/dl/" + archiveName

	return fmt.Sprintf(`set -euo pipefail
version=%s
go_arch=%s
archive_name=%s
download_url=%s
install_root="$HOME/.local/share/grut"
install_dir="$install_root/go"
archive="$install_root/$archive_name"
mkdir -p "$install_root"
if [ ! -x "$install_dir/bin/go" ] || ! "$install_dir/bin/go" version | grep -q " $version "; then
  rm -rf "$install_dir"
  if ! command -v python3 >/dev/null 2>&1; then
    echo "python3 is required to verify the Go download manifest" >&2
    exit 127
  fi
  if ! command -v sha256sum >/dev/null 2>&1; then
    echo "sha256sum is required to verify the Go archive" >&2
    exit 127
  fi
  if ! command -v tar >/dev/null 2>&1; then
    echo "tar is required to install Go in WSL" >&2
    exit 127
  fi
  if command -v curl >/dev/null 2>&1; then
    curl -fL --retry 3 --connect-timeout 20 -o "$archive" "$download_url"
  elif command -v wget >/dev/null 2>&1; then
    wget -O "$archive" "$download_url"
  else
    echo "curl or wget is required to install Go in WSL" >&2
    exit 127
  fi
  checksum="$(
    GRUT_GO_VERSION="$version" GRUT_GO_ARCH="$go_arch" python3 - <<'PY'
import json
import os
import sys
import urllib.request

version = os.environ["GRUT_GO_VERSION"]
filename = f"{version}.linux-{os.environ['GRUT_GO_ARCH']}.tar.gz"
with urllib.request.urlopen("https://go.dev/dl/?mode=json&include=all", timeout=30) as response:
    releases = json.load(response)
for release in releases:
    if release.get("version") != version:
        continue
    for file_info in release.get("files", []):
        if file_info.get("filename") == filename:
            print(file_info["sha256"])
            sys.exit(0)
print(f"checksum not found for {filename}", file=sys.stderr)
sys.exit(1)
PY
  )"
  actual="$(sha256sum "$archive" | awk '{print $1}')"
  if [ "$checksum" != "$actual" ]; then
    echo "checksum mismatch for $archive_name" >&2
    echo "expected: $checksum" >&2
    echo "actual:   $actual" >&2
    exit 1
  fi
  tar -C "$install_root" -xzf "$archive"
fi
"$install_dir/bin/go" version
`, shQuote(version), shQuote(arch), shQuote(archiveName), shQuote(downloadURL)), nil
}

func validGoVersion(version string) bool {
	return validGoVersionRE.MatchString(version)
}

func validGoArch(arch string) bool {
	switch arch {
	case "386", "amd64", "arm64", "armv6l":
		return true
	default:
		return false
	}
}

func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// Vet runs go vet on all packages.
func Vet() error {
	fmt.Println("\n=== Running vet ===")
	return run("go", "vet", "./...")
}

// Build compiles the dev binary with version info into bin/.
func Build() error {
	fmt.Println("\n=== Building binary ===")
	binDir := filepath.Join(projectDir(), "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return fmt.Errorf("create bin dir: %w", err)
	}

	version := devVersion()
	ldflags := fmt.Sprintf("-X %s=%s", versionVar, version)
	outPath := filepath.Join(binDir, binaryName())

	if err := run("go", "build", "-ldflags", ldflags, "-o", outPath, mainPkg); err != nil {
		return err
	}
	fmt.Printf("   Version: %s\n", version)
	return nil
}

// Preflight runs all pre-commit checks: format, tidy, mod verify, vet, lint,
// build, test, race detection, WSL test, vulnerability scan, strict formatting,
// dead code detection, benchmark smoke test, and benchmark regression check.
// If preflight passes, CI will pass.
func Preflight() error {
	fmt.Println("\n=== 1/14 Formatting ===")
	if err := fmtSources(); err != nil {
		return fmt.Errorf("format: %w", err)
	}

	fmt.Println("\n=== 2/14 Tidying modules ===")
	if err := run("go", "mod", "tidy"); err != nil {
		return fmt.Errorf("mod tidy: %w", err)
	}

	fmt.Println("\n=== 3/14 Verifying module checksums ===")
	if err := run("go", "mod", "verify"); err != nil {
		return fmt.Errorf("mod verify: %w", err)
	}

	fmt.Println("\n=== 4/14 Vetting ===")
	if err := run("go", "vet", "./..."); err != nil {
		return fmt.Errorf("vet: %w", err)
	}

	fmt.Println("\n=== 5/14 Linting ===")
	if _, err := exec.LookPath("golangci-lint"); err == nil {
		if err := run("golangci-lint", "run"); err != nil {
			return fmt.Errorf("lint: %w", err)
		}
	} else {
		fmt.Println("   Skipped (install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)")
	}

	fmt.Println("\n=== 6/14 Building ===")
	if err := run("go", "build", "./..."); err != nil {
		return fmt.Errorf("build: %w", err)
	}

	fmt.Println("\n=== 7/14 Testing ===")
	if err := run("go", "test", "./...", "-count=1"); err != nil {
		return fmt.Errorf("test: %w", err)
	}
	if err := runMagefileTests(); err != nil {
		return fmt.Errorf("magefile test: %w", err)
	}

	fmt.Println("\n=== 8/14 Testing (race detector) ===")
	if err := ensureWindowsRaceToolchain(); err != nil {
		return fmt.Errorf("race toolchain: %w", err)
	}
	// Race-instrumented binaries are very large; redirect linker temp to
	// GOTMPDIR (if set) to avoid filling a small C: drive on Windows.
	if tmpDir := os.Getenv("GOTMPDIR"); tmpDir != "" && runtime.GOOS == "windows" {
		origTemp := os.Getenv("TEMP")
		origTmp := os.Getenv("TMP")
		os.Setenv("TEMP", tmpDir)
		os.Setenv("TMP", tmpDir)
		defer func() {
			os.Setenv("TEMP", origTemp)
			os.Setenv("TMP", origTmp)
		}()
	}
	if err := run("go", "test", "-race", "./...", "-count=1"); err != nil {
		return fmt.Errorf("race test: %w", err)
	}

	fmt.Println("\n=== 9/14 Testing (WSL) ===")
	if runtime.GOOS == "windows" {
		if _, err := exec.LookPath("wsl"); err == nil {
			if err := TestWSL(); err != nil {
				return fmt.Errorf("wsl test: %w", err)
			}
		} else {
			fmt.Println("   Skipped (WSL not installed)")
		}
	} else {
		fmt.Println("   Skipped (only available on Windows)")
	}

	fmt.Println("\n=== 10/14 Vulnerability scan ===")
	if _, err := exec.LookPath("govulncheck"); err == nil {
		if err := run("govulncheck", "./..."); err != nil {
			return fmt.Errorf("vulncheck: %w", err)
		}
	} else {
		fmt.Println("   Skipped (install: go install golang.org/x/vuln/cmd/govulncheck@latest)")
	}

	fmt.Println("\n=== 11/14 Strict formatting (gofumpt) ===")
	if _, err := exec.LookPath("gofumpt"); err == nil {
		out, _ := cmdOutput("gofumpt", "-l", ".")
		if files := strings.TrimSpace(out); files != "" {
			return fmt.Errorf("gofumpt: files need formatting:\n%s", files)
		}
	} else {
		fmt.Println("   Skipped (install: go install mvdan.cc/gofumpt@latest)")
	}

	fmt.Println("\n=== 12/14 Dead code detection ===")
	if _, err := exec.LookPath("deadcode"); err == nil {
		if err := runDeadcode(); err != nil {
			return err
		}
	} else {
		fmt.Println("   Skipped (install: go install golang.org/x/tools/cmd/deadcode@latest)")
	}

	fmt.Println("\n=== 13/14 Benchmark smoke test ===")
	// Quick single-iteration run to verify all benchmarks compile and execute.
	if err := run(
		"go", "test", "-bench=.", "-benchmem", "-run=^$", "-count=1", "-timeout=5m",
		"./internal/git/",
		"./internal/ai/",
		"./internal/config/",
		"./internal/crashlog/",
		"./internal/panels/gitdiff/",
		"./internal/panels/filetree/",
		"./internal/markdown/",
		"./internal/notify/",
	); err != nil {
		return fmt.Errorf("benchmark smoke: %w", err)
	}

	fmt.Println("\n=== 14/14 Benchmark regression check ===")
	platform := benchPlatform()
	baseline := filepath.Join(projectDir(), "perf", "baselines", platform, "main.txt")
	if _, err := os.Stat(baseline); err == nil {
		// Baseline exists -- run benchmarks and check for regressions.
		current := filepath.Join(projectDir(), "perf", "bench-preflight.txt")
		if err := runBenchmarks(projectDir(), current, 3); err != nil {
			return fmt.Errorf("benchmark regression run: %w", err)
		}
		regressScript := filepath.Join(projectDir(), "scripts", "bench-regress.sh")
		if _, err := os.Stat(regressScript); err == nil {
			var bashErr error
			if runtime.GOOS == "windows" {
				// On Windows, run benchstat natively and analyze output
				// in Go (avoids bash/awk/WSL dependency issues).
				benchstatPath, benchstatErr := exec.LookPath("benchstat")
				if benchstatErr != nil {
					fmt.Println("   Skipped regression analysis (benchstat not in PATH)")
				} else {
					benchOut, err := cmdOutput(benchstatPath, baseline, current)
					if err != nil {
						os.Remove(current)
						return fmt.Errorf("benchstat: %w", err)
					}
					fmt.Println(benchOut)
					// Preflight uses count=3 for speed; lower sample count
					// means wider variance. Use generous thresholds here —
					// the CI workflow (count=6) enforces tighter gates.
					bashErr = checkBenchRegressions(benchOut, 50, 25)
				}
			} else {
				scriptArg := strings.ReplaceAll(regressScript, `\`, `/`)
				baselineArg := strings.ReplaceAll(baseline, `\`, `/`)
				currentArg := strings.ReplaceAll(current, `\`, `/`)
				bashErr = run("bash", scriptArg, baselineArg, currentArg)
			}
			if bashErr != nil {
				os.Remove(current)
				return fmt.Errorf("benchmark regression detected: %w", bashErr)
			}
		} else {
			fmt.Println("   Skipped regression analysis (bench-regress.sh not found)")
		}
		// Clean up temporary file.
		os.Remove(current)
	} else {
		fmt.Printf("   Skipped (no baseline for %s -- run 'mage benchbaseline' to create one)\n", platform)
	}

	fmt.Println("\n=== Preflight passed — ready to commit ===")
	return nil
}

func ensureWindowsRaceToolchain() error {
	if runtime.GOOS != "windows" {
		return nil
	}
	if _, err := exec.LookPath("gcc"); err == nil {
		return nil
	}

	spec, err := windowsRaceToolchainSpec(runtime.GOARCH)
	if err != nil {
		return err
	}
	root, err := windowsRaceToolchainRoot(spec)
	if err != nil {
		return err
	}
	binDir := windowsRaceToolchainBinDir(root)
	compiler := filepath.Join(binDir, "gcc.exe")
	if _, err := os.Stat(compiler); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		fmt.Printf("   C compiler missing; installing %s under %s\n", spec.name, root)
		if err := installWindowsRaceToolchain(spec, root); err != nil {
			return err
		}
	}
	if _, err := os.Stat(compiler); err != nil {
		return fmt.Errorf("installed compiler not found at %s: %w", compiler, err)
	}
	prependEnvPath(binDir)
	fmt.Printf("   Race compiler: %s\n", compiler)
	return nil
}

type windowsToolchainSpec struct {
	name   string
	url    string
	sha256 string
}

func windowsRaceToolchainSpec(goarch string) (windowsToolchainSpec, error) {
	if goarch != "amd64" {
		return windowsToolchainSpec{}, fmt.Errorf("unsupported Windows race compiler architecture %q", goarch)
	}
	name := fmt.Sprintf("w64devkit-x64-%s.7z.exe", windowsRaceToolchainVersion)
	return windowsToolchainSpec{
		name:   name,
		url:    fmt.Sprintf("https://github.com/skeeto/w64devkit/releases/download/v%s/%s", windowsRaceToolchainVersion, name),
		sha256: windowsRaceToolchainSHA256,
	}, nil
}

func windowsRaceToolchainRoot(spec windowsToolchainSpec) (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locate user cache dir: %w", err)
	}
	name := strings.TrimSuffix(spec.name, ".7z.exe")
	return filepath.Join(cacheDir, "grut", "toolchains", name), nil
}

func windowsRaceToolchainBinDir(root string) string {
	return filepath.Join(root, "w64devkit", "bin")
}

func installWindowsRaceToolchain(spec windowsToolchainSpec, root string) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	archive := filepath.Join(root, spec.name)
	if err := ensureDownloadedFile(spec.url, archive, spec.sha256); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), windowsRaceToolchainExtractTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, archive, "-y", "-o"+root)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = root
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("extract %s timed out after %s: %w", spec.name, windowsRaceToolchainExtractTimeout, ctx.Err())
		}
		return fmt.Errorf("extract %s: %w", spec.name, err)
	}
	return nil
}

func ensureDownloadedFile(url, path, wantSHA256 string) (err error) {
	if fileSHA256(path) == wantSHA256 {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	tmp := path + ".tmp"
	if err := os.Remove(tmp); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			_ = out.Close()
		}
		if err != nil {
			_ = os.Remove(tmp)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), windowsRaceToolchainDownloadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create download request for %s: %w", url, err)
	}
	resp, err := http.DefaultClient.Do(req) //nolint:gosec // URL is a pinned HTTPS release asset.
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("download %s: %s", url, resp.Status)
	}

	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(out, hasher), resp.Body); err != nil {
		return err
	}
	got := hex.EncodeToString(hasher.Sum(nil))
	if got != wantSHA256 {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", filepath.Base(path), wantSHA256, got)
	}
	closeErr := out.Close()
	closed = true
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(tmp, path)
}

func fileSHA256(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return ""
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func prependEnvPath(dir string) {
	path := os.Getenv("PATH")
	if path == "" {
		os.Setenv("PATH", dir)
		return
	}
	for _, entry := range filepath.SplitList(path) {
		if strings.EqualFold(filepath.Clean(entry), filepath.Clean(dir)) {
			return
		}
	}
	os.Setenv("PATH", dir+string(os.PathListSeparator)+path)
}

// Fmt formats all Go source files.
func Fmt() error {
	fmt.Println("=== Formatting ===")
	return fmtSources()
}

// Lint runs golangci-lint if available, otherwise falls back to go vet.
func Lint() error {
	fmt.Println("\n=== Linting ===")
	if _, err := exec.LookPath("golangci-lint"); err == nil {
		return run("golangci-lint", "run")
	}
	fmt.Println("   golangci-lint not found, using go vet")
	return run("go", "vet", "./...")
}

// Bench runs all benchmarks with memory allocation stats.
func Bench() error {
	fmt.Println("\n=== Running benchmarks ===")
	return run("go", "test", "-bench=.", "-benchmem", "-run=^$", "-count=3", "./...")
}

// BenchBaseline captures a benchmark baseline for the current platform and saves
// it to perf/baselines/{goos-goarch}/main.txt.
// Use this after merging improvements to record the new performance baseline.
//
//	mage benchbaseline
func BenchBaseline() error {
	fmt.Println("\n=== Capturing benchmark baseline ===")
	dir := projectDir()
	platform := benchPlatform()
	outDir := filepath.Join(dir, "perf", "baselines", platform)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	out := filepath.Join(outDir, "main.txt")
	fmt.Printf("   Platform: %s\n", platform)
	if err := runBenchmarks(dir, out); err != nil {
		return err
	}
	fmt.Printf("\n   Baseline saved: %s\n", out)
	return nil
}

// BenchCompare runs benchmarks and compares against the current platform's
// perf/baselines/{goos-goarch}/main.txt using benchstat.
// Requires benchstat: go install golang.org/x/perf/cmd/benchstat@latest
//
//	mage benchcompare
func BenchCompare() error {
	fmt.Println("\n=== Running benchmark comparison ===")
	dir := projectDir()
	platform := benchPlatform()
	baseline := filepath.Join(dir, "perf", "baselines", platform, "main.txt")
	if _, err := os.Stat(baseline); errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("no baseline for %s at %s — run 'mage benchbaseline' first", platform, baseline)
	}
	fmt.Printf("   Platform: %s\n   Baseline: %s\n", platform, baseline)
	current := filepath.Join(dir, "perf", "bench-current.txt")
	if err := runBenchmarks(dir, current); err != nil {
		return err
	}
	fmt.Println("\n=== benchstat comparison ===")
	return run("benchstat", baseline, current)
}

// BenchWSL runs benchmarks under WSL (Linux) and compares against the
// perf/baselines/linux-amd64/main.txt baseline. Useful on Windows for
// getting Linux performance numbers without leaving the terminal.
// Requires WSL and Go installed inside WSL.
//
//	mage benchwsl
func BenchWSL() error {
	fmt.Println("\n=== Running WSL benchmark comparison ===")
	if runtime.GOOS != "windows" {
		fmt.Println("   Skipped (BenchWSL only runs on Windows with WSL)")
		return nil
	}
	if _, err := exec.LookPath("wsl"); err != nil {
		fmt.Println("   Skipped (WSL not installed)")
		return nil
	}

	dir := projectDir()
	wslPath := winToWSLPath(dir)
	// Single-quote for bash to prevent injection via directory names.
	escapedPath := "'" + strings.ReplaceAll(wslPath, "'", "'\\''") + "'"

	baselineDir := filepath.Join(dir, "perf", "baselines", "linux-amd64")
	if err := os.MkdirAll(baselineDir, 0o755); err != nil {
		return err
	}

	pkgs := "./internal/git/ ./internal/ai/ ./internal/config/ ./internal/crashlog/ ./internal/panels/gitdiff/ ./internal/panels/filetree/"
	currentTxt := escapedPath + "/perf/bench-wsl-current.txt"
	benchCmd := fmt.Sprintf(
		"cd %s && go test -bench=. -benchmem -count=6 -run='^$' -timeout=20m %s | tee %s",
		escapedPath, pkgs, currentTxt,
	)

	fmt.Printf("   Project (WSL): %s\n", wslPath)
	fmt.Println("   Running benchmarks under WSL...")
	cmd := exec.Command("wsl", "bash", "-lc", benchCmd)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("wsl benchmarks: %w", err)
	}

	baseline := filepath.Join(baselineDir, "main.txt")
	wslBaseline := escapedPath + "/perf/baselines/linux-amd64/main.txt"
	if _, err := os.Stat(baseline); errors.Is(err, fs.ErrNotExist) {
		fmt.Printf("\n   No linux-amd64 baseline yet — saving current run as baseline.\n")
		saveCmd := fmt.Sprintf("cp %s %s", currentTxt, wslBaseline)
		save := exec.Command("wsl", "bash", "-lc", saveCmd)
		save.Stdout = os.Stdout
		save.Stderr = os.Stderr
		if err := save.Run(); err != nil {
			return fmt.Errorf("save baseline: %w", err)
		}
		fmt.Printf("   Baseline saved: %s\n", baseline)
		return nil
	}

	fmt.Println("\n=== benchstat comparison (linux-amd64) ===")
	compareCmd := fmt.Sprintf(
		"cd %s && benchstat perf/baselines/linux-amd64/main.txt perf/bench-wsl-current.txt",
		escapedPath,
	)
	cmp := exec.Command("wsl", "bash", "-lc", compareCmd)
	cmp.Stdout = os.Stdout
	cmp.Stderr = os.Stderr
	if err := cmp.Run(); err != nil {
		return fmt.Errorf("benchstat: %w", err)
	}
	return nil
}

// winToWSLPath converts a Windows absolute path to its WSL /mnt/... equivalent.
// E.g. E:\code\grut -> /mnt/e/code/grut
func winToWSLPath(winPath string) string {
	p := strings.ReplaceAll(winPath, `\`, `/`)
	if len(p) >= 2 && p[1] == ':' {
		drive := strings.ToLower(string(p[0]))
		p = "/mnt/" + drive + p[2:]
	}
	return p
}

// Contributors regenerates CONTRIBUTORS.md from the full git history.
func Contributors() error {
	fmt.Println("=== Generating CONTRIBUTORS.md ===")
	out, err := cmdOutput("go", "run", "./cmd/contrib-notes", "-format=contributors")
	if err != nil {
		return fmt.Errorf("contributors: %w", err)
	}
	path := filepath.Join(projectDir(), "CONTRIBUTORS.md")
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		return fmt.Errorf("write CONTRIBUTORS.md: %w", err)
	}
	fmt.Printf("✓ CONTRIBUTORS.md updated (%d bytes)\n", len(out))
	return nil
}

// Deadcode runs dead-code analysis with allowlist filtering.
func Deadcode() error {
	fmt.Println("=== Dead code detection ===")
	if _, err := exec.LookPath("deadcode"); err != nil {
		return fmt.Errorf("deadcode not installed (run: go install golang.org/x/tools/cmd/deadcode@latest)")
	}
	return runDeadcode()
}

// Clean removes the bin/ directory.
func Clean() error {
	fmt.Println("=== Cleaning ===")
	return os.RemoveAll(filepath.Join(projectDir(), "bin"))
}

// Uninstall removes the dev binary, cleans bin/, kills stale processes,
// and removes the bin/ directory from PATH. After uninstalling, the
// release build (if installed) becomes the active `grut` binary.
func Uninstall() error {
	fmt.Println("=== Uninstalling dev build ===")

	killStale()

	binDir := filepath.Join(projectDir(), "bin")

	// Remove the bin/ directory.
	if err := os.RemoveAll(binDir); err != nil {
		return fmt.Errorf("removing bin/: %w", err)
	}
	fmt.Println("   Removed bin/")

	// Also remove any copy in GOBIN / GOPATH/bin.
	gobin := os.Getenv("GOBIN")
	if gobin == "" {
		gopath := os.Getenv("GOPATH")
		if gopath == "" {
			home, _ := os.UserHomeDir()
			gopath = filepath.Join(home, "go")
		}
		gobin = filepath.Join(gopath, "bin")
	}
	devInGobin := filepath.Join(gobin, binaryName())
	if _, err := os.Stat(devInGobin); err == nil {
		if err := os.Remove(devInGobin); err != nil {
			fmt.Printf("   Warning: could not remove %s: %v\n", devInGobin, err)
		} else {
			fmt.Printf("   Removed %s\n", devInGobin)
		}
	}

	if runtime.GOOS == "windows" {
		removeBinFromPATH(binDir)
	}

	// Show what grut resolves to now.
	if resolved, err := exec.LookPath("grut"); err == nil {
		fmt.Printf("\n✓ Dev build uninstalled. Active grut: %s\n", resolved)
	} else {
		fmt.Println("\n✓ Dev build uninstalled. No grut binary found in PATH.")
		fmt.Println("  Install a release build: go install github.com/jongio/grut@latest")
		fmt.Println("  Or download from: https://github.com/jongio/grut/releases")
	}
	return nil
}

// removeBinFromPATH removes binDir from both Machine and User PATH on Windows.
func removeBinFromPATH(binDir string) {
	for _, scope := range []string{"Machine", "User"} {
		pathVal, _ := cmdOutput("powershell", "-NoProfile", "-Command",
			fmt.Sprintf(`[Environment]::GetEnvironmentVariable('Path','%s')`, scope))
		pathVal = strings.TrimSpace(pathVal)
		if pathVal == "" || !containsPath(pathVal, binDir) {
			continue
		}
		cleaned := removePathEntry(pathVal, binDir)
		err := exec.Command("powershell", "-NoProfile", "-Command",
			fmt.Sprintf(`[Environment]::SetEnvironmentVariable('Path','%s','%s')`,
				psSingleQuoteEscape(cleaned), scope)).Run()
		if err != nil {
			fmt.Printf("   Warning: could not update %s PATH: %v\n", scope, err)
		} else {
			fmt.Printf("   Removed %s from %s PATH\n", binDir, scope)
		}
	}
}

// removePathEntry removes all occurrences of dir from a semicolon-separated
// PATH string (case-insensitive match on Windows).
func removePathEntry(pathStr, dir string) string {
	target := strings.ToLower(filepath.Clean(dir))
	entries := strings.Split(pathStr, ";")
	var kept []string
	for _, e := range entries {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if strings.EqualFold(filepath.Clean(e), target) {
			continue
		}
		kept = append(kept, e)
	}
	return strings.Join(kept, ";")
}

// --- helpers ---

// runDeadcode runs the deadcode tool with an allowlist filter for known
// false-positives (build-tag stubs, interface impls, mage targets).  Only
// genuinely dead functions cause a failure.
func runDeadcode() error {
	out, err := cmdOutput("deadcode", "./...")
	if err != nil {
		return fmt.Errorf("deadcode: %w", err)
	}

	var genuine []string
	allowed := 0
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if isAllowlisted(line) {
			allowed++
			continue
		}
		genuine = append(genuine, line)
	}

	if len(genuine) > 0 {
		fmt.Println("   Unexpected dead code found:")
		for _, g := range genuine {
			fmt.Printf("     %s\n", g)
		}
		return fmt.Errorf("deadcode: %d genuine finding(s) (update deadcodeAllowlist if false positive)", len(genuine))
	}

	fmt.Printf("   OK (%d known exclusions)\n", allowed)
	return nil
}

// isAllowlisted reports whether a deadcode output line matches a function in
// the deadcodeAllowlist.  Each deadcode line ends with "unreachable func: <name>".
func isAllowlisted(line string) bool {
	for _, name := range deadcodeAllowlist {
		if strings.HasSuffix(line, ": "+name) {
			return true
		}
	}
	return false
}

// checkBenchRegressions parses benchstat output for significant regressions.
// Mirrors the logic in scripts/bench-regress.sh but in pure Go (no bash/awk
// dependency). Returns an error if any regression exceeds the thresholds.
func checkBenchRegressions(benchstatOutput string, timingThreshold, memThreshold float64) error {
	// Match lines like: +17.89% (p=0.008 n=6)
	re := regexp.MustCompile(`\+(\d+\.\d+)%\s+\(p=`)
	var regressions []string
	for _, line := range strings.Split(benchstatOutput, "\n") {
		m := re.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		pct, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			continue
		}
		isMem := strings.Contains(line, "heap-inuse") || strings.Contains(line, "gc-cycles")
		threshold := timingThreshold
		if isMem {
			threshold = memThreshold
		}
		if pct > threshold {
			regressions = append(regressions, "  REGRESSION: "+strings.TrimSpace(line))
		}
	}
	if len(regressions) > 0 {
		for _, r := range regressions {
			fmt.Println(r)
		}
		return fmt.Errorf("%d regression(s) exceed threshold", len(regressions))
	}
	fmt.Println("   No significant regressions detected")
	return nil
}

func fmtSources() error {
	out, _ := cmdOutput("gofmt", "-l", ".")
	files := strings.TrimSpace(out)
	if files == "" {
		fmt.Println("   All files formatted")
		return nil
	}
	var unformatted []string
	for _, f := range strings.Split(files, "\n") {
		f = strings.TrimSpace(f)
		if f != "" {
			unformatted = append(unformatted, f)
		}
	}
	fmt.Printf("   Formatting %d file(s):\n", len(unformatted))
	for _, f := range unformatted {
		fmt.Printf("     %s\n", f)
	}
	return run("gofmt", "-w", ".")
}

func binaryName() string {
	if runtime.GOOS == "windows" {
		return binName + ".exe"
	}
	return binName
}

func projectDir() string {
	dir, _ := os.Getwd()
	return dir
}

// benchPlatform returns the current platform as "goos-goarch" (e.g. linux-amd64, darwin-arm64).
func benchPlatform() string {
	goos, _ := cmdOutput("go", "env", "GOOS")
	goarch, _ := cmdOutput("go", "env", "GOARCH")
	return strings.TrimSpace(goos) + "-" + strings.TrimSpace(goarch)
}

// runBenchmarks runs the benchmark suite and tees output to outFile.
func runBenchmarks(dir, outFile string, opts ...int) error {
	count := 6
	if len(opts) > 0 && opts[0] > 0 {
		count = opts[0]
	}
	pkgs := []string{
		"./internal/git/",
		"./internal/ai/",
		"./internal/config/",
		"./internal/crashlog/",
		"./internal/panels/gitdiff/",
		"./internal/panels/filetree/",
		"./internal/markdown/",
		"./internal/notify/",
	}
	args := append([]string{
		"test", "-bench=.", "-benchmem", "-run=^$",
		fmt.Sprintf("-count=%d", count), "-timeout=15m",
	}, pkgs...)
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	f, err := os.Create(outFile)
	if err != nil {
		return err
	}
	defer f.Close()
	cmd.Stdout = io.MultiWriter(os.Stdout, f)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func devVersion() string {
	hash, _ := cmdOutput("git", "rev-parse", "--short", "HEAD")
	ts := time.Now().Format("20060102-150405")
	return fmt.Sprintf("dev-%s-%s", strings.TrimSpace(hash), ts)
}

func killStale() {
	fmt.Println("\n=== Killing stale processes ===")
	if runtime.GOOS == "windows" {
		// Use -Name with a single-quoted literal to prevent injection via binName.
		script := fmt.Sprintf(`Get-Process -Name '%s' -ErrorAction SilentlyContinue | Stop-Process -Force`,
			psSingleQuoteEscape(binName))
		exec.Command("powershell", "-NoProfile", "-Command", script).Run()
	} else {
		exec.Command("pkill", "-f", binaryName()).Run()
	}
	time.Sleep(500 * time.Millisecond)
}

func ensurePath() error {
	binDir := filepath.Join(projectDir(), "bin")

	if runtime.GOOS != "windows" {
		path := os.Getenv("PATH")
		if !strings.Contains(path, binDir) {
			fmt.Printf("NOTE: Add %s to your PATH:\n  export PATH=\"%s:$PATH\"\n", binDir, binDir)
		}
		return nil
	}

	// Windows: clean stale worktree bin entries from User PATH before
	// checking or modifying anything. Each 'mage install' from a different
	// worktree appends its own bin/ dir; over time these accumulate and
	// cause the wrong binary to resolve via Get-Command.
	if repoName := detectRepoName(); repoName != "" {
		userPath, _ := cmdOutput("powershell", "-NoProfile", "-Command",
			`[Environment]::GetEnvironmentVariable('Path','User')`)
		userPath = strings.TrimSpace(userPath)
		if userPath != "" {
			cleaned, removed := removeStaleWorktreeBins(userPath, repoName, binDir)
			if len(removed) > 0 {
				fmt.Println("   Cleaning stale worktree PATH entries:")
				for _, r := range removed {
					fmt.Printf("   - %s\n", r)
				}
				if err := exec.Command("powershell", "-NoProfile", "-Command",
					fmt.Sprintf(`[Environment]::SetEnvironmentVariable('Path','%s','User')`,
						psSingleQuoteEscape(cleaned))).Run(); err != nil {
					fmt.Printf("   ⚠ Failed to remove stale PATH entries: %v\n", err)
				}
			}
		}
	}

	// Windows: ensure bin/ is in persistent PATH
	machinePath, _ := cmdOutput("powershell", "-NoProfile", "-Command",
		`[Environment]::GetEnvironmentVariable('Path','Machine')`)
	machinePath = strings.TrimSpace(machinePath)

	if containsPath(machinePath, binDir) {
		ensureSessionPath(binDir)
		return nil
	}

	// Also check User PATH before trying to modify anything.
	userPath, _ := cmdOutput("powershell", "-NoProfile", "-Command",
		`[Environment]::GetEnvironmentVariable('Path','User')`)
	userPath = strings.TrimSpace(userPath)
	if containsPath(userPath, binDir) {
		ensureSessionPath(binDir)
		return nil
	}

	fmt.Printf("\n=== Adding %s to system PATH ===\n", binDir)
	newPath := binDir + ";" + machinePath
	err := exec.Command("powershell", "-NoProfile", "-Command",
		fmt.Sprintf(`[Environment]::SetEnvironmentVariable('Path','%s','Machine')`, psSingleQuoteEscape(newPath))).Run()
	if err != nil {
		fmt.Println("   Machine PATH failed (need admin), trying User PATH...")
		userPath, _ := cmdOutput("powershell", "-NoProfile", "-Command",
			`[Environment]::GetEnvironmentVariable('Path','User')`)
		userPath = strings.TrimSpace(userPath)
		if !containsPath(userPath, binDir) {
			err = exec.Command("powershell", "-NoProfile", "-Command",
				fmt.Sprintf(`[Environment]::SetEnvironmentVariable('Path','%s','User')`,
					psSingleQuoteEscape(binDir+";"+userPath))).Run()
		} else {
			err = nil // already present in User PATH
		}
	}
	if err == nil {
		broadcastPathChange()
	}
	ensureSessionPath(binDir)
	if err != nil {
		fmt.Println("   ⚠ Could not update persistent PATH (tried Machine and User).")
		fmt.Println("   To use in this terminal only, run:")
		fmt.Printf("   $env:Path = \"%s;\" + $env:Path\n", binDir)
	} else {
		fmt.Println("   To use in this terminal, run:")
		fmt.Printf("   $env:Path = \"%s;\" + $env:Path\n", binDir)
	}
	return nil
}

func containsPath(pathList, dir string) bool {
	return strings.Contains(strings.ToLower(pathList), strings.ToLower(dir))
}

// psSingleQuoteEscape escapes a string for safe embedding inside a
// PowerShell single-quoted literal. The only character that needs escaping
// is the single quote itself, which is doubled (”).
func psSingleQuoteEscape(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// detectRepoName returns the repository name by inspecting the current
// working directory. It handles both main-repo and worktree layouts:
//   - Worktree: .../.worktrees/<repoName>/<branch>/... → repoName
//   - Main repo: .../<repoName> → basename of cwd
func detectRepoName() string {
	dir := projectDir()
	sep := string(filepath.Separator)
	marker := sep + ".worktrees" + sep
	lower := strings.ToLower(dir)
	if idx := strings.Index(lower, strings.ToLower(marker)); idx >= 0 {
		rest := dir[idx+len(marker):]
		if sepIdx := strings.IndexByte(rest, filepath.Separator); sepIdx > 0 {
			return strings.ToLower(rest[:sepIdx])
		}
		if rest != "" {
			return strings.ToLower(rest)
		}
	}
	return strings.ToLower(filepath.Base(dir))
}

// removeStaleWorktreeBins filters a semicolon-separated PATH string,
// removing entries that belong to other worktrees of the same repo.
// An entry is considered a stale worktree bin if it contains
// /.worktrees/<repoName>/ and ends with /bin, but is not currentBinDir.
// Returns the filtered PATH and the list of removed entries.
func removeStaleWorktreeBins(pathStr, repoName, currentBinDir string) (string, []string) {
	sep := string(filepath.Separator)
	worktreeMarker := strings.ToLower(sep + ".worktrees" + sep + repoName + sep)
	binSuffix := strings.ToLower(sep + "bin")
	currentClean := strings.ToLower(filepath.Clean(currentBinDir))

	entries := strings.Split(pathStr, ";")
	var kept []string
	var removed []string
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		entryClean := strings.ToLower(filepath.Clean(entry))
		if strings.Contains(entryClean, worktreeMarker) &&
			strings.HasSuffix(entryClean, binSuffix) &&
			entryClean != currentClean {
			removed = append(removed, entry)
			continue
		}
		kept = append(kept, entry)
	}
	return strings.Join(kept, ";"), removed
}

func ensureSessionPath(binDir string) {
	current := os.Getenv("Path")
	// Remove stale worktree bin entries from the session PATH so the
	// freshly-built binary is what resolves for the rest of this process.
	if repoName := detectRepoName(); repoName != "" {
		cleaned, _ := removeStaleWorktreeBins(current, repoName, binDir)
		current = cleaned
	}
	if !containsPath(current, binDir) {
		current = binDir + ";" + current
	}
	os.Setenv("Path", current)
}

// broadcastPathChange sends WM_SETTINGCHANGE so new Explorer/shell windows
// pick up the modified persistent PATH immediately without requiring a reboot.
func broadcastPathChange() {
	if runtime.GOOS != "windows" {
		return
	}
	// SendMessageTimeout with HWND_BROADCAST notifies all top-level windows
	// that the environment has changed. New terminals will read the updated
	// persistent PATH; existing terminals remain unaffected (OS limitation).
	script := `Add-Type -Namespace Win32 -Name NativeMethods -MemberDefinition @"
[DllImport("user32.dll", SetLastError = true, CharSet = CharSet.Auto)]
public static extern IntPtr SendMessageTimeout(
    IntPtr hWnd, uint Msg, UIntPtr wParam, string lParam,
    uint fuFlags, uint uTimeout, out UIntPtr lpdwResult);
"@
$HWND_BROADCAST = [IntPtr]0xFFFF
$WM_SETTINGCHANGE = 0x001A
$result = [UIntPtr]::Zero
[Win32.NativeMethods]::SendMessageTimeout($HWND_BROADCAST, $WM_SETTINGCHANGE, [UIntPtr]::Zero, "Environment", 0x0002, 5000, [ref]$result) | Out-Null`
	if err := exec.Command("powershell", "-NoProfile", "-Command", script).Run(); err != nil {
		fmt.Printf("   Warning: PATH broadcast failed: %v\n", err)
	}
}

func verify() error {
	outPath := filepath.Join(projectDir(), "bin", binaryName())
	info, err := os.Stat(outPath)
	if err != nil {
		return fmt.Errorf("binary not found after build: %s", outPath)
	}

	age := time.Since(info.ModTime())
	if age > 30*time.Second {
		return fmt.Errorf("%s seems stale (built %s, %.0fs ago)", binaryName(), info.ModTime().Format(time.DateTime), age.Seconds())
	}

	resolved, err := exec.LookPath(binaryName())
	if err != nil {
		return fmt.Errorf("%s not found in PATH", binaryName())
	}
	resolvedAbs, _ := filepath.Abs(resolved)
	expectedAbs, _ := filepath.Abs(outPath)
	if !strings.EqualFold(resolvedAbs, expectedAbs) {
		return fmt.Errorf("PATH resolves to %s, expected %s — another binary may be shadowing", resolvedAbs, expectedAbs)
	}

	fmt.Printf("\n✓ %s installed\n", binaryName())
	fmt.Printf("   Path:  %s\n", outPath)
	fmt.Printf("   Built: %s\n", info.ModTime().Format(time.DateTime))
	return nil
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = projectDir()
	return cmd.Run()
}

func cmdOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = projectDir()
	out, err := cmd.Output()
	return string(out), err
}

// cmdOutputTimeout runs a command with a deadline, returning an error on timeout.
func cmdOutputTimeout(d time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = projectDir()
	out, err := cmd.Output()
	return string(out), err
}
