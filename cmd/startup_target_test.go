package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// fakeFileInfo is a minimal os.FileInfo used to stub statFn in tests.
type fakeFileInfo struct {
	name  string
	isDir bool
}

func (f fakeFileInfo) Name() string { return f.name }
func (f fakeFileInfo) Size() int64  { return 0 }

func (f fakeFileInfo) Mode() os.FileMode {
	if f.isDir {
		return os.ModeDir
	}
	return 0
}

func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return f.isDir }
func (f fakeFileInfo) Sys() any           { return nil }

// statStub returns a statFn that reports the paths in existing (mapped to a
// directory flag) as present and everything else as not found.
func statStub(existing map[string]bool) func(string) (os.FileInfo, error) {
	return func(name string) (os.FileInfo, error) {
		isDir, ok := existing[name]
		if !ok {
			return nil, os.ErrNotExist
		}
		return fakeFileInfo{name: filepath.Base(name), isDir: isDir}, nil
	}
}

func TestResolveStartupTarget(t *testing.T) {
	fileRel := filepath.Join("pkg", "file.go")
	fileAbs, err := filepath.Abs(fileRel)
	assert.NoError(t, err)
	dirRel := "mydir"
	dirAbs, err := filepath.Abs(dirRel)
	assert.NoError(t, err)

	tests := []struct {
		name     string
		arg      string
		existing map[string]bool
		want     startupTarget
	}{
		{
			name:     "existing directory roots there",
			arg:      dirRel,
			existing: map[string]bool{dirRel: true},
			want:     startupTarget{chdir: dirAbs},
		},
		{
			name:     "existing file selects file at its directory",
			arg:      fileRel,
			existing: map[string]bool{fileRel: false},
			want:     startupTarget{chdir: filepath.Dir(fileAbs), file: fileAbs},
		},
		{
			name:     "file with line suffix scrolls to line",
			arg:      fileRel + ":42",
			existing: map[string]bool{fileRel: false},
			want:     startupTarget{chdir: filepath.Dir(fileAbs), file: fileAbs, line: 42},
		},
		{
			name:     "line suffix on a missing file is ignored",
			arg:      fileRel + ":42",
			existing: map[string]bool{},
			want:     startupTarget{},
		},
		{
			name:     "nonexistent path is ignored",
			arg:      "nope",
			existing: map[string]bool{},
			want:     startupTarget{},
		},
		{
			name:     "zero line is not treated as a jump",
			arg:      fileRel + ":0",
			existing: map[string]bool{fileRel: false},
			want:     startupTarget{},
		},
		{
			name:     "directory with a line suffix is ignored",
			arg:      dirRel + ":10",
			existing: map[string]bool{dirRel: true},
			want:     startupTarget{},
		},
		{
			name:     "empty arg yields empty target",
			arg:      "",
			existing: map[string]bool{},
			want:     startupTarget{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveStartupTarget(tt.arg, statStub(tt.existing))
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveStartupTargetWindowsDriveColon(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("drive-letter paths are Windows-specific")
	}
	fileAbs := `C:\repo\file.go`
	existing := map[string]bool{fileAbs: false}

	// An existing drive path resolves via the exists check, so the drive colon
	// is never treated as a line separator.
	got := resolveStartupTarget(fileAbs, statStub(existing))
	assert.Equal(t, startupTarget{chdir: `C:\repo`, file: fileAbs}, got)

	// A drive path with a trailing line suffix splits on the last colon only.
	got = resolveStartupTarget(fileAbs+":88", statStub(existing))
	assert.Equal(t, startupTarget{chdir: `C:\repo`, file: fileAbs, line: 88}, got)
}
