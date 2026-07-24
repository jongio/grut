package preview

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxArchiveEntries    = 1000
	archiveModTimeLayout = "2006-01-02 15:04"
	// maxArchiveScanBytes caps how many (decompressed) bytes are read while
	// listing a tar/tar.gz table of contents. It guards against decompression
	// bombs (a tiny gzip stream expanding to gigabytes) and archives with a
	// pathological number of entries, both of which would otherwise make the
	// scan consume unbounded CPU/time. Zip listings read only the central
	// directory and are bounded separately by maxArchiveEntries.
	maxArchiveScanBytes = 256 << 20 // 256 MiB
)

type archiveType string

const (
	archiveTypeTar   archiveType = "tar"
	archiveTypeTarGZ archiveType = "tar.gz"
	archiveTypeZip   archiveType = "zip"
)

type archiveEntry struct {
	path    string
	size    int64
	modTime time.Time
}

func archiveManifest(path string) ([]string, error) {
	archiveType, ok := detectArchiveType(path)
	if !ok {
		return nil, errUnsupportedArchive
	}

	entries, omitted, truncated, err := readArchiveEntries(path, archiveType)
	if err != nil {
		return nil, err
	}

	return buildArchiveManifestLines(path, archiveType, entries, omitted, truncated), nil
}

func archiveErrorLines(path string, err error) []string {
	return []string{
		fmt.Sprintf("Cannot read archive: %s", filepath.Base(path)),
		fmt.Sprintf("Error: %v", err),
	}
}

var errUnsupportedArchive = errors.New("unsupported archive type")

func detectArchiveType(path string) (archiveType, bool) {
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return archiveTypeTarGZ, true
	case strings.HasSuffix(lower, ".tar"):
		return archiveTypeTar, true
	case strings.HasSuffix(lower, ".zip"):
		return archiveTypeZip, true
	default:
		return "", false
	}
}

func readArchiveEntries(path string, archiveType archiveType) ([]archiveEntry, int, bool, error) {
	switch archiveType {
	case archiveTypeZip:
		entries, omitted, err := readZipEntries(path)
		return entries, omitted, false, err
	case archiveTypeTar:
		file, err := os.Open(path)
		if err != nil {
			return nil, 0, false, err
		}
		defer file.Close()

		entries, truncated, err := readTarEntries(file)
		return entries, 0, truncated, err
	case archiveTypeTarGZ:
		file, err := os.Open(path)
		if err != nil {
			return nil, 0, false, err
		}
		defer file.Close()

		gzipReader, err := gzip.NewReader(file)
		if err != nil {
			return nil, 0, false, err
		}
		defer gzipReader.Close()

		entries, truncated, err := readTarEntries(gzipReader)
		return entries, 0, truncated, err
	default:
		return nil, 0, false, errUnsupportedArchive
	}
}

func readZipEntries(path string) ([]archiveEntry, int, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return nil, 0, err
	}
	defer reader.Close()

	entryCount := len(reader.File)
	limit := min(entryCount, maxArchiveEntries)
	entries := make([]archiveEntry, 0, limit)
	for _, file := range reader.File[:limit] {
		entries = append(entries, archiveEntry{
			path:    file.Name,
			size:    int64(file.UncompressedSize64),
			modTime: file.Modified,
		})
	}

	return entries, entryCount - limit, nil
}

func readTarEntries(reader io.Reader) ([]archiveEntry, bool, error) {
	// Bound the number of (decompressed) bytes read so a decompression bomb or
	// an archive with a huge number of entries cannot consume unbounded CPU.
	tarReader := tar.NewReader(io.LimitReader(reader, maxArchiveScanBytes))
	entries := make([]archiveEntry, 0, maxArchiveEntries)

	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			// A malformed archive, or one that exceeds the scan-byte budget
			// (e.g. a decompression bomb). Show whatever was gathered so far
			// rather than failing the whole preview, and flag it as truncated.
			if len(entries) == 0 {
				return nil, false, err
			}
			return entries, true, nil
		}

		// Include directory headers so the manifest mirrors the archive table of contents.
		if len(entries) >= maxArchiveEntries {
			// Stop at the display cap; scanning the remainder is unnecessary
			// and unbounded for archives with many entries.
			return entries, true, nil
		}
		entries = append(entries, archiveEntry{
			path:    header.Name,
			size:    header.Size,
			modTime: header.ModTime,
		})
	}

	return entries, false, nil
}

func buildArchiveManifestLines(path string, archiveType archiveType, entries []archiveEntry, omitted int, truncated bool) []string {
	lines := make([]string, 0, len(entries)+4)
	entriesLabel := fmt.Sprintf("%d", len(entries)+omitted)
	if truncated {
		// The total is unknown because scanning stopped at a safety limit.
		entriesLabel = fmt.Sprintf("%d+", len(entries))
	}
	lines = append(
		lines,
		fmt.Sprintf("Archive: %s (%s)", filepath.Base(path), archiveType),
		"Entries: "+entriesLabel,
		"",
		"Size       Modified          Path",
	)

	for _, entry := range entries {
		lines = append(lines, fmt.Sprintf("%-10s %-16s  %s", formatSize(entry.size), formatArchiveModTime(entry.modTime), entry.path))
	}
	switch {
	case truncated:
		lines = append(lines, "... additional entries omitted (archive too large to list fully)")
	case omitted > 0:
		lines = append(lines, fmt.Sprintf("... and %d more entries", omitted))
	}

	return lines
}

func formatArchiveModTime(modTime time.Time) string {
	if modTime.IsZero() {
		return "-"
	}
	return modTime.Format(archiveModTimeLayout)
}
