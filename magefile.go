//go:build mage

// This magefile handles building, testing, and installing grut.
//
// On Windows, Go's temp binaries in %TEMP% can be blocked by Defender.
// The init() function below automatically sets GOTMPDIR to a project-local
// directory (bin/.tmp) so `mage install` works without manual env setup.

package main

import (
	"fmt"
	"io"
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
	// mcp_procattr_windows.go — build-tag no-op stubs; called from Load/Close
	// but unreachable under deadcode's Windows call-graph analysis because
	// MCPRuntime.Load itself is factory-dispatched (also allowlisted above).
	"setProcGroup",
	"killProcGroup",

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
	"ResetFirstRunIn",

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
	// filepath.ToSlash converts backslashes to forward slashes, which is
	// required for wslpath on WSL 1 — passing single-backslash Windows paths
	// causes wslpath to strip the backslashes and return exit status 1.
	wslPath, err := cmdOutput("wsl", "wslpath", "-u", filepath.ToSlash(projectDir()))
	if err != nil {
		return fmt.Errorf("wslpath: %w", err)
	}
	wslPath = strings.TrimSpace(wslPath)

	fmt.Printf("   Project (WSL): %s\n", wslPath)

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
			wslGitDir, convErr := cmdOutput("wsl", "wslpath", "-u", filepath.ToSlash(winGitDir))
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

	cmd := exec.Command("wsl", "bash", "-lc",
		fmt.Sprintf("cd %s && go test ./... -count=1", escapedPath))
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

	fmt.Println("\n=== 8/14 Testing (race detector) ===")
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
	if err := run("go", "test", "-bench=.", "-benchmem", "-run=^$", "-count=1", "-timeout=5m",
		"./internal/git/",
		"./internal/ai/",
		"./internal/config/",
		"./internal/crashlog/",
		"./internal/panels/gitdiff/",
		"./internal/panels/filetree/",
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
	if _, err := os.Stat(baseline); os.IsNotExist(err) {
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
	if _, err := os.Stat(baseline); os.IsNotExist(err) {
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
