package filetree

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jongio/grut/internal/notify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newLazyTestFT(t *testing.T, dir string) *FileTree {
	t.Helper()
	ft := New(defaultCfg(), dir, nil)
	ft.ctx = context.Background()
	loadChildrenStatic(ft.root, ft.cfg)
	ft.rebuildVisible()
	ft.Focus()
	return ft
}

func syntheticLoadedRoot(request treeLoadRequest, childName string) *node {
	return &node{
		name:   request.name,
		path:   request.path,
		depth:  request.depth,
		isDir:  true,
		loaded: true,
		children: []*node{{
			name:  childName,
			path:  filepath.Join(request.path, childName),
			depth: request.depth + 1,
		}},
	}
}

func findToastMessage(messages []tea.Msg) (notify.ShowToastMsg, bool) {
	for _, message := range messages {
		if toast, ok := message.(notify.ShowToastMsg); ok {
			return toast, true
		}
	}
	return notify.ShowToastMsg{}, false
}

func TestTreeLoadRejectsStaleGeneration(t *testing.T) {
	ft := newLazyTestFT(t, createTestTree(t))
	ft.treeBuilder = func(_ context.Context, request treeLoadRequest) (*node, error) {
		return syntheticLoadedRoot(request, fmt.Sprintf("generation-%d", request.generation)), nil
	}

	first := ft.requestTreeReload(treeLoadRefresh)
	second := ft.requestTreeReload(treeLoadRefresh)
	require.NotNil(t, first)
	require.NotNil(t, second)

	secondResult, ok := second().(treeLoadedMsg)
	require.True(t, ok)
	ft.Update(secondResult)
	require.Equal(t, "generation-2", ft.visibleName(0))

	firstResult, ok := first().(treeLoadedMsg)
	require.True(t, ok)
	ft.Update(firstResult)
	assert.Equal(t, "generation-2", ft.visibleName(0))
}

func TestRefreshCancelsOverlappingExpansion(t *testing.T) {
	dir := createTestTree(t)
	ft := newLazyTestFT(t, dir)
	started := make(chan struct{})
	canceled := make(chan struct{})
	ft.treeBuilder = func(ctx context.Context, request treeLoadRequest) (*node, error) {
		if request.purpose == treeLoadExpand {
			close(started)
			<-ctx.Done()
			close(canceled)
			return nil, ctx.Err()
		}
		return buildDetachedTree(ctx, request)
	}

	_, expandCmd := ft.Update(specialKeyMsg(tea.KeyEnter))
	require.NotNil(t, expandCmd)
	resultCh := make(chan tea.Msg, 1)
	go func() {
		resultCh <- expandCmd()
	}()
	<-started

	require.NoError(t, os.WriteFile(filepath.Join(dir, "external.txt"), []byte("x"), 0o644))
	_, refreshCmd := ft.Update(RefreshMsg{})
	require.NotNil(t, refreshCmd)
	<-canceled
	applyFileTreeCmd(t, ft, refreshCmd)

	staleResult := <-resultCh
	ft.Update(staleResult)

	assert.NotNil(t, findNodeByPath(ft.root, filepath.Join(dir, "external.txt")))
	docs := findNodeByPath(ft.root, filepath.Join(dir, "docs"))
	require.NotNil(t, docs)
	assert.False(t, docs.loading)
}

func TestDetachedExpansionAndRefreshPreserveState(t *testing.T) {
	dir := createTestTree(t)
	ft := newLazyTestFT(t, dir)

	_, expandCmd := ft.Update(specialKeyMsg(tea.KeyEnter))
	require.NotNil(t, expandCmd)
	assert.Equal(t, 4, ft.visibleCount(), "expansion must not read the directory in Update")
	applyFileTreeCmd(t, ft, expandCmd)
	assert.Equal(t, "guide.md", ft.visibleName(1))

	srcPath := filepath.Join(dir, "src")
	mainPath := filepath.Join(dir, "main.go")
	ft.selected[mainPath] = true
	ft.filter.gitFilter = true
	ft.restoreCursorToPath(srcPath)
	require.Equal(t, srcPath, ft.CursorPath())

	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs", "new.md"), []byte("new"), 0o644))
	_, refreshCmd := ft.Update(RefreshMsg{})
	applyFileTreeCmd(t, ft, refreshCmd)

	docs := findNodeByPath(ft.root, filepath.Join(dir, "docs"))
	require.NotNil(t, docs)
	assert.True(t, docs.expanded)
	assert.NotNil(t, findNodeByPath(ft.root, filepath.Join(dir, "docs", "new.md")))
	assert.Equal(t, srcPath, ft.CursorPath())
	assert.True(t, ft.selected[mainPath])
	assert.True(t, ft.filter.gitFilter)
}

func TestExpandAllLoadsRecursivelyOutsideUpdate(t *testing.T) {
	ft := newLazyTestFT(t, createTestTree(t))
	ft.Focus()

	_, cmd := ft.Update(keyMsg('L'))
	require.NotNil(t, cmd)
	assert.Equal(t, 4, ft.visibleCount())

	applyFileTreeCmd(t, ft, cmd)
	assert.Equal(t, 7, ft.visibleCount())
	for _, child := range ft.root.children {
		if child.isDir {
			assert.True(t, child.loaded)
			assert.True(t, child.expanded)
		}
	}
}

func TestExactExpandedTreeLoadUsesLoadedAndDetachedPaths(t *testing.T) {
	dir := createTestTree(t)
	ft := newLazyTestFT(t, dir)
	docsPath := filepath.Join(dir, "docs")
	srcPath := filepath.Join(dir, "src")

	_, docsCmd := ft.Update(specialKeyMsg(tea.KeyEnter))
	applyFileTreeCmd(t, ft, docsCmd)
	require.True(t, findNodeByPath(ft.root, docsPath).loaded)
	require.False(t, findNodeByPath(ft.root, srcPath).loaded)

	fastCmd := ft.requestExactExpandedTreeLoad(
		treeLoadModeRestore,
		map[string]bool{docsPath: true},
		docsPath,
	)
	assert.Equal(t, docsPath, ft.CursorPath())
	assert.True(t, findNodeByPath(ft.root, docsPath).expanded)
	assert.False(t, findNodeByPath(ft.root, srcPath).expanded)
	applyFileTreeCmd(t, ft, fastCmd)

	detachedCmd := ft.requestExactExpandedTreeLoad(
		treeLoadModeRestore,
		map[string]bool{srcPath: true},
		srcPath,
	)
	require.NotNil(t, detachedCmd)
	applyFileTreeCmd(t, ft, detachedCmd)
	assert.True(t, findNodeByPath(ft.root, srcPath).expanded)
	assert.Equal(t, srcPath, ft.CursorPath())
}

func TestBuildDetachedTreeHandlesCancellationErrorsAndDepth(t *testing.T) {
	t.Run("canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		request := treeLoadRequest{
			config:   snapshotTreeLoadConfig(defaultCfg()),
			name:     "root",
			path:     t.TempDir(),
			rootPath: t.TempDir(),
			depth:    -1,
		}
		root, err := buildDetachedTree(ctx, request)
		require.ErrorIs(t, err, context.Canceled)
		assert.False(t, root.loaded)
	})

	t.Run("missing directory", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "missing")
		request := treeLoadRequest{
			config:   snapshotTreeLoadConfig(defaultCfg()),
			name:     "missing",
			path:     missing,
			rootPath: filepath.Dir(missing),
			depth:    -1,
		}
		root, err := buildDetachedTree(context.Background(), request)
		require.Error(t, err)
		assert.True(t, root.loaded)
		assert.Error(t, root.loadErr)
	})

	t.Run("maximum depth", func(t *testing.T) {
		dir := createTestTree(t)
		cfg := snapshotTreeLoadConfig(defaultCfg())
		cfg.maxDepth = 0
		request := treeLoadRequest{
			config:   cfg,
			name:     filepath.Base(dir),
			path:     dir,
			rootPath: dir,
			depth:    -1,
		}
		root, err := buildDetachedTree(context.Background(), request)
		require.NoError(t, err)
		assert.True(t, root.loaded)
		assert.Empty(t, root.children)
	})
}

func TestBuildDetachedTreeEnforcesSymlinkPolicy(t *testing.T) {
	rootDir := t.TempDir()
	childDir := filepath.Join(rootDir, "child")
	require.NoError(t, os.Mkdir(childDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(childDir, "inside.txt"), []byte("x"), 0o644))

	disabledConfig := snapshotTreeLoadConfig(defaultCfg())
	disabledRequest := treeLoadRequest{
		config:    disabledConfig,
		name:      "child",
		path:      childDir,
		rootPath:  rootDir,
		depth:     0,
		isSymlink: true,
	}
	disabledRoot, err := buildDetachedTree(context.Background(), disabledRequest)
	require.Error(t, err)
	assert.True(t, disabledRoot.loaded)

	enabledRequest := disabledRequest
	enabledRequest.config.followSymlinks = true
	enabledRoot, err := buildDetachedTree(context.Background(), enabledRequest)
	require.NoError(t, err)
	assert.Len(t, enabledRoot.children, 1)
	assert.True(t, safeSymlinkTarget(rootDir, childDir))

	outsideRequest := enabledRequest
	outsideRequest.path = t.TempDir()
	outsideRequest.name = filepath.Base(outsideRequest.path)
	outsideRoot, err := buildDetachedTree(context.Background(), outsideRequest)
	require.Error(t, err)
	assert.True(t, outsideRoot.loaded)
	assert.False(t, safeSymlinkTarget(rootDir, outsideRequest.path))
}

func TestPopulateSymlinkNodeUsesTargetMetadata(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "file.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("x"), 0o755))
	fileNode := &node{path: filePath}
	populateSymlinkNode(fileNode)
	assert.True(t, fileNode.isSymlink)
	assert.False(t, fileNode.isDir)

	dirNode := &node{path: dir}
	populateSymlinkNode(dirNode)
	assert.True(t, dirNode.isSymlink)
	assert.True(t, dirNode.isDir)
}

type failingDirEntry struct{}

func (failingDirEntry) Name() string               { return "broken" }
func (failingDirEntry) IsDir() bool                { return false }
func (failingDirEntry) Type() fs.FileMode          { return 0 }
func (failingDirEntry) Info() (fs.FileInfo, error) { return nil, errors.New("metadata failed") }

type symlinkDirEntry struct {
	name string
}

func (entry symlinkDirEntry) Name() string         { return entry.name }
func (symlinkDirEntry) IsDir() bool                { return false }
func (symlinkDirEntry) Type() fs.FileMode          { return os.ModeSymlink }
func (symlinkDirEntry) Info() (fs.FileInfo, error) { return nil, errors.New("unexpected info call") }

func TestNodeFromDirEntryReturnsMetadataError(t *testing.T) {
	parent := &node{path: t.TempDir(), depth: -1}
	child, err := nodeFromDirEntry(parent, failingDirEntry{})
	assert.Nil(t, child)
	assert.EqualError(t, err, "metadata failed")
}

func TestNodeFromDirEntryUsesTypeMetadataForSymlink(t *testing.T) {
	parent := &node{path: t.TempDir(), depth: -1}
	require.NoError(t, os.Mkdir(filepath.Join(parent.path, "link"), 0o755))
	child, err := nodeFromDirEntry(parent, symlinkDirEntry{name: "link"})
	require.NoError(t, err)
	require.NotNil(t, child)
	assert.True(t, child.isSymlink)
	assert.True(t, child.isDir)
}

func TestLoadDetachedChildrenSurfacesMetadataErrors(t *testing.T) {
	parent := &node{path: t.TempDir(), depth: -1, isDir: true}
	config := snapshotTreeLoadConfig(defaultCfg())
	config.readDir = func(string) ([]os.DirEntry, error) {
		return []os.DirEntry{failingDirEntry{}}, nil
	}
	err := loadDetachedChildren(context.Background(), parent, treeLoadRequest{config: config})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "metadata failed")
	assert.Equal(t, err.Error(), parent.loadErr.Error())
	assert.True(t, parent.loaded)
}

func TestCollapsedDirectoryDoesNotEmitLateDirChanged(t *testing.T) {
	ft := newLazyTestFT(t, createTestTree(t))
	_, expandCmd := ft.Update(specialKeyMsg(tea.KeyEnter))
	require.NotNil(t, expandCmd)
	ft.Update(keyMsg('h'))

	messages := applyFileTreeCmd(t, ft, expandCmd)
	for _, message := range messages {
		_, isDirChanged := message.(DirChangedMsg)
		assert.False(t, isDirChanged)
	}
	docs := findNodeByPath(ft.root, filepath.Join(ft.rootPath, "docs"))
	require.NotNil(t, docs)
	assert.True(t, docs.loaded)
	assert.False(t, docs.expanded)
}

func TestTreeLoadErrorsUseConsistentToast(t *testing.T) {
	t.Run("directory expansion", func(t *testing.T) {
		ft := newLazyTestFT(t, createTestTree(t))
		loadErr := errors.New("read failed")
		ft.treeBuilder = func(_ context.Context, request treeLoadRequest) (*node, error) {
			root := &node{
				name:      request.name,
				path:      request.path,
				depth:     request.depth,
				isDir:     true,
				loaded:    true,
				loadErr:   loadErr,
				isSymlink: request.isSymlink,
			}
			return root, loadErr
		}

		_, cmd := ft.Update(specialKeyMsg(tea.KeyEnter))
		messages := applyFileTreeCmd(t, ft, cmd)
		toast, ok := findToastMessage(messages)
		require.True(t, ok)
		assert.Equal(t, notify.Error, toast.Level)
		assert.Contains(t, toast.Message, "Cannot load docs: read failed")

		docs := findNodeByPath(ft.root, filepath.Join(ft.rootPath, "docs"))
		require.NotNil(t, docs)
		assert.ErrorIs(t, docs.loadErr, loadErr)
	})

	t.Run("refresh", func(t *testing.T) {
		ft := newLazyTestFT(t, createTestTree(t))
		loadErr := errors.New("refresh failed")
		ft.treeBuilder = func(_ context.Context, request treeLoadRequest) (*node, error) {
			root := syntheticLoadedRoot(request, "partial.txt")
			root.loadErr = loadErr
			return root, loadErr
		}

		_, cmd := ft.Update(RefreshMsg{})
		messages := applyFileTreeCmd(t, ft, cmd)
		toast, ok := findToastMessage(messages)
		require.True(t, ok)
		assert.Equal(t, notify.Error, toast.Level)
		assert.Contains(t, toast.Message, "Cannot load")
		assert.Contains(t, toast.Message, "refresh failed")
		assert.Equal(t, "partial.txt", ft.visibleName(0))
	})
}

func TestConcurrentChildLoadCommandsAreRaceSafe(t *testing.T) {
	dir := t.TempDir()
	const dirCount = 16
	for i := range dirCount {
		subdir := filepath.Join(dir, fmt.Sprintf("dir-%02d", i))
		require.NoError(t, os.Mkdir(subdir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(subdir, "file.txt"), []byte("x"), 0o644))
	}
	ft := newLazyTestFT(t, dir)

	cmds := make([]tea.Cmd, 0, dirCount)
	for _, child := range ft.root.children {
		require.True(t, child.isDir)
		child.expanded = true
		cmd := ft.requestChildLoad(child)
		require.NotNil(t, cmd)
		cmds = append(cmds, cmd)
	}

	results := make(chan tea.Msg, len(cmds))
	var wg sync.WaitGroup
	wg.Add(len(cmds))
	for _, cmd := range cmds {
		go func(load tea.Cmd) {
			defer wg.Done()
			results <- load()
		}(cmd)
	}
	wg.Wait()
	close(results)

	for result := range results {
		ft.Update(result)
	}
	for _, child := range ft.root.children {
		assert.True(t, child.loaded)
		assert.Len(t, child.children, 1)
	}
}
