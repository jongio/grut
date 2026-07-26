package update

import (
	"archive/zip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- isStaleLock coverage ---

func TestIsStaleLock_NonexistentFile(t *testing.T) {
	stale, err := isStaleLock(filepath.Join(t.TempDir(), "no-such-lock"))
	require.NoError(t, err)
	assert.False(t, stale, "nonexistent file should not be considered stale")
}

func TestIsStaleLock_RecentInvalidJSON(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "lock.json")
	require.NoError(t, os.WriteFile(lockPath, []byte("not-json"), cacheFilePerm))
	// Touch with recent mod time — should NOT be stale.
	now := time.Now()
	require.NoError(t, os.Chtimes(lockPath, now, now))

	stale, err := isStaleLock(lockPath)
	require.NoError(t, err)
	assert.False(t, stale, "recent modtime with invalid JSON should not be stale")
}

func TestIsStaleLock_RecentValidMetadata(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "lock.json")
	meta := lockMetadata{PID: 12345, CreatedAt: time.Now().UTC()}
	raw, err := json.Marshal(meta)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(lockPath, raw, cacheFilePerm))

	stale, err := isStaleLock(lockPath)
	require.NoError(t, err)
	assert.False(t, stale, "recent metadata should not be stale")
}

func TestIsStaleLock_ZeroCreatedAt(t *testing.T) {
	// Metadata with zero CreatedAt falls through to mod-time check.
	lockPath := filepath.Join(t.TempDir(), "lock.json")
	meta := lockMetadata{PID: 12345} // CreatedAt is zero value
	raw, err := json.Marshal(meta)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(lockPath, raw, cacheFilePerm))
	// Set old mod time so fallback detects staleness.
	old := time.Now().Add(-lockStaleDuration - time.Minute)
	require.NoError(t, os.Chtimes(lockPath, old, old))

	stale, err := isStaleLock(lockPath)
	require.NoError(t, err)
	assert.True(t, stale, "zero createdAt with old modtime should be stale")
}

// --- acquireUpdateLock coverage ---

func TestAcquireUpdateLock_NonExistCreateError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cannot reliably trigger non-EEXIST on Windows")
	}
	// Use a path under a non-existent parent so os.OpenFile returns a
	// non-EEXIST error (ENOENT on the parent directory).
	lockPath := filepath.Join(t.TempDir(), "missing-parent", "subdir", lockFileName)
	_, err := acquireUpdateLock(lockPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "creating lock file")
}

// --- copyWithLimit coverage ---

func TestCopyWithLimit_ExactlyAtLimit(t *testing.T) {
	data := strings.Repeat("x", 100)
	var buf strings.Builder
	err := copyWithLimit(&buf, strings.NewReader(data), 100)
	require.NoError(t, err, "exactly at limit should succeed")
	assert.Equal(t, 100, buf.Len())
}

func TestCopyWithLimit_OneByteBeyondLimit(t *testing.T) {
	data := strings.Repeat("x", 101)
	var buf strings.Builder
	err := copyWithLimit(&buf, strings.NewReader(data), 100)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPayloadTooLarge)
}

// --- replaceWindows companion binary coverage ---

func TestReplaceWindows_CompanionUpdate(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific test")
	}
	tmpDir := t.TempDir()
	// Use a non-standard exe name so the companion ("grut.exe") is different.
	exeName := "grut-dev.exe"
	exePath := filepath.Join(tmpDir, exeName)
	newPath := filepath.Join(tmpDir, "new-binary.exe")
	companionPath := filepath.Join(tmpDir, binaryName+".exe")

	// Create existing exe, companion, and new binary.
	require.NoError(t, os.WriteFile(exePath, []byte("old-main"), newBinaryPerm))
	require.NoError(t, os.WriteFile(companionPath, []byte("old-companion"), newBinaryPerm))
	require.NoError(t, os.WriteFile(newPath, []byte("new-content"), newBinaryPerm))

	err := replaceWindows(newPath, tmpDir, exeName)
	require.NoError(t, err)

	// Verify main binary was updated.
	got, err := os.ReadFile(exePath)
	require.NoError(t, err)
	assert.Equal(t, "new-content", string(got), "main binary should be updated")

	// Verify companion was also updated.
	got, err = os.ReadFile(companionPath)
	require.NoError(t, err)
	assert.Equal(t, "new-content", string(got), "companion binary should be updated")
}

func TestReplaceWindows_CompanionSkipsSelf(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific test")
	}
	tmpDir := t.TempDir()
	// When exeName == "grut.exe", the companion loop should skip it.
	exeName := binaryName + ".exe"
	exePath := filepath.Join(tmpDir, exeName)
	newPath := filepath.Join(tmpDir, "new-binary.exe")

	require.NoError(t, os.WriteFile(exePath, []byte("old"), newBinaryPerm))
	require.NoError(t, os.WriteFile(newPath, []byte("new"), newBinaryPerm))

	err := replaceWindows(newPath, tmpDir, exeName)
	require.NoError(t, err)

	got, err := os.ReadFile(exePath)
	require.NoError(t, err)
	assert.Equal(t, "new", string(got))
}

func TestReplaceWindows_CompanionMissing(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific test")
	}
	tmpDir := t.TempDir()
	exeName := "grut-dev.exe"
	exePath := filepath.Join(tmpDir, exeName)
	newPath := filepath.Join(tmpDir, "new-binary.exe")
	// Companion grut.exe doesn't exist — should be silently skipped.

	require.NoError(t, os.WriteFile(exePath, []byte("old"), newBinaryPerm))
	require.NoError(t, os.WriteFile(newPath, []byte("new"), newBinaryPerm))

	err := replaceWindows(newPath, tmpDir, exeName)
	require.NoError(t, err)

	got, err := os.ReadFile(exePath)
	require.NoError(t, err)
	assert.Equal(t, "new", string(got))
}

// --- replaceUnix coverage ---

func TestReplaceUnix_Success(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}
	tmpDir := t.TempDir()
	exePath := filepath.Join(tmpDir, "grut")
	newPath := filepath.Join(tmpDir, "grut-new")

	require.NoError(t, os.WriteFile(exePath, []byte("old"), newBinaryPerm))
	require.NoError(t, os.WriteFile(newPath, []byte("new"), newBinaryPerm))

	err := replaceUnix(newPath, exePath)
	require.NoError(t, err)

	got, err := os.ReadFile(exePath)
	require.NoError(t, err)
	assert.Equal(t, "new", string(got))
}

// --- fetchLatestVersion coverage ---

func TestFetchLatestVersion_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{invalid json"))
	}))
	defer srv.Close()

	orig := latestReleaseURL
	latestReleaseURL = srv.URL
	defer func() { latestReleaseURL = orig }()

	_, err := fetchLatestVersion()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decoding release response")
}

func TestFetchLatestVersion_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	orig := latestReleaseURL
	latestReleaseURL = srv.URL
	defer func() { latestReleaseURL = orig }()

	_, err := fetchLatestVersion()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 500")
}

func TestFetchLatestVersion_StripsVPrefix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tag_name":"v2.3.4"}`))
	}))
	defer srv.Close()

	orig := latestReleaseURL
	latestReleaseURL = srv.URL
	defer func() { latestReleaseURL = orig }()

	ver, err := fetchLatestVersion()
	require.NoError(t, err)
	assert.Equal(t, "2.3.4", ver)
}

// --- CheckForUpdate error paths ---

func TestCheckForUpdate_NetworkErrorReturnsNil(t *testing.T) {
	orig := latestReleaseURL
	latestReleaseURL = "http://127.0.0.1:1/nonexistent"
	defer func() { latestReleaseURL = orig }()

	// Use a unique version to bypass any cached results.
	result := CheckForUpdate("0.0.1")
	assert.Nil(t, result, "network error should return nil")
}

func TestCheckForUpdate_InvalidVersionFromAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tag_name":"invalid"}`))
	}))
	defer srv.Close()

	orig := latestReleaseURL
	latestReleaseURL = srv.URL
	defer func() { latestReleaseURL = orig }()

	// Use a unique version to bypass any cached results.
	result := CheckForUpdate("0.0.2")
	assert.Nil(t, result, "invalid version from API should return nil")
}

func TestCheckForUpdate_NewerVersionAvailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tag_name":"v99.0.0"}`))
	}))
	defer srv.Close()

	orig := latestReleaseURL
	latestReleaseURL = srv.URL
	defer func() { latestReleaseURL = orig }()

	result := CheckForUpdate("1.0.0")
	require.NotNil(t, result)
	assert.Equal(t, "99.0.0", result.LatestVersion)
	assert.Equal(t, "1.0.0", result.CurrentVersion)
}

func TestCheckForUpdate_SameVersionReturnsNil(t *testing.T) {
	result := CheckForUpdate("dev-abc123")
	assert.Nil(t, result, "dev version should return nil")
}

// --- copyFile edge cases ---

func TestCopyFile_DestinationIsDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "src.bin")
	require.NoError(t, os.WriteFile(src, []byte("data"), 0o644))

	dst := filepath.Join(tmpDir, "dst-dir")
	require.NoError(t, os.MkdirAll(dst, 0o755))

	err := copyFile(src, dst)
	require.Error(t, err)
}

// --- writeCache coverage ---

func TestWriteCache_ParentIsFile(t *testing.T) {
	tmpDir := t.TempDir()
	// Create a file where we need a directory.
	blocker := filepath.Join(tmpDir, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))
	path := filepath.Join(blocker, "subdir", "cache.json")

	// writeCache ignores errors — just verify no panic.
	writeCache(path, &updateCache{
		CheckedAt:      time.Now(),
		LatestVersion:  "1.0.0",
		CurrentVersion: "0.9.0",
	})
}

// --- RunUpdate coverage ---

// isolateUserConfigDir points os.UserConfigDir at a temp directory so tests
// that call RunUpdate don't contend for the real user-level update lock.
// go test runs package binaries concurrently, so without this the cmd
// package's update tests race with these over the same lock file and one
// side fails with "another update is already in progress".
func isolateUserConfigDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("AppData", dir)         // Windows
	t.Setenv("XDG_CONFIG_HOME", dir) // Unix
	t.Setenv("HOME", dir)            // macOS, and Unix fallback
}

func TestRunUpdate_AlreadyUpToDate(t *testing.T) {
	isolateUserConfigDir(t)

	// Mock API returns the same version as current.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tag_name":"v1.0.0"}`))
	}))
	defer srv.Close()

	orig := latestReleaseURL
	latestReleaseURL = srv.URL
	defer func() { latestReleaseURL = orig }()

	err := RunUpdate(context.Background(), "1.0.0")
	require.NoError(t, err) // "already up to date" is not an error
}

func TestRunUpdate_FetchFailure(t *testing.T) {
	isolateUserConfigDir(t)

	// Mock API returns an error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	orig := latestReleaseURL
	latestReleaseURL = srv.URL
	defer func() { latestReleaseURL = orig }()

	err := RunUpdate(context.Background(), "1.0.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checking latest version")
}

func TestRunUpdate_InvalidVersionFromAPI(t *testing.T) {
	isolateUserConfigDir(t)

	// Mock API returns a non-semver version.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tag_name":"not-a-version"}`))
	}))
	defer srv.Close()

	orig := latestReleaseURL
	latestReleaseURL = srv.URL
	defer func() { latestReleaseURL = orig }()

	err := RunUpdate(context.Background(), "1.0.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid latest version")
}

func TestRunUpdate_DownloadFailure(t *testing.T) {
	isolateUserConfigDir(t)

	// Mock API returns a newer version. RunUpdate will try to download
	// the release archive from downloadBaseURL which will fail since
	// the version doesn't exist.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tag_name":"v99.99.99"}`))
	}))
	defer srv.Close()

	orig := latestReleaseURL
	latestReleaseURL = srv.URL
	defer func() { latestReleaseURL = orig }()

	err := RunUpdate(context.Background(), "1.0.0")
	require.Error(t, err)
	// The download will fail because v99.99.99 doesn't exist on GitHub.
	assert.Contains(t, err.Error(), "downloading")
}

// --- extractFromZip additional coverage ---

func TestExtractFromZip_OutputDirNotWritable(t *testing.T) {
	if runtime.GOOS != "windows" {
		// On Unix, we can't easily make a dir non-writable and have
		// zip extraction fail on open in the same way.
		t.Skip("Windows-specific test")
	}
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.zip")

	// Create a valid zip containing grut.exe.
	f, err := os.Create(archivePath)
	require.NoError(t, err)
	zw := zip.NewWriter(f)
	w, err := zw.Create(binaryName + ".exe")
	require.NoError(t, err)
	_, err = w.Write([]byte("binary-content"))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	require.NoError(t, f.Close())

	// Use a non-existent destDir so os.OpenFile fails.
	extractDir := filepath.Join(tmpDir, "no", "such", "path")

	_, err = extractFromZip(archivePath, extractDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "creating")
}

// --- downloadAsset additional coverage ---

func TestDownloadAsset_DestPathNotWritable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data"))
	}))
	defer srv.Close()

	// Use a path that can't be created.
	badPath := filepath.Join(t.TempDir(), "no", "such", "dir", "file.tar.gz")
	err := downloadAsset(context.Background(), badPath, srv.URL+"/asset")
	require.Error(t, err)
}
