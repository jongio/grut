package preview

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testArchiveEntryName = "docs/readme.txt"
	testArchiveContent   = "hello archive"
	testArchiveModDate   = "2026-07-22"
)

var testArchiveModTime = time.Date(2026, 7, 22, 14, 23, 0, 0, time.Local)

type testArchiveEntry struct {
	name    string
	content string
	modTime time.Time
}

func TestArchiveManifestSupportedTypes(t *testing.T) {
	tests := []struct {
		name        string
		fileName    string
		write       func(*testing.T, string, []testArchiveEntry)
		archiveType archiveType
	}{
		{
			name:        "zip",
			fileName:    "sample.zip",
			write:       writeTestZipArchive,
			archiveType: archiveTypeZip,
		},
		{
			name:        "tar",
			fileName:    "sample.tar",
			write:       writeTestTarArchive,
			archiveType: archiveTypeTar,
		},
		{
			name:        "tar_gz",
			fileName:    "sample.tar.gz",
			write:       writeTestTarGZArchive,
			archiveType: archiveTypeTarGZ,
		},
		{
			name:        "tgz",
			fileName:    "sample.tgz",
			write:       writeTestTarGZArchive,
			archiveType: archiveTypeTarGZ,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, tt.fileName)
			tt.write(t, path, []testArchiveEntry{{
				name:    testArchiveEntryName,
				content: testArchiveContent,
				modTime: testArchiveModTime,
			}})

			lines, err := archiveManifest(path)

			require.NoError(t, err)
			assert.Contains(t, lines, fmt.Sprintf("Archive: %s (%s)", tt.fileName, tt.archiveType))
			assert.Contains(t, lines, "Entries: 1")
			assert.Contains(t, lines, "Size       Modified          Path")
			entryLine, ok := lineContaining(lines, testArchiveEntryName)
			require.True(t, ok)
			assert.Contains(t, entryLine, "13 B")
			assert.Contains(t, entryLine, testArchiveModDate)
		})
	}
}

func TestArchiveManifestTruncatesEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "large.zip")
	entries := make([]testArchiveEntry, 0, maxArchiveEntries+2)
	for i := range maxArchiveEntries + 2 {
		entries = append(entries, testArchiveEntry{
			name:    fmt.Sprintf("file-%04d.txt", i),
			content: testArchiveContent,
			modTime: testArchiveModTime,
		})
	}
	writeTestZipArchive(t, path, entries)

	lines, err := archiveManifest(path)

	require.NoError(t, err)
	assert.Contains(t, lines, fmt.Sprintf("Entries: %d", maxArchiveEntries+2))
	assert.Len(t, lines, maxArchiveEntries+5)
	assert.Equal(t, "... and 2 more entries", lines[len(lines)-1])
	assert.True(t, lineContainsAll(lines, "file-0000.txt"))
	assert.False(t, lineContainsAll(lines, fmt.Sprintf("file-%04d.txt", maxArchiveEntries+1)))
}

func TestArchiveManifestCorruptArchiveReturnsClearError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupt.zip")
	require.NoError(t, os.WriteFile(path, []byte("not a zip archive"), 0o644))

	_, err := archiveManifest(path)
	lines := archiveErrorLines(path, err)

	require.Error(t, err)
	assert.Contains(t, strings.Join(lines, "\n"), "Cannot read archive: corrupt.zip")
	assert.Contains(t, strings.Join(lines, "\n"), "Error:")
}

func TestLoadFileCmdShowsArchiveManifest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "preview.zip")
	writeTestZipArchive(t, path, []testArchiveEntry{{
		name:    testArchiveEntryName,
		content: testArchiveContent,
		modTime: testArchiveModTime,
	}})

	p := New(defaultCfg(), defaultEditorCfg(), nil)
	loadFile(t, p, path)

	assert.False(t, p.isBinary)
	assert.False(t, p.isLarge)
	assert.True(t, lineContainsAll(p.lines, "Archive: preview.zip (zip)"))
	entryLine, ok := lineContaining(p.lines, testArchiveEntryName)
	require.True(t, ok)
	assert.Contains(t, entryLine, "13 B")
	assert.Contains(t, entryLine, testArchiveModDate)
}

func writeTestZipArchive(t *testing.T, path string, entries []testArchiveEntry) {
	t.Helper()
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for _, entry := range entries {
		header := &zip.FileHeader{
			Name:     entry.name,
			Method:   zip.Store,
			Modified: entry.modTime,
		}
		fileWriter, err := writer.CreateHeader(header)
		require.NoError(t, err)
		_, err = fileWriter.Write([]byte(entry.content))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0o644))
}

func writeTestTarArchive(t *testing.T, path string, entries []testArchiveEntry) {
	t.Helper()
	var buf bytes.Buffer
	writer := tar.NewWriter(&buf)
	writeTestTarEntries(t, writer, entries)
	require.NoError(t, writer.Close())
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0o644))
}

func writeTestTarGZArchive(t *testing.T, path string, entries []testArchiveEntry) {
	t.Helper()
	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzipWriter)
	writeTestTarEntries(t, tarWriter, entries)
	require.NoError(t, tarWriter.Close())
	require.NoError(t, gzipWriter.Close())
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0o644))
}

func writeTestTarEntries(t *testing.T, writer *tar.Writer, entries []testArchiveEntry) {
	t.Helper()
	for _, entry := range entries {
		content := []byte(entry.content)
		err := writer.WriteHeader(&tar.Header{
			Name:    entry.name,
			Mode:    0o644,
			Size:    int64(len(content)),
			ModTime: entry.modTime,
		})
		require.NoError(t, err)
		_, err = writer.Write(content)
		require.NoError(t, err)
	}
}

func lineContainsAll(lines []string, values ...string) bool {
	for _, line := range lines {
		matches := true
		for _, value := range values {
			if !strings.Contains(line, value) {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}

func lineContaining(lines []string, value string) (string, bool) {
	for _, line := range lines {
		if strings.Contains(line, value) {
			return line, true
		}
	}
	return "", false
}
