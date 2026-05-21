//go:build mage

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLinuxGoArchFromUname(t *testing.T) {
	tests := []struct {
		name  string
		uname string
		want  string
	}{
		{name: "x86_64 newline", uname: "x86_64\n", want: "amd64"},
		{name: "amd64", uname: "amd64", want: "amd64"},
		{name: "aarch64", uname: "aarch64", want: "arm64"},
		{name: "armv7l", uname: "armv7l", want: "armv6l"},
		{name: "i686", uname: "i686", want: "386"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := linuxGoArchFromUname(tt.uname)
			if err != nil {
				t.Fatalf("linuxGoArchFromUname(%q): %v", tt.uname, err)
			}
			if got != tt.want {
				t.Fatalf("linuxGoArchFromUname(%q) = %q, want %q", tt.uname, got, tt.want)
			}
		})
	}
}

func TestWSLGoInstallScriptBuildsVerifiedUserLocalInstall(t *testing.T) {
	script, err := wslGoInstallScript("go1.26.3", "amd64")
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		`$HOME/.local/share/grut`,
		`https://go.dev/dl/go1.26.3.linux-amd64.tar.gz`,
		`https://go.dev/dl/?mode=json&include=all`,
		`sha256sum "$archive"`,
		`tar -C "$install_root" -xzf "$archive"`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("install script missing %q:\n%s", want, script)
		}
	}
	if strings.Contains(script, "sudo ") || strings.Contains(script, "apt-get") {
		t.Fatalf("install script should not require privileged package installs:\n%s", script)
	}
}

func TestWSLGoInstallScriptRejectsUnsafeInputs(t *testing.T) {
	tests := []struct {
		name    string
		version string
		arch    string
	}{
		{name: "unsafe version", version: "go1.26.3; rm -rf /", arch: "amd64"},
		{name: "unsafe architecture", version: "go1.26.3", arch: "amd64; rm -rf /"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := wslGoInstallScript(tt.version, tt.arch); err == nil {
				t.Fatal("expected unsafe input to be rejected")
			}
		})
	}
}

func TestValidGoVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    bool
	}{
		{name: "patch", version: "go1.26.3", want: true},
		{name: "minor", version: "go1.26", want: true},
		{name: "release candidate", version: "go1.27rc1", want: true},
		{name: "beta", version: "go1.27beta1", want: true},
		{name: "shell metacharacters", version: "go1.26.3; rm -rf /", want: false},
		{name: "missing prefix", version: "1.26.3", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validGoVersion(tt.version); got != tt.want {
				t.Fatalf("validGoVersion(%q) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}

func TestWSLGoRunnerInstallsWhenGoMissing(t *testing.T) {
	var runScripts []string
	runner := wslGoRunner{
		run: func(_ time.Duration, script string) error {
			runScripts = append(runScripts, script)
			if len(runScripts) == 1 {
				return errors.New("go missing")
			}
			return nil
		},
		output: func(_ time.Duration, script string) (string, error) {
			if script == "uname -m" {
				return "x86_64\n", nil
			}
			t.Fatalf("unexpected output script: %s", script)
			return "", nil
		},
		localVersion: func() (string, error) {
			return "go1.26.3", nil
		},
	}

	if err := runner.ensureGoToolchain(); err != nil {
		t.Fatal(err)
	}
	if len(runScripts) != 3 {
		t.Fatalf("run scripts = %d, want 3: %#v", len(runScripts), runScripts)
	}
	if !strings.Contains(runScripts[0], "command -v go") {
		t.Fatalf("first script should check for go: %s", runScripts[0])
	}
	if !strings.Contains(runScripts[1], "go1.26.3.linux-amd64.tar.gz") {
		t.Fatalf("second script should install Go: %s", runScripts[1])
	}
	if !strings.Contains(runScripts[2], "command -v go") {
		t.Fatalf("third script should verify installed Go: %s", runScripts[2])
	}
}

func TestWSLGoRunnerDoesNotInstallWhenGoExists(t *testing.T) {
	var localVersionCalled bool
	runner := wslGoRunner{
		run: func(_ time.Duration, script string) error {
			if !strings.Contains(script, "command -v go") {
				t.Fatalf("unexpected run script: %s", script)
			}
			return nil
		},
		output: func(_ time.Duration, script string) (string, error) {
			if !strings.Contains(script, "go version") {
				t.Fatalf("unexpected output script: %s", script)
			}
			return "go version go1.26.3 linux/amd64\n", nil
		},
		localVersion: func() (string, error) {
			localVersionCalled = true
			return "", nil
		},
	}

	if err := runner.ensureGoToolchain(); err != nil {
		t.Fatal(err)
	}
	if localVersionCalled {
		t.Fatal("local Go version should not be needed when WSL already has Go")
	}
}

func TestWindowsRaceToolchainSpecPinsPortableCompiler(t *testing.T) {
	spec, err := windowsRaceToolchainSpec("amd64")
	if err != nil {
		t.Fatal(err)
	}
	if spec.name != "w64devkit-x64-2.8.0.7z.exe" {
		t.Fatalf("unexpected toolchain name: %s", spec.name)
	}
	if spec.url != "https://github.com/skeeto/w64devkit/releases/download/v2.8.0/w64devkit-x64-2.8.0.7z.exe" {
		t.Fatalf("unexpected toolchain URL: %s", spec.url)
	}
	if spec.sha256 != "6252bf34fe2231a55ac7f03d482b36d2c7c58697990551bba508102cfb3f342e" {
		t.Fatalf("unexpected toolchain SHA256: %s", spec.sha256)
	}
}

func TestWindowsRaceToolchainSpecRejectsUnsupportedArch(t *testing.T) {
	if _, err := windowsRaceToolchainSpec("arm64"); err == nil {
		t.Fatal("expected unsupported architecture to be rejected")
	}
}

func TestPrependEnvPathAddsDirectoryOnce(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "toolchain", "bin")
	originalPath := filepath.Join(t.TempDir(), "other")
	t.Setenv("PATH", originalPath)

	prependEnvPath(dir)
	got := filepath.SplitList(os.Getenv("PATH"))
	if len(got) != 2 || got[0] != dir || got[1] != originalPath {
		t.Fatalf("PATH after prepend = %#v, want [%q %q]", got, dir, originalPath)
	}

	prependEnvPath(dir)
	got = filepath.SplitList(os.Getenv("PATH"))
	if len(got) != 2 {
		t.Fatalf("PATH should not duplicate prepended dir: %#v", got)
	}
}

func TestFileSHA256ReturnsEmptyForMissingFile(t *testing.T) {
	if got := fileSHA256(filepath.Join(t.TempDir(), "missing")); got != "" {
		t.Fatalf("fileSHA256 missing file = %q, want empty string", got)
	}
}
