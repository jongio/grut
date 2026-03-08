//go:build mage

// This magefile handles building, testing, and installing grut.
//
// On Windows, Go's temp binaries in %TEMP% can be blocked by Defender.
// The init() function below automatically sets GOTMPDIR to a project-local
// directory (bin/.tmp) so `mage install` works without manual env setup.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var (
	binName    = "grut-dev"
	mainPkg    = "."
	versionVar = "github.com/jongio/grut/internal/config.AppVersion"
)

func init() {
	// On Windows, Defender may block Go temp binaries compiled into %TEMP%.
	// Redirect GOTMPDIR to a project-local directory to avoid this.
	if os.Getenv("GOTMPDIR") != "" {
		return
	}
	tmpDir := filepath.Join(projectDir(), "bin", ".tmp")
	os.MkdirAll(tmpDir, 0o755)
	os.Setenv("GOTMPDIR", tmpDir)
}

// deadcodeAllowlist contains functions reported by deadcode that are not
// genuinely dead: interface implementations dispatched via panels.Panel or
// extension.Runtime, factory-constructed runtimes, build-tag stubs, WASM
// host-function callbacks, and test-only accessors providing public API surface.
var deadcodeAllowlist = []string{
	// config — test-only accessor (used in app_test.go, engine_test.go)
	"LoadDefaults",

	// crashlog — test-only accessors
	"CollectDiagnostics",
	"RecoverAndReport",

	// filetree — test accessor methods (not wired to key bindings yet)
	"FileTree.bookmarkCurrent",
	"FileTree.addToContext",

	// actions registry — public API getter
	"Description",

	// extension permissions — public API
	"CheckPermission",
	"AllPermissions",

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

	// ai — token estimation + sanitization (public API / test accessors)
	"tokenBudget.remaining",
	"estimateTokensForStatus",
	"SanitizeCommitMessage",
	"stripControlCharsPreserveNewlines",

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

	// theme — public API
	"LoadFromFile",
	"BuiltinNames",

	// mcp/server — test-only helper for validating path arrays
	"validateGitPaths",

	// notify/modal — public API for action picker with message subtitle
	"ShowActionPickerWithMessage",

	// fuzzyfinder/source — test-support cache invalidation API
	"fileCache.invalidate",
	"InvalidateFileCache",
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
	return run("go", "test", "./...", "-count=1")
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
	wslPath, err := cmdOutput("wsl", "wslpath", "-u", projectDir())
	if err != nil {
		return fmt.Errorf("wslpath: %w", err)
	}
	wslPath = strings.TrimSpace(wslPath)

	fmt.Printf("   Project (WSL): %s\n", wslPath)

	// Single-quote the path for bash to prevent injection via directory names
	// containing $(), backticks, or other shell metacharacters.
	escapedPath := "'" + strings.ReplaceAll(wslPath, "'", "'\\''") + "'"

	cmd := exec.Command("wsl", "bash", "-lc",
		fmt.Sprintf("cd %s && go test ./... -count=1", escapedPath))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = projectDir()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("wsl test: %w", err)
	}

	fmt.Println("   WSL tests passed")
	return nil
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
	os.MkdirAll(binDir, 0o755)

	version := devVersion()
	ldflags := fmt.Sprintf("-X %s=%s", versionVar, version)
	outPath := filepath.Join(binDir, binaryName())

	if err := run("go", "build", "-ldflags", ldflags, "-o", outPath, mainPkg); err != nil {
		return err
	}
	fmt.Printf("   Version: %s\n", version)
	return nil
}

// Preflight runs all pre-commit checks: format, tidy, vet, lint, build, test,
// race detection, vulnerability scan, strict formatting, and dead code detection.
// If preflight passes, CI will pass.
func Preflight() error {
	fmt.Println("\n=== 1/12 Formatting ===")
	if err := fmtSources(); err != nil {
		return fmt.Errorf("format: %w", err)
	}

	fmt.Println("\n=== 2/12 Tidying modules ===")
	if err := run("go", "mod", "tidy"); err != nil {
		return fmt.Errorf("mod tidy: %w", err)
	}

	fmt.Println("\n=== 3/12 Verifying module checksums ===")
	if err := run("go", "mod", "verify"); err != nil {
		return fmt.Errorf("mod verify: %w", err)
	}

	fmt.Println("\n=== 4/12 Vetting ===")
	if err := run("go", "vet", "./..."); err != nil {
		return fmt.Errorf("vet: %w", err)
	}

	fmt.Println("\n=== 5/12 Linting ===")
	if _, err := exec.LookPath("golangci-lint"); err == nil {
		if err := run("golangci-lint", "run"); err != nil {
			return fmt.Errorf("lint: %w", err)
		}
	} else {
		fmt.Println("   Skipped (install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)")
	}

	fmt.Println("\n=== 6/12 Building ===")
	if err := run("go", "build", "./..."); err != nil {
		return fmt.Errorf("build: %w", err)
	}

	fmt.Println("\n=== 7/12 Testing ===")
	if err := run("go", "test", "./...", "-count=1"); err != nil {
		return fmt.Errorf("test: %w", err)
	}

	fmt.Println("\n=== 8/12 Testing (race detector) ===")
	if err := run("go", "test", "-race", "./...", "-count=1"); err != nil {
		return fmt.Errorf("race test: %w", err)
	}

	fmt.Println("\n=== 9/12 Testing (WSL) ===")
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

	fmt.Println("\n=== 10/12 Vulnerability scan ===")
	if _, err := exec.LookPath("govulncheck"); err == nil {
		if err := run("govulncheck", "./..."); err != nil {
			return fmt.Errorf("vulncheck: %w", err)
		}
	} else {
		fmt.Println("   Skipped (install: go install golang.org/x/vuln/cmd/govulncheck@latest)")
	}

	fmt.Println("\n=== 11/12 Strict formatting (gofumpt) ===")
	if _, err := exec.LookPath("gofumpt"); err == nil {
		out, _ := cmdOutput("gofumpt", "-l", ".")
		if files := strings.TrimSpace(out); files != "" {
			return fmt.Errorf("gofumpt: files need formatting:\n%s", files)
		}
	} else {
		fmt.Println("   Skipped (install: go install mvdan.cc/gofumpt@latest)")
	}

	fmt.Println("\n=== 12/12 Dead code detection ===")
	if _, err := exec.LookPath("deadcode"); err == nil {
		if err := runDeadcode(); err != nil {
			return err
		}
	} else {
		fmt.Println("   Skipped (install: go install golang.org/x/tools/cmd/deadcode@latest)")
	}

	fmt.Println("\n=== Preflight passed — ready to commit ===")
	return nil
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

// Clean removes the bin/ directory.
func Clean() error {
	fmt.Println("=== Cleaning ===")
	return os.RemoveAll(filepath.Join(projectDir(), "bin"))
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

	// Windows: ensure bin/ is in persistent PATH
	machinePath, _ := cmdOutput("powershell", "-NoProfile", "-Command",
		`[Environment]::GetEnvironmentVariable('Path','Machine')`)
	machinePath = strings.TrimSpace(machinePath)

	if containsPath(machinePath, binDir) {
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
			exec.Command("powershell", "-NoProfile", "-Command",
				fmt.Sprintf(`[Environment]::SetEnvironmentVariable('Path','%s','User')`,
					psSingleQuoteEscape(binDir+";"+userPath))).Run()
		}
	}
	ensureSessionPath(binDir)
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

func ensureSessionPath(binDir string) {
	current := os.Getenv("Path")
	if !containsPath(current, binDir) {
		os.Setenv("Path", binDir+";"+current)
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
