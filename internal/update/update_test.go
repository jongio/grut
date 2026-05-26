package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// AssetName / assetNameForPlatform
// ---------------------------------------------------------------------------

func TestAssetNameForPlatform(t *testing.T) {
	tests := []struct {
		version, goos, goarch string
		want                  string
	}{
		{"0.4.1", "linux", "amd64", "grut_0.4.1_linux_amd64.tar.gz"},
		{"0.4.1", "linux", "arm64", "grut_0.4.1_linux_arm64.tar.gz"},
		{"0.4.1", "darwin", "amd64", "grut_0.4.1_darwin_amd64.tar.gz"},
		{"0.4.1", "darwin", "arm64", "grut_0.4.1_darwin_arm64.tar.gz"},
		{"0.4.1", "windows", "amd64", "grut_0.4.1_windows_amd64.zip"},
		{"0.4.1", "windows", "arm64", "grut_0.4.1_windows_arm64.zip"},
		{"1.0.0", "linux", "amd64", "grut_1.0.0_linux_amd64.tar.gz"},
	}
	for _, tt := range tests {
		name := tt.goos + "_" + tt.goarch + "_v" + tt.version
		t.Run(name, func(t *testing.T) {
			got := assetNameForPlatform(tt.version, tt.goos, tt.goarch)
			if got != tt.want {
				t.Errorf("assetNameForPlatform(%q, %q, %q) = %q, want %q",
					tt.version, tt.goos, tt.goarch, got, tt.want)
			}
		})
	}
}

func TestAssetName_CurrentPlatform(t *testing.T) {
	name := AssetName("1.2.3")
	if name == "" {
		t.Fatal("AssetName returned empty string")
	}
	// Should contain the version.
	if got := name; got == "" {
		t.Error("AssetName should not be empty")
	}
}

// ---------------------------------------------------------------------------
// ParseChecksum
// ---------------------------------------------------------------------------

func TestParseChecksum(t *testing.T) {
	content := `aabbccdd1122334455667788aabbccdd1122334455667788aabbccdd11223344  grut_0.4.1_linux_amd64.tar.gz
11223344556677889900aabbccddeeff11223344556677889900aabbccddeeff  grut_0.4.1_windows_amd64.zip
ffeeddccbbaa99887766554433221100ffeeddccbbaa99887766554433221100  grut_0.4.1_darwin_arm64.tar.gz
`

	tests := []struct {
		filename string
		want     string
		wantErr  bool
	}{
		{
			"grut_0.4.1_linux_amd64.tar.gz",
			"aabbccdd1122334455667788aabbccdd1122334455667788aabbccdd11223344",
			false,
		},
		{
			"grut_0.4.1_windows_amd64.zip",
			"11223344556677889900aabbccddeeff11223344556677889900aabbccddeeff",
			false,
		},
		{
			"grut_0.4.1_darwin_arm64.tar.gz",
			"ffeeddccbbaa99887766554433221100ffeeddccbbaa99887766554433221100",
			false,
		},
		{"grut_0.4.1_freebsd_amd64.tar.gz", "", true}, // missing
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got, err := ParseChecksum(content, tt.filename)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("ParseChecksum() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseChecksum_EmptyContent(t *testing.T) {
	_, err := ParseChecksum("", "grut_0.4.1_linux_amd64.tar.gz")
	if err == nil {
		t.Error("expected error for empty content")
	}
}

func TestParseChecksum_MixedCase(t *testing.T) {
	content := `AABBCCDD1122334455667788AABBCCDD1122334455667788AABBCCDD11223344  grut_0.4.1_linux_amd64.tar.gz`

	got, err := ParseChecksum(content, "grut_0.4.1_linux_amd64.tar.gz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should be normalized to lowercase.
	want := "aabbccdd1122334455667788aabbccdd1122334455667788aabbccdd11223344"
	if got != want {
		t.Errorf("ParseChecksum() = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// SHA256File
// ---------------------------------------------------------------------------

func TestSHA256File(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.bin")
	data := []byte("hello grut update test")

	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := SHA256File(path)
	if err != nil {
		t.Fatalf("SHA256File: %v", err)
	}

	// Compute expected hash.
	h := sha256.Sum256(data)
	want := hex.EncodeToString(h[:])

	if got != want {
		t.Errorf("SHA256File() = %q, want %q", got, want)
	}
}

func TestSHA256File_Missing(t *testing.T) {
	_, err := SHA256File(filepath.Join(t.TempDir(), "nonexistent"))
	if err == nil {
		t.Error("expected error for missing file")
	}
}

// ---------------------------------------------------------------------------
// extractFromTarGz
// ---------------------------------------------------------------------------

func TestExtractFromTarGz(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.tar.gz")
	binaryContent := []byte("#!/bin/sh\necho hello")

	// Create a tar.gz archive with a "grut" entry.
	if err := createTestTarGz(archivePath, "grut", binaryContent); err != nil {
		t.Fatalf("creating test archive: %v", err)
	}

	extractDir := filepath.Join(tmpDir, "extract")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := extractFromTarGz(archivePath, extractDir)
	if err != nil {
		t.Fatalf("extractFromTarGz: %v", err)
	}

	if filepath.Base(got) != "grut" {
		t.Errorf("extracted file name = %q, want %q", filepath.Base(got), "grut")
	}

	content, err := os.ReadFile(got)
	if err != nil {
		t.Fatalf("reading extracted file: %v", err)
	}
	if string(content) != string(binaryContent) {
		t.Errorf("extracted content = %q, want %q", string(content), string(binaryContent))
	}
}

func TestExtractFromTarGz_NoBinary(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.tar.gz")

	// Create a tar.gz archive with a different file name.
	if err := createTestTarGz(archivePath, "other-file", []byte("data")); err != nil {
		t.Fatalf("creating test archive: %v", err)
	}

	extractDir := filepath.Join(tmpDir, "extract")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := extractFromTarGz(archivePath, extractDir)
	if err == nil {
		t.Error("expected error when binary not found in archive")
	}
}

// ---------------------------------------------------------------------------
// extractFromZip
// ---------------------------------------------------------------------------

func TestExtractFromZip(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.zip")
	binaryContent := []byte("MZ fake exe content")

	// Create a zip archive with a "grut.exe" entry.
	if err := createTestZip(archivePath, "grut.exe", binaryContent); err != nil {
		t.Fatalf("creating test zip: %v", err)
	}

	extractDir := filepath.Join(tmpDir, "extract")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := extractFromZip(archivePath, extractDir)
	if err != nil {
		t.Fatalf("extractFromZip: %v", err)
	}

	if filepath.Base(got) != "grut.exe" {
		t.Errorf("extracted file name = %q, want %q", filepath.Base(got), "grut.exe")
	}

	content, err := os.ReadFile(got)
	if err != nil {
		t.Fatalf("reading extracted file: %v", err)
	}
	if string(content) != string(binaryContent) {
		t.Errorf("extracted content = %q, want %q", string(content), string(binaryContent))
	}
}

func TestExtractFromZip_NoBinary(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.zip")

	if err := createTestZip(archivePath, "readme.txt", []byte("readme")); err != nil {
		t.Fatalf("creating test zip: %v", err)
	}

	extractDir := filepath.Join(tmpDir, "extract")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := extractFromZip(archivePath, extractDir)
	if err == nil {
		t.Error("expected error when binary not found in zip")
	}
}

// ---------------------------------------------------------------------------
// extractBinary (routing)
// ---------------------------------------------------------------------------

func TestExtractBinary_RoutesToZip(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.zip")
	if err := createTestZip(archivePath, "grut.exe", []byte("exe")); err != nil {
		t.Fatal(err)
	}

	extractDir := filepath.Join(tmpDir, "extract")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := extractBinary(archivePath, extractDir)
	if err != nil {
		t.Errorf("extractBinary(.zip) failed: %v", err)
	}
}

func TestExtractBinary_RoutesToTarGz(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.tar.gz")
	if err := createTestTarGz(archivePath, "grut", []byte("bin")); err != nil {
		t.Fatal(err)
	}

	extractDir := filepath.Join(tmpDir, "extract")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := extractBinary(archivePath, extractDir)
	if err != nil {
		t.Errorf("extractBinary(.tar.gz) failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// copyFile
// ---------------------------------------------------------------------------

func TestCopyFile(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "src.bin")
	dst := filepath.Join(tmpDir, "dst.bin")
	content := []byte("binary content for copy test")

	if err := os.WriteFile(src, content, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("reading dst: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("copied content = %q, want %q", string(got), string(content))
	}
}

func TestCopyFile_MissingSrc(t *testing.T) {
	tmpDir := t.TempDir()
	err := copyFile(filepath.Join(tmpDir, "nope"), filepath.Join(tmpDir, "dst"))
	if err == nil {
		t.Error("expected error for missing source")
	}
}

// ---------------------------------------------------------------------------
// RunUpdate edge cases
// ---------------------------------------------------------------------------

func TestReplaceUnix(t *testing.T) {
	tmpDir := t.TempDir()
	exePath := filepath.Join(tmpDir, "grut")
	newPath := filepath.Join(tmpDir, "grut-new")

	if err := os.WriteFile(exePath, []byte("old binary"), 0o755); err != nil {
		t.Fatalf("write old binary: %v", err)
	}
	if err := os.WriteFile(newPath, []byte("new binary"), 0o755); err != nil {
		t.Fatalf("write new binary: %v", err)
	}

	err := replaceUnix(newPath, exePath)
	if err != nil {
		t.Fatalf("replaceUnix: %v", err)
	}

	got, err := os.ReadFile(exePath)
	if err != nil {
		t.Fatalf("reading replaced file: %v", err)
	}
	if string(got) != "new binary" {
		t.Errorf("replaced content = %q, want %q", string(got), "new binary")
	}

	if _, err := os.Stat(exePath + ".new"); !errors.Is(err, fs.ErrNotExist) {
		t.Error("expected .new temp file to be cleaned up")
	}
}

func TestReplaceWindows(t *testing.T) {
	tmpDir := t.TempDir()
	exeName := "grut.exe"
	exePath := filepath.Join(tmpDir, exeName)
	newPath := filepath.Join(tmpDir, "grut-new.exe")

	if err := os.WriteFile(exePath, []byte("old exe"), 0o755); err != nil {
		t.Fatalf("write old exe: %v", err)
	}
	if err := os.WriteFile(newPath, []byte("new exe"), 0o755); err != nil {
		t.Fatalf("write new exe: %v", err)
	}

	err := replaceWindows(newPath, tmpDir, exeName)
	if err != nil {
		t.Fatalf("replaceWindows: %v", err)
	}

	got, err := os.ReadFile(exePath)
	if err != nil {
		t.Fatalf("reading replaced exe: %v", err)
	}
	if string(got) != "new exe" {
		t.Errorf("replaced content = %q, want %q", string(got), "new exe")
	}
}

func TestDownloadAsset(t *testing.T) {
	content := []byte("fake binary content for download test")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	dst := filepath.Join(tmpDir, "downloaded.tar.gz")

	err := downloadAsset(context.Background(), dst, srv.URL+"/asset.tar.gz")
	if err != nil {
		t.Fatalf("downloadAsset: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("downloaded content = %q, want %q", string(got), string(content))
	}
}

func TestDownloadAsset_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	dst := filepath.Join(tmpDir, "download.tar.gz")

	err := downloadAsset(context.Background(), dst, srv.URL+"/missing")
	if err == nil {
		t.Fatal("expected error for HTTP 404")
	}
}

func TestVerifyChecksum_Valid(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "archive.tar.gz")
	content := []byte("archive content for checksum")
	if err := os.WriteFile(archivePath, content, 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}

	h := sha256.Sum256(content)
	hash := hex.EncodeToString(h[:])

	checksumContent := hash + "  archive.tar.gz\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(checksumContent))
	}))
	defer srv.Close()

	err := verifyChecksum(context.Background(), archivePath, srv.URL+"/checksums.txt", "archive.tar.gz")
	if err != nil {
		t.Fatalf("verifyChecksum: %v", err)
	}
}

func TestVerifyChecksum_Mismatch(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "archive.tar.gz")
	if err := os.WriteFile(archivePath, []byte("actual content"), 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}

	checksumContent := "0000000000000000000000000000000000000000000000000000000000000000  archive.tar.gz\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(checksumContent))
	}))
	defer srv.Close()

	err := verifyChecksum(context.Background(), archivePath, srv.URL+"/checksums.txt", "archive.tar.gz")
	if err == nil {
		t.Fatal("expected error for checksum mismatch")
	}
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Errorf("error should wrap ErrChecksumMismatch, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// createTestTarGz creates a .tar.gz archive containing a single file.
func createTestTarGz(archivePath, filename string, content []byte) error {
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
		Typeflag: tar.TypeReg,
		Name:     filename,
		Mode:     0o755,
		Size:     int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	if _, err := tw.Write(content); err != nil {
		return err
	}
	return nil
}

// createTestZip creates a .zip archive containing a single file.
func createTestZip(archivePath, filename string, content []byte) error {
	f, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck

	zw := zip.NewWriter(f)
	defer zw.Close() //nolint:errcheck

	w, err := zw.Create(filename)
	if err != nil {
		return err
	}
	if _, err := w.Write(content); err != nil {
		return err
	}
	return nil
}
