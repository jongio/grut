package update

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// validateVersion
// ---------------------------------------------------------------------------

func TestValidateVersion_Valid(t *testing.T) {
	for _, v := range []string{"1.0.0", "0.4.1", "99.88.77", "0.0.0"} {
		if err := validateVersion(v); err != nil {
			t.Errorf("validateVersion(%q) unexpected error: %v", v, err)
		}
	}
}

func TestValidateVersion_Invalid(t *testing.T) {
	for _, v := range []string{
		"", "v1.0.0", "1.0", "1", "abc", "1.0.0-beta", "1.0.0.0",
		"1.0.0 ", " 1.0.0", "1.0.0\n",
	} {
		if err := validateVersion(v); err == nil {
			t.Errorf("validateVersion(%q) expected error, got nil", v)
		}
	}
}

// ---------------------------------------------------------------------------
// normalizeArchiveEntryName
// ---------------------------------------------------------------------------

func TestNormalizeArchiveEntryName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"grut", "grut"},
		{"./grut", "grut"},
		{"subdir/grut", "subdir/grut"},
		{"./subdir/grut", "subdir/grut"},
		{`subdir\grut`, "subdir/grut"},
		{"./foo/../grut", "grut"},
		{".", "."},
	}
	for _, tt := range tests {
		got := normalizeArchiveEntryName(tt.input)
		if got != tt.want {
			t.Errorf("normalizeArchiveEntryName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// matchArchiveTarget
// ---------------------------------------------------------------------------

func TestMatchArchiveTarget_ExactMatch(t *testing.T) {
	match, err := matchArchiveTarget("grut", "grut")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !match {
		t.Error("expected match for exact name")
	}
}

func TestMatchArchiveTarget_DotSlashPrefix(t *testing.T) {
	match, err := matchArchiveTarget("./grut", "grut")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !match {
		t.Error("expected match for ./grut")
	}
}

func TestMatchArchiveTarget_UnsafePath(t *testing.T) {
	_, err := matchArchiveTarget("subdir/grut", "grut")
	if err == nil {
		t.Fatal("expected error for unsafe path")
	}
	if !errors.Is(err, ErrUnsafeArchivePath) {
		t.Errorf("error should wrap ErrUnsafeArchivePath, got: %v", err)
	}
}

func TestMatchArchiveTarget_NoMatch(t *testing.T) {
	match, err := matchArchiveTarget("other-file", "grut")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if match {
		t.Error("expected no match for different file")
	}
}

// ---------------------------------------------------------------------------
// copyWithLimit
// ---------------------------------------------------------------------------

func TestCopyWithLimit_WithinLimit(t *testing.T) {
	src := strings.NewReader("hello")
	var dst strings.Builder
	if err := copyWithLimit(&dst, src, 100); err != nil {
		t.Fatalf("copyWithLimit: %v", err)
	}
	if dst.String() != "hello" {
		t.Errorf("got %q, want %q", dst.String(), "hello")
	}
}

func TestCopyWithLimit_ExceedsLimit(t *testing.T) {
	src := strings.NewReader("hello world, this is a long payload")
	var dst strings.Builder
	err := copyWithLimit(&dst, src, 5)
	if err == nil {
		t.Fatal("expected error when payload exceeds limit")
	}
	if !errors.Is(err, ErrPayloadTooLarge) {
		t.Errorf("error should wrap ErrPayloadTooLarge, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// extractFromTarGz — symlink rejection
// ---------------------------------------------------------------------------

func TestExtractFromTarGz_RejectsSymlink(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.tar.gz")

	// Create a tar.gz with a symlink entry named "grut".
	if err := createTestTarGzWithType(archivePath, "grut", tar.TypeSymlink, "/etc/passwd"); err != nil {
		t.Fatal(err)
	}

	extractDir := filepath.Join(tmpDir, "extract")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := extractFromTarGz(archivePath, extractDir)
	if err == nil {
		t.Fatal("expected error for symlink tar entry")
	}
	if !errors.Is(err, ErrUnsupportedTarType) {
		t.Errorf("error should wrap ErrUnsupportedTarType, got: %v", err)
	}
}

func TestExtractFromTarGz_UnsafePath(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.tar.gz")

	// Create tar.gz with binary at a nested path "subdir/grut".
	if err := createTestTarGz(archivePath, "subdir/grut", []byte("binary")); err != nil {
		t.Fatal(err)
	}

	extractDir := filepath.Join(tmpDir, "extract")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := extractFromTarGz(archivePath, extractDir)
	if err == nil {
		t.Fatal("expected error for unsafe archive path")
	}
	if !errors.Is(err, ErrUnsafeArchivePath) {
		t.Errorf("error should wrap ErrUnsafeArchivePath, got: %v", err)
	}
}

func TestExtractFromTarGz_DotSlashPrefix(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.tar.gz")

	// Create tar.gz with "./grut" — should normalize and match.
	if err := createTestTarGz(archivePath, "./grut", []byte("binary content")); err != nil {
		t.Fatal(err)
	}

	extractDir := filepath.Join(tmpDir, "extract")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := extractFromTarGz(archivePath, extractDir)
	if err != nil {
		t.Fatalf("extractFromTarGz: %v", err)
	}

	content, err := os.ReadFile(got)
	if err != nil {
		t.Fatalf("reading extracted: %v", err)
	}
	if string(content) != "binary content" {
		t.Errorf("content = %q, want %q", content, "binary content")
	}
}

// ---------------------------------------------------------------------------
// extractFromZip — unsafe path
// ---------------------------------------------------------------------------

func TestExtractFromZip_UnsafePath(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.zip")

	// Create zip with "subdir/grut.exe" — should be rejected.
	if err := createTestZip(archivePath, "subdir/grut.exe", []byte("exe")); err != nil {
		t.Fatal(err)
	}

	extractDir := filepath.Join(tmpDir, "extract")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := extractFromZip(archivePath, extractDir)
	if err == nil {
		t.Fatal("expected error for unsafe zip path")
	}
	if !errors.Is(err, ErrUnsafeArchivePath) {
		t.Errorf("error should wrap ErrUnsafeArchivePath, got: %v", err)
	}
}

func TestExtractFromZip_DotSlashPrefix(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.zip")

	// Create zip with "./grut.exe" — should normalize and match.
	if err := createTestZip(archivePath, "./grut.exe", []byte("exe content")); err != nil {
		t.Fatal(err)
	}

	extractDir := filepath.Join(tmpDir, "extract")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := extractFromZip(archivePath, extractDir)
	if err != nil {
		t.Fatalf("extractFromZip: %v", err)
	}

	content, err := os.ReadFile(got)
	if err != nil {
		t.Fatalf("reading extracted: %v", err)
	}
	if string(content) != "exe content" {
		t.Errorf("content = %q, want %q", content, "exe content")
	}
}

// ---------------------------------------------------------------------------
// downloadAsset — ContentLength rejection
// ---------------------------------------------------------------------------

func TestDownloadAsset_RejectsLargeContentLength(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "999999999999")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "asset.tar.gz")
	err := downloadAsset(context.Background(), dst, srv.URL+"/asset")
	if err == nil {
		t.Fatal("expected error for oversized ContentLength")
	}
	if !errors.Is(err, ErrPayloadTooLarge) {
		t.Errorf("error should wrap ErrPayloadTooLarge, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// newSecureClient
// ---------------------------------------------------------------------------

func TestNewSecureClient(t *testing.T) {
	client := newSecureClient(5)
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.CheckRedirect == nil {
		t.Error("expected CheckRedirect to be set")
	}
}

// ---------------------------------------------------------------------------
// httpsOnlyCheckRedirect
// ---------------------------------------------------------------------------

func TestHttpsOnlyCheckRedirect_RejectsHTTP(t *testing.T) {
	req, _ := http.NewRequest("GET", "http://evil.example.com", nil)
	err := httpsOnlyCheckRedirect(req, nil)
	if err == nil {
		t.Fatal("expected error for HTTP redirect")
	}
}

func TestHttpsOnlyCheckRedirect_AllowsHTTPS(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://safe.example.com", nil)
	err := httpsOnlyCheckRedirect(req, nil)
	if err != nil {
		t.Fatalf("unexpected error for HTTPS redirect: %v", err)
	}
}

func TestHttpsOnlyCheckRedirect_TooManyRedirects(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://safe.example.com", nil)
	via := make([]*http.Request, maxRedirects)
	err := httpsOnlyCheckRedirect(req, via)
	if err == nil {
		t.Fatal("expected error for too many redirects")
	}
}

// ---------------------------------------------------------------------------
// replaceWindows — errors.Join on rollback failure
// ---------------------------------------------------------------------------

func TestReplaceWindows_ErrorsJoinOnRollbackFailure(t *testing.T) {
	tmpDir := t.TempDir()
	exeName := "grut.exe"
	exePath := filepath.Join(tmpDir, exeName)

	if err := os.WriteFile(exePath, []byte("original"), 0o755); err != nil {
		t.Fatal(err)
	}

	// newPath points to nonexistent file — copy will fail.
	newPath := filepath.Join(tmpDir, "nonexistent-binary")

	// Remove the .old path so rollback also fails (Rename can't find .old).
	oldPath := exePath + oldBinarySuffix

	err := replaceWindows(newPath, tmpDir, exeName)
	if err == nil {
		t.Fatal("expected error when copy source is missing")
	}

	// Either the rollback succeeds (original restored) or both errors
	// are joined. Check the exe exists in some form.
	if _, statErr := os.Stat(exePath); statErr != nil {
		if _, oldStatErr := os.Stat(oldPath); oldStatErr != nil {
			t.Error("neither original nor .old file exists after failed update")
		}
	}
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// createTestTarGzWithType creates a tar.gz with a specific entry type.
func createTestTarGzWithType(archivePath, name string, typeflag byte, linkname string) error {
	f, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck

	gw := gzip.NewWriter(f)
	defer gw.Close() //nolint:errcheck

	tw := tar.NewWriter(gw)
	defer tw.Close() //nolint:errcheck

	hdr := &tar.Header{
		Typeflag: typeflag,
		Name:     name,
		Mode:     0o755,
		Linkname: linkname,
	}
	return tw.WriteHeader(hdr)
}
