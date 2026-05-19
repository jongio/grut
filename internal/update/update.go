package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const (
	// downloadTimeout limits the overall duration for downloading
	// release assets.
	downloadTimeout = 120 * time.Second

	// maxDownloadSize limits the size of downloaded release archives to
	// prevent resource exhaustion from unexpectedly large responses.
	maxDownloadSize int64 = 256 << 20 // 256 MiB

	// maxRedirects caps the number of HTTP redirects followed during
	// asset download.
	maxRedirects = 10

	// checksumFileName is the name of the SHA-256 checksum file in each
	// release.
	checksumFileName = "grut_checksums.txt"

	// binaryName is the base name of the grut executable.
	binaryName = "grut"

	// oldBinarySuffix is appended to the current binary on Windows before
	// replacement, since a running executable cannot be deleted.
	oldBinarySuffix = ".old"

	// newBinaryPerm is the file permission for the extracted binary.
	newBinaryPerm = 0o755

	// maxChecksumSize limits the checksums file read to 1 MiB.
	maxChecksumSize int64 = 1 << 20
)

var (
	updateHTTPTransport http.RoundTripper = http.DefaultTransport
	versionPattern                        = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

	// downloadBaseURL is the base URL for GitHub release asset downloads.
	// Declared as var (not const) to allow test overrides.
	downloadBaseURL = "https://github.com/jongio/grut/releases/download"

	// Sentinel errors for testable error checking via errors.Is().
	ErrTooManyRedirects   = errors.New("too many redirects")
	ErrPayloadTooLarge    = errors.New("payload exceeds size limit")
	ErrUnsafeArchivePath  = errors.New("unsafe archive entry path")
	ErrChecksumMismatch   = errors.New("checksum mismatch")
	ErrChecksumNotFound   = errors.New("no checksum found")
	ErrUnsupportedTarType = errors.New("unsupported tar entry type")
	ErrBinaryNotFound     = errors.New("binary not found in archive")
)

// validateVersion ensures v looks like a semantic version (major.minor.patch).
func validateVersion(v string) error {
	if !versionPattern.MatchString(v) {
		return fmt.Errorf("invalid version %q: expected semantic version in major.minor.patch format", v)
	}
	return nil
}

func httpsOnlyCheckRedirect(req *http.Request, via []*http.Request) error {
	if req.URL.Scheme != "https" {
		return fmt.Errorf("refusing redirect to non-HTTPS URL: %s", req.URL)
	}
	if len(via) >= maxRedirects {
		return ErrTooManyRedirects
	}
	return nil
}

func newSecureClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:       timeout,
		Transport:     updateHTTPTransport,
		CheckRedirect: httpsOnlyCheckRedirect,
	}
}

// copyWithLimit copies src to dst, returning an error if more than
// maxBytes are read.
func copyWithLimit(dst io.Writer, src io.Reader, maxBytes int64) error {
	limited := &io.LimitedReader{R: src, N: maxBytes + 1}
	written, err := io.Copy(dst, limited)
	if err != nil {
		return err
	}
	if written > maxBytes {
		return fmt.Errorf("payload exceeds %d bytes: %w", maxBytes, ErrPayloadTooLarge)
	}
	return nil
}

// normalizeArchiveEntryName normalises slashes and strips leading "./"
// so that archive entries can be compared safely.
func normalizeArchiveEntryName(name string) string {
	normalized := strings.ReplaceAll(name, "\\", "/")
	normalized = strings.TrimPrefix(normalized, "./")
	return path.Clean(normalized)
}

// matchArchiveTarget returns true when the archive entry name matches
// target exactly after normalisation.  If the base name matches but the
// full path does not, it returns an error (possible path traversal).
func matchArchiveTarget(name, target string) (bool, error) {
	normalized := normalizeArchiveEntryName(name)
	if normalized == target {
		return true, nil
	}
	if path.Base(normalized) == target {
		return false, fmt.Errorf("%w: %q", ErrUnsafeArchivePath, name)
	}
	return false, nil
}

// RunUpdate downloads and installs the latest version of grut. It
// prints progress to stderr and returns an error on failure.
func RunUpdate(ctx context.Context, currentVersion string) error {
	// Acquire an exclusive lock to prevent concurrent updates.
	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("resolving config directory: %w", err)
	}
	lockDir := filepath.Join(configDir, "grut")
	if err := os.MkdirAll(lockDir, configDirPerm); err != nil {
		return fmt.Errorf("ensuring config directory: %w", err)
	}
	lockPath := filepath.Join(lockDir, lockFileName)
	lock, err := acquireUpdateLock(lockPath)
	if err != nil {
		return fmt.Errorf("another update is already in progress: %w", err)
	}
	defer releaseUpdateLock(lock)

	latest, err := fetchLatestVersion()
	if err != nil {
		return fmt.Errorf("checking latest version: %w", err)
	}
	if err := validateVersion(latest); err != nil {
		return fmt.Errorf("invalid latest version: %w", err)
	}

	if CompareVersions(latest, currentVersion) <= 0 {
		fmt.Fprintf(os.Stderr, "grut is already up to date (v%s)\n", currentVersion)
		return nil
	}

	fmt.Fprintf(os.Stderr, "Downloading grut v%s...\n", latest)

	// Create a temporary directory for the download and extraction.
	tmpDir, err := os.MkdirTemp("", "grut-update-*")
	if err != nil {
		return fmt.Errorf("creating temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir) //nolint:errcheck // best-effort cleanup

	// Download the release archive.
	asset := AssetName(latest)
	assetURL := fmt.Sprintf("%s/v%s/%s", downloadBaseURL, latest, asset)
	archivePath := filepath.Join(tmpDir, asset)
	if err := downloadAsset(ctx, archivePath, assetURL); err != nil {
		return fmt.Errorf("downloading %s: %w", asset, err)
	}

	// Download the checksum file.
	checksumURL := fmt.Sprintf("%s/v%s/%s", downloadBaseURL, latest, checksumFileName)
	checksumData, err := downloadChecksumData(ctx, checksumURL)
	if err != nil {
		return fmt.Errorf("downloading checksums: %w", err)
	}

	// Verify the cosign signature on the checksum file before trusting it.
	// This prevents a compromised release from silently replacing both the
	// archive and checksum file (CWE-347).
	fmt.Fprintf(os.Stderr, "Verifying cosign signature...\n")
	if err := verifyCosignChecksums(ctx, checksumData, latest); err != nil {
		return fmt.Errorf("cosign signature verification: %w", err)
	}

	// Verify the archive hash against the authenticated checksum data.
	if err := verifyArchiveChecksum(archivePath, checksumData, asset); err != nil {
		return fmt.Errorf("checksum verification: %w", err)
	}

	// Extract the binary from the archive.
	binPath, err := extractBinary(archivePath, tmpDir)
	if err != nil {
		return fmt.Errorf("extracting binary: %w", err)
	}

	// Replace the running binary.
	if err := replaceBinary(binPath); err != nil {
		return fmt.Errorf("replacing binary: %w", err)
	}

	// Update the cache to reflect the new version.
	writeCache("", &updateCache{
		CheckedAt:      time.Now(),
		LatestVersion:  latest,
		CurrentVersion: latest,
	})

	fmt.Fprintf(os.Stderr, "Updated grut: v%s → v%s\n", currentVersion, latest)
	return nil
}

// AssetName returns the expected release archive filename for the current
// platform and architecture.
func AssetName(version string) string {
	return assetNameForPlatform(version, runtime.GOOS, runtime.GOARCH)
}

// assetNameForPlatform returns the archive filename for a given platform.
func assetNameForPlatform(version, goos, goarch string) string {
	ext := "tar.gz"
	if goos == "windows" { //nolint:goconst // standard platform literal
		ext = "zip"
	}
	return fmt.Sprintf("grut_%s_%s_%s.%s", version, goos, goarch, ext)
}

// downloadAsset downloads a URL to a local file path with HTTPS-only
// redirect enforcement and size limits.
func downloadAsset(ctx context.Context, dst, url string) error {
	client := newSecureClient(downloadTimeout)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil) //nolint:gosec // URL built from constants+version
	if err != nil {
		return fmt.Errorf("creating request for %s: %w", url, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", url, err)
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort cleanup

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	if resp.ContentLength > maxDownloadSize {
		return fmt.Errorf("download exceeds %d bytes: %w", maxDownloadSize, ErrPayloadTooLarge)
	}

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("creating %s: %w", dst, err)
	}

	if err := copyWithLimit(out, resp.Body, maxDownloadSize); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return fmt.Errorf("writing %s: %w", dst, err)
	}

	return out.Close()
}

// verifyChecksum downloads the checksum file and verifies the SHA-256
// hash of the downloaded archive.
func verifyChecksum(ctx context.Context, archivePath, checksumURL, archiveName string) error {
	checksumData, err := downloadChecksumData(ctx, checksumURL)
	if err != nil {
		return err
	}
	return verifyArchiveChecksum(archivePath, checksumData, archiveName)
}

// downloadChecksumData downloads and returns the raw checksum file content.
func downloadChecksumData(ctx context.Context, checksumURL string) ([]byte, error) {
	client := newSecureClient(apiTimeout)

	csReq, err := http.NewRequestWithContext(ctx, http.MethodGet, checksumURL, nil) //nolint:gosec // URL built from constants
	if err != nil {
		return nil, fmt.Errorf("creating checksum request: %w", err)
	}
	resp, err := client.Do(csReq)
	if err != nil {
		return nil, fmt.Errorf("downloading checksums: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort cleanup

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d fetching checksums", resp.StatusCode)
	}
	if resp.ContentLength > maxChecksumSize {
		return nil, fmt.Errorf("checksums file exceeds %d bytes", maxChecksumSize)
	}

	checksumData, err := io.ReadAll(io.LimitReader(resp.Body, maxChecksumSize+1))
	if err != nil {
		return nil, fmt.Errorf("reading checksums: %w", err)
	}
	if int64(len(checksumData)) > maxChecksumSize {
		return nil, fmt.Errorf("checksums file exceeds %d bytes", maxChecksumSize)
	}

	return checksumData, nil
}

// verifyArchiveChecksum verifies the SHA-256 hash of a downloaded archive
// against pre-downloaded checksum data.
func verifyArchiveChecksum(archivePath string, checksumData []byte, archiveName string) error {
	// Find the expected hash for our archive.
	expectedHash, err := ParseChecksum(string(checksumData), archiveName)
	if err != nil {
		return fmt.Errorf("parsing checksum for %s: %w", archiveName, err)
	}

	// Compute the actual hash of the downloaded file.
	actualHash, err := SHA256File(archivePath)
	if err != nil {
		return fmt.Errorf("computing checksum: %w", err)
	}

	if actualHash != expectedHash {
		return fmt.Errorf("%w: expected %s, got %s", ErrChecksumMismatch, expectedHash, actualHash)
	}

	return nil
}

// ParseChecksum extracts the SHA-256 hash for a specific file from a
// checksums.txt file in "hash  filename\n" format.
func ParseChecksum(content, filename string) (string, error) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format: "<hash>  <filename>" (two spaces) or "<hash> <filename>"
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == filename {
			return strings.ToLower(parts[0]), nil
		}
	}
	return "", fmt.Errorf("%w for %s", ErrChecksumNotFound, filename)
}

// SHA256File computes the SHA-256 hash of a file and returns it as a
// lowercase hex string.
func SHA256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close() //nolint:errcheck // read-only

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// extractBinary extracts the grut binary from a release archive
// (tar.gz or zip) and returns the path to the extracted file.
func extractBinary(archivePath, destDir string) (string, error) {
	if strings.HasSuffix(archivePath, ".zip") {
		return extractFromZip(archivePath, destDir)
	}
	return extractFromTarGz(archivePath, destDir)
}

// extractFromTarGz extracts the grut binary from a .tar.gz archive.
func extractFromTarGz(archivePath, destDir string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("opening archive: %w", err)
	}
	defer f.Close() //nolint:errcheck // read-only

	gr, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("decompressing archive: %w", err)
	}
	defer gr.Close() //nolint:errcheck // read-only

	tr := tar.NewReader(gr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("reading tar entry: %w", err)
		}

		match, err := matchArchiveTarget(header.Name, binaryName)
		if err != nil {
			return "", err
		}
		if !match {
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != 0 {
			return "", fmt.Errorf("%w for %s", ErrUnsupportedTarType, header.Name)
		}

		dst := filepath.Join(destDir, binaryName)
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, newBinaryPerm)
		if err != nil {
			return "", fmt.Errorf("creating %s: %w", dst, err)
		}

		if err := copyWithLimit(out, tr, maxDownloadSize); err != nil {
			_ = out.Close()
			_ = os.Remove(dst)
			return "", fmt.Errorf("extracting %s: %w", binaryName, err)
		}
		if err := out.Close(); err != nil {
			return "", fmt.Errorf("closing %s: %w", dst, err)
		}
		return dst, nil
	}
	return "", fmt.Errorf("grut %w", ErrBinaryNotFound)
}

// extractFromZip extracts the grut binary from a .zip archive.
func extractFromZip(archivePath, destDir string) (string, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", fmt.Errorf("opening zip: %w", err)
	}
	defer r.Close() //nolint:errcheck // read-only

	target := binaryName + ".exe"
	for _, f := range r.File {
		match, err := matchArchiveTarget(f.Name, target)
		if err != nil {
			return "", err
		}
		if !match {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return "", fmt.Errorf("opening %s in zip: %w", f.Name, err)
		}

		dst := filepath.Join(destDir, target)
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, newBinaryPerm)
		if err != nil {
			_ = rc.Close()
			return "", fmt.Errorf("creating %s: %w", dst, err)
		}

		if err := copyWithLimit(out, rc, maxDownloadSize); err != nil {
			_ = out.Close()
			_ = rc.Close()
			_ = os.Remove(dst)
			return "", fmt.Errorf("extracting %s: %w", target, err)
		}

		_ = rc.Close()
		if err := out.Close(); err != nil {
			return "", fmt.Errorf("closing %s: %w", dst, err)
		}
		return dst, nil
	}

	return "", fmt.Errorf("grut.exe %w", ErrBinaryNotFound)
}

// replaceBinary replaces the currently running grut binary with a
// new version. On Unix, it uses an atomic rename. On Windows, it renames
// the running executable to .old before replacing.
func replaceBinary(newBinaryPath string) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving executable path: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("resolving symlinks: %w", err)
	}

	exeDir := filepath.Dir(exePath)
	exeName := filepath.Base(exePath)

	if runtime.GOOS == "windows" {
		return replaceWindows(newBinaryPath, exeDir, exeName)
	}
	return replaceUnix(newBinaryPath, exePath)
}

// replaceUnix atomically replaces the binary via rename.
func replaceUnix(newBinaryPath, exePath string) error {
	// Write to a temp file in the same directory (same filesystem) to
	// ensure os.Rename is atomic.
	tmpPath := exePath + ".new"

	src, err := os.Open(newBinaryPath)
	if err != nil {
		return fmt.Errorf("opening new binary: %w", err)
	}
	defer src.Close() //nolint:errcheck // read-only

	dst, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, newBinaryPerm)
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}

	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("writing temp file: %w", err)
	}

	if err := dst.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Rename(tmpPath, exePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replacing binary: %w", err)
	}

	return nil
}

// replaceWindows renames the running exe to .old, then copies the new
// binary and updates any companion binaries if present.
func replaceWindows(newBinaryPath, exeDir, exeName string) error {
	exePath := filepath.Join(exeDir, exeName)
	oldPath := exePath + oldBinarySuffix

	// Remove any previous .old file.
	_ = os.Remove(oldPath)

	// Rename running exe so we can write the new one.
	if err := os.Rename(exePath, oldPath); err != nil {
		return fmt.Errorf("renaming current binary: %w", err)
	}

	if err := copyFile(newBinaryPath, exePath); err != nil {
		rollbackErr := os.Rename(oldPath, exePath)
		if rollbackErr != nil {
			return fmt.Errorf("installing new binary: %w", errors.Join(err, fmt.Errorf("restoring original binary: %w", rollbackErr)))
		}
		return fmt.Errorf("installing new binary: %w", err)
	}

	// Best-effort cleanup of old binary.
	_ = os.Remove(oldPath)

	// Update companion binaries (e.g. grut.exe alias). Skip the binary
	// we just replaced to avoid a self-referencing rename. Copy directly
	// from the extracted newBinaryPath to avoid Windows file-locking
	// issues on the just-written exe.
	companions := []string{binaryName + ".exe"}
	for _, name := range companions {
		if strings.EqualFold(name, exeName) {
			continue
		}
		companionPath := filepath.Join(exeDir, name)
		if _, err := os.Stat(companionPath); err != nil {
			continue
		}
		oldCompanion := companionPath + oldBinarySuffix
		_ = os.Remove(oldCompanion)
		if err := os.Rename(companionPath, oldCompanion); err != nil {
			continue
		}
		if err := copyFile(newBinaryPath, companionPath); err != nil {
			_ = os.Rename(oldCompanion, companionPath) // restore on failure
			continue
		}
		_ = os.Remove(oldCompanion)
	}

	return nil
}

// copyFile copies a file from src to dst, preserving executable
// permissions.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening source %s: %w", src, err)
	}
	defer in.Close() //nolint:errcheck // read-only

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, newBinaryPerm)
	if err != nil {
		return fmt.Errorf("creating destination %s: %w", dst, err)
	}

	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}

	return out.Close()
}
