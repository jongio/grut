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

	entries, omitted, err := readArchiveEntries(path, archiveType)
	if err != nil {
		return nil, err
	}

	return buildArchiveManifestLines(path, archiveType, entries, omitted), nil
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

func readArchiveEntries(path string, archiveType archiveType) ([]archiveEntry, int, error) {
	switch archiveType {
	case archiveTypeZip:
		return readZipEntries(path)
	case archiveTypeTar:
		file, err := os.Open(path)
		if err != nil {
			return nil, 0, err
		}
		defer file.Close()

		return readTarEntries(file)
	case archiveTypeTarGZ:
		file, err := os.Open(path)
		if err != nil {
			return nil, 0, err
		}
		defer file.Close()

		gzipReader, err := gzip.NewReader(file)
		if err != nil {
			return nil, 0, err
		}
		defer gzipReader.Close()

		return readTarEntries(gzipReader)
	default:
		return nil, 0, errUnsupportedArchive
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

func readTarEntries(reader io.Reader) ([]archiveEntry, int, error) {
	tarReader := tar.NewReader(reader)
	entries := make([]archiveEntry, 0, maxArchiveEntries)
	omitted := 0

	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, 0, err
		}

		// Include directory headers so the manifest mirrors the archive table of contents.
		if len(entries) < maxArchiveEntries {
			entries = append(entries, archiveEntry{
				path:    header.Name,
				size:    header.Size,
				modTime: header.ModTime,
			})
			continue
		}
		omitted++
	}

	return entries, omitted, nil
}

func buildArchiveManifestLines(path string, archiveType archiveType, entries []archiveEntry, omitted int) []string {
	totalEntries := len(entries) + omitted
	lines := make([]string, 0, len(entries)+4)
	lines = append(
		lines,
		fmt.Sprintf("Archive: %s (%s)", filepath.Base(path), archiveType),
		fmt.Sprintf("Entries: %d", totalEntries),
		"",
		"Size       Modified          Path",
	)

	for _, entry := range entries {
		lines = append(lines, fmt.Sprintf("%-10s %-16s  %s", formatSize(entry.size), formatArchiveModTime(entry.modTime), entry.path))
	}
	if omitted > 0 {
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
