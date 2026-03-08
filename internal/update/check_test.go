package update

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// CompareVersions
// ---------------------------------------------------------------------------

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.1", "1.0.0", 1},
		{"1.0.0", "1.0.1", -1},
		{"2.0.0", "1.9.9", 1},
		{"0.4.1", "0.3.0", 1},
		{"0.3.0", "0.4.1", -1},
		{"1.0", "1.0.0", 0},
		{"1", "1.0.0", 0},
		{"v1.2.3", "1.2.3", 0},
		{"v1.2.3", "v1.2.3", 0},
		{"1.2.3", "v1.2.3", 0},
		{"0.10.0", "0.9.0", 1},
		{"0.10.0", "0.2.0", 1},
		{"1.0.0", "0.99.99", 1},
		{"0.0.0", "0.0.0", 0},
		{"0.0.1", "0.0.0", 1},
	}
	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			got := CompareVersions(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("CompareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestCompareVersions_Symmetry(t *testing.T) {
	pairs := [][2]string{
		{"1.0.0", "2.0.0"},
		{"0.1.0", "0.2.0"},
		{"0.0.1", "0.0.2"},
	}
	for _, p := range pairs {
		if CompareVersions(p[0], p[1]) != -CompareVersions(p[1], p[0]) {
			t.Errorf("CompareVersions not symmetric for %q vs %q", p[0], p[1])
		}
	}
}

// ---------------------------------------------------------------------------
// isDevVersion
// ---------------------------------------------------------------------------

func TestIsDevVersion(t *testing.T) {
	tests := []struct {
		v    string
		want bool
	}{
		{"dev", true},
		{"dev-abc123-20240101", true},
		{"dev-", true},
		{"0.1.0", false},
		{"1.0.0", false},
		{"v1.0.0", false},
		{"develop", false},
		{"", false},
	}
	for _, tt := range tests {
		name := tt.v
		if name == "" {
			name = "(empty)"
		}
		t.Run(name, func(t *testing.T) {
			got := isDevVersion(tt.v)
			if got != tt.want {
				t.Errorf("isDevVersion(%q) = %v, want %v", tt.v, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// splitVersion
// ---------------------------------------------------------------------------

func TestSplitVersion(t *testing.T) {
	tests := []struct {
		v    string
		want []int
	}{
		{"1.2.3", []int{1, 2, 3}},
		{"v1.2.3", []int{1, 2, 3}},
		{"0.4.1", []int{0, 4, 1}},
		{"1.0", []int{1, 0}},
		{"1", []int{1}},
		{"abc", []int{0}},
		{"1.abc.3", []int{1, 0, 3}},
	}
	for _, tt := range tests {
		t.Run(tt.v, func(t *testing.T) {
			got := splitVersion(tt.v)
			if len(got) != len(tt.want) {
				t.Fatalf("splitVersion(%q) = %v (len %d), want %v (len %d)",
					tt.v, got, len(got), tt.want, len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitVersion(%q)[%d] = %d, want %d",
						tt.v, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Cache read/write
// ---------------------------------------------------------------------------

func TestCacheReadWriteRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test-cache.json")

	now := time.Now().Truncate(time.Second)
	original := &updateCache{
		CheckedAt:      now,
		LatestVersion:  "1.2.3",
		CurrentVersion: "1.0.0",
	}

	writeCache(path, original)

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading cache file: %v", err)
	}

	var got updateCache
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("parsing cache file: %v", err)
	}

	if got.LatestVersion != original.LatestVersion {
		t.Errorf("LatestVersion = %q, want %q", got.LatestVersion, original.LatestVersion)
	}
	if got.CurrentVersion != original.CurrentVersion {
		t.Errorf("CurrentVersion = %q, want %q", got.CurrentVersion, original.CurrentVersion)
	}
}

func TestCacheCreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	nested := filepath.Join(tmpDir, "a", "b", "c")
	path := filepath.Join(nested, "cache.json")

	writeCache(path, &updateCache{
		CheckedAt:      time.Now(),
		LatestVersion:  "1.0.0",
		CurrentVersion: "0.9.0",
	})

	if _, err := os.Stat(path); err != nil {
		t.Errorf("cache file should exist at %s: %v", path, err)
	}
}

func TestCacheFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission checks not applicable on Windows")
	}

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "perm-cache.json")

	writeCache(path, &updateCache{
		CheckedAt:      time.Now(),
		LatestVersion:  "1.0.0",
		CurrentVersion: "0.9.0",
	})

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat cache file: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != cacheFilePerm {
		t.Errorf("cache file perm = %o, want %o", perm, cacheFilePerm)
	}
}

func TestReadCacheMissing(t *testing.T) {
	tmpDir := t.TempDir()
	setConfigDir(t, tmpDir)

	cache, path := readCache()
	if cache != nil {
		t.Error("readCache on missing file should return nil cache")
	}
	if path == "" {
		t.Error("readCache should return cache path even when file is missing")
	}
}

func TestReadCacheCorrupt(t *testing.T) {
	tmpDir := t.TempDir()
	setConfigDir(t, tmpDir)

	// Determine the actual config directory (varies by OS).
	dir, err := configDirForTest(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, cacheFileName), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	cache, _ := readCache()
	if cache != nil {
		t.Error("readCache on corrupt file should return nil cache")
	}
}

// ---------------------------------------------------------------------------
// CheckForUpdate (offline / cache-only tests)
// ---------------------------------------------------------------------------

func TestCheckForUpdate_DevVersion(t *testing.T) {
	info := CheckForUpdate("dev")
	if info != nil {
		t.Error("CheckForUpdate(\"dev\") should return nil")
	}

	info = CheckForUpdate("dev-abc123-20240101")
	if info != nil {
		t.Error("CheckForUpdate(\"dev-...\") should return nil")
	}
}

func TestCheckForUpdate_CachedUpToDate(t *testing.T) {
	tmpDir := t.TempDir()
	setConfigDir(t, tmpDir)

	dir, err := configDirForTest(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	cache := &updateCache{
		CheckedAt:      time.Now(),
		LatestVersion:  "1.0.0",
		CurrentVersion: "1.0.0",
	}
	raw, _ := json.Marshal(cache)
	if err := os.WriteFile(filepath.Join(dir, cacheFileName), raw, 0o600); err != nil {
		t.Fatal(err)
	}

	info := CheckForUpdate("1.0.0")
	if info != nil {
		t.Error("CheckForUpdate should return nil when cache says up to date")
	}
}

func TestCheckForUpdate_CachedUpdateAvailable(t *testing.T) {
	tmpDir := t.TempDir()
	setConfigDir(t, tmpDir)

	dir, err := configDirForTest(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	cache := &updateCache{
		CheckedAt:      time.Now(),
		LatestVersion:  "2.0.0",
		CurrentVersion: "1.0.0",
	}
	raw, _ := json.Marshal(cache)
	if err := os.WriteFile(filepath.Join(dir, cacheFileName), raw, 0o600); err != nil {
		t.Fatal(err)
	}

	info := CheckForUpdate("1.0.0")
	if info == nil {
		t.Fatal("CheckForUpdate should return UpdateInfo when update available")
	}
	if info.CurrentVersion != "1.0.0" {
		t.Errorf("CurrentVersion = %q, want %q", info.CurrentVersion, "1.0.0")
	}
	if info.LatestVersion != "2.0.0" {
		t.Errorf("LatestVersion = %q, want %q", info.LatestVersion, "2.0.0")
	}
	if info.ReleaseURL == "" {
		t.Error("ReleaseURL should not be empty")
	}
}

func TestCheckForUpdate_StaleCache(t *testing.T) {
	tmpDir := t.TempDir()
	setConfigDir(t, tmpDir)

	dir, err := configDirForTest(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	// Write a cache that is older than cacheTTL.
	cache := &updateCache{
		CheckedAt:      time.Now().Add(-25 * time.Hour),
		LatestVersion:  "1.0.0",
		CurrentVersion: "1.0.0",
	}
	raw, _ := json.Marshal(cache)
	if err := os.WriteFile(filepath.Join(dir, cacheFileName), raw, 0o600); err != nil {
		t.Fatal(err)
	}

	// With stale cache and no network, CheckForUpdate returns nil
	// (network error is silently swallowed).
	info := CheckForUpdate("1.0.0")
	if info != nil {
		t.Error("CheckForUpdate with stale cache and no network should return nil")
	}
}

func TestCheckForUpdate_VersionChanged(t *testing.T) {
	tmpDir := t.TempDir()
	setConfigDir(t, tmpDir)

	dir, err := configDirForTest(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	// Cache was written for version 0.9.0, but we're now running 1.0.0.
	cache := &updateCache{
		CheckedAt:      time.Now(),
		LatestVersion:  "1.0.0",
		CurrentVersion: "0.9.0",
	}
	raw, _ := json.Marshal(cache)
	if err := os.WriteFile(filepath.Join(dir, cacheFileName), raw, 0o600); err != nil {
		t.Fatal(err)
	}

	// Cache version doesn't match current → triggers network call → fails → nil.
	info := CheckForUpdate("1.0.0")
	if info != nil {
		t.Error("CheckForUpdate with version mismatch and no network should return nil")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// setConfigDir overrides the OS config directory env var for the duration
// of the test, following the same pattern as config_test.go.
func setConfigDir(t *testing.T, dir string) {
	t.Helper()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("APPDATA", dir)
	case "darwin":
		t.Setenv("HOME", dir)
		appSupportDir := filepath.Join(dir, "Library", "Application Support")
		if err := os.MkdirAll(appSupportDir, 0o755); err != nil {
			t.Fatalf("creating macOS config dir: %v", err)
		}
	default:
		t.Setenv("XDG_CONFIG_HOME", dir)
	}
}

// configDirForTest returns the expected grut config directory for the
// given base temp directory, accounting for OS differences.
func configDirForTest(tmpDir string) (string, error) {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(tmpDir, "grut"), nil
	case "darwin":
		return filepath.Join(tmpDir, "Library", "Application Support", "grut"), nil
	default:
		return filepath.Join(tmpDir, "grut"), nil
	}
}
