package filetree

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jongio/grut/internal/notify"
	"github.com/jongio/grut/internal/panels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var ansiSequenceRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func TestHumanizeSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		size int64
		want string
	}{
		{name: "zero", size: 0, want: "0 B"},
		{name: "bytes", size: 42, want: "42 B"},
		{name: "kb edge", size: bytesPerKiB - 1, want: "1023 B"},
		{name: "kb", size: bytesPerKiB, want: "1.0 KB"},
		{name: "kb decimal", size: bytesPerKiB + bytesPerKiB/2, want: "1.5 KB"},
		{name: "mb", size: bytesPerMiB, want: "1.0 MB"},
		{name: "gb", size: bytesPerGiB, want: "1.0 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, humanizeSize(tt.size))
		})
	}
}

func TestScanDirectorySizesAggregatesAndSkipsGit(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "dir", "nested"), 0o755))
	require.NoError(t, os.Mkdir(filepath.Join(root, "empty"), 0o755))
	require.NoError(t, os.Mkdir(filepath.Join(root, gitDirName), 0o755))
	writeSizedFile(t, filepath.Join(root, "a.txt"), 10)
	writeSizedFile(t, filepath.Join(root, "dir", "b.txt"), 20)
	writeSizedFile(t, filepath.Join(root, "dir", "nested", "c.txt"), 30)
	writeSizedFile(t, filepath.Join(root, gitDirName, "ignored.pack"), 2048)

	sizes, err := scanDirectorySizes(root)
	require.NoError(t, err)

	assert.Equal(t, int64(60), sizes[root])
	assert.Equal(t, int64(10), sizes[filepath.Join(root, "a.txt")])
	assert.Equal(t, int64(50), sizes[filepath.Join(root, "dir")])
	assert.Equal(t, int64(30), sizes[filepath.Join(root, "dir", "nested")])
	assert.Equal(t, int64(0), sizes[filepath.Join(root, "empty")])
	assert.NotContains(t, sizes, filepath.Join(root, gitDirName))
	assert.NotContains(t, sizes, filepath.Join(root, gitDirName, "ignored.pack"))
}

func TestScanDirectorySizesDoesNotFollowSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	targetDir := filepath.Join(root, "target")
	linkDir := filepath.Join(root, "link-target")
	require.NoError(t, os.Mkdir(targetDir, 0o755))
	writeSizedFile(t, filepath.Join(targetDir, "payload.bin"), 100)
	if err := os.Symlink(targetDir, linkDir); err != nil {
		t.Skipf("symlink creation requires privileges on this platform: %v", err)
	}
	linkInfo, err := os.Lstat(linkDir)
	require.NoError(t, err)

	sizes, err := scanDirectorySizes(root)
	require.NoError(t, err)

	assert.Equal(t, linkInfo.Size(), sizes[linkDir])
	assert.Equal(t, int64(100)+linkInfo.Size(), sizes[root])
	assert.NotContains(t, sizes, filepath.Join(linkDir, "payload.bin"))
}

func TestDirSizeScanCommandCachesVisibleSizes(t *testing.T) {
	t.Parallel()

	root := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), root)
	ft.focused = true
	ft.viewport.cursor = 0

	_, cmd := ft.handleKey(keyMsg('D'))
	require.NotNil(t, cmd)

	result, ok := cmd().(dirSizeResultMsg)
	require.True(t, ok)
	_, toastCmd := ft.Update(result)
	require.NotNil(t, toastCmd)
	toast, ok := toastCmd().(notify.ShowToastMsg)
	require.True(t, ok)

	docsPath := filepath.Join(root, "docs")
	assert.Equal(t, notify.Success, toast.Level)
	assert.Equal(t, int64(1), ft.dirSizeCache[docsPath])
	assert.Equal(t, int64(1), ft.dirSizeCache[filepath.Join(docsPath, "guide.md")])
}

func TestDirSizeCommandPaletteAction(t *testing.T) {
	t.Parallel()

	root := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), root)
	ft.focused = true
	ft.viewport.cursor = 0

	_, cmd := ft.Update(panels.CommandSelectedMsg{Action: actionDirSize})

	require.NotNil(t, cmd)
	_, ok := cmd().(dirSizeResultMsg)
	assert.True(t, ok)
}

func TestDirSizeCacheClearedOnRefresh(t *testing.T) {
	t.Parallel()

	root := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), root)
	ft.dirSizeCache = map[string]int64{filepath.Join(root, "docs"): 1}

	ft.reloadTree()

	assert.Nil(t, ft.dirSizeCache)
}

func TestRenderLineShowsSizeWithGitStatusMarker(t *testing.T) {
	t.Parallel()

	root := createTestTree(t)
	ft := newTestFT(t, defaultCfg(), root)
	ft.focused = true
	ft.SetGitClient(&mockGitClient{})
	mainGoPath := filepath.Join(root, "main.go")
	ft.gitFileStatus = map[string]string{mainGoPath: "M"}
	ft.dirSizeCache = map[string]int64{mainGoPath: bytesPerKiB}

	var mainNode *node
	for _, visibleNode := range ft.visible {
		if visibleNode.path == mainGoPath {
			mainNode = visibleNode
			break
		}
	}
	require.NotNil(t, mainNode)

	line := stripANSI(ft.renderLine(mainNode, 32, false))

	assert.Contains(t, line, "main.go")
	assert.Contains(t, line, "1.0 KB")
	assert.True(t, strings.HasSuffix(line, " M"), "git marker should remain right-aligned: %q", line)
}

func writeSizedFile(t *testing.T, path string, size int) {
	t.Helper()

	require.NoError(t, os.WriteFile(path, []byte(strings.Repeat("x", size)), 0o644))
}

func stripANSI(s string) string {
	return ansiSequenceRE.ReplaceAllString(s, "")
}
