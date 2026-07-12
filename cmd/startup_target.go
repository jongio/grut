package cmd

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// startupTarget describes how a positional command-line argument to grut is
// interpreted: an optional directory to change into, an optional file to open
// in the preview panel, and an optional 1-based line to scroll that file to.
type startupTarget struct {
	chdir string // directory to change into before starting (empty = stay in cwd)
	file  string // absolute path of the file to preview (empty = none)
	line  int    // 1-based line to scroll the preview to (0 = none)
}

// resolveStartupTarget interprets a positional CLI argument as a directory, a
// file, or a "file:line" reference. statFn is injected so the logic can be unit
// tested without touching the filesystem; callers pass os.Stat.
//
// Resolution order:
//  1. If arg names an existing path: a directory roots grut there; a file roots
//     grut at the file's directory and selects the file.
//  2. Otherwise, if arg ends in ":<positive-int>" and the text before the last
//     colon names an existing file, that file is opened at the given line.
//  3. Otherwise the argument is left unresolved (empty target): the caller keeps
//     the current directory, matching grut's behavior before file arguments were
//     supported.
//
// Returned paths are absolute so a later os.Chdir does not invalidate them.
func resolveStartupTarget(arg string, statFn func(string) (os.FileInfo, error)) startupTarget {
	if arg == "" {
		return startupTarget{}
	}
	if t, ok := targetForExistingPath(arg, statFn); ok {
		return t
	}
	if t, ok := targetForFileLine(arg, statFn); ok {
		return t
	}
	return startupTarget{}
}

// targetForExistingPath resolves arg when it names a path that exists on disk.
func targetForExistingPath(arg string, statFn func(string) (os.FileInfo, error)) (startupTarget, bool) {
	info, err := statFn(arg)
	if err != nil {
		return startupTarget{}, false
	}
	abs, err := filepath.Abs(arg)
	if err != nil {
		return startupTarget{}, false
	}
	if info.IsDir() {
		return startupTarget{chdir: abs}, true
	}
	return startupTarget{chdir: filepath.Dir(abs), file: abs}, true
}

// targetForFileLine resolves arg of the form "file:line" where file exists and
// line is a positive integer. The last colon is used as the separator so that
// Windows drive letters (for example C:\path) are not mistaken for a suffix.
func targetForFileLine(arg string, statFn func(string) (os.FileInfo, error)) (startupTarget, bool) {
	idx := strings.LastIndex(arg, ":")
	if idx <= 0 || idx == len(arg)-1 {
		return startupTarget{}, false
	}
	line, err := strconv.Atoi(arg[idx+1:])
	if err != nil || line <= 0 {
		return startupTarget{}, false
	}
	base := arg[:idx]
	info, err := statFn(base)
	if err != nil || info.IsDir() {
		return startupTarget{}, false
	}
	abs, err := filepath.Abs(base)
	if err != nil {
		return startupTarget{}, false
	}
	return startupTarget{chdir: filepath.Dir(abs), file: abs, line: line}, true
}
