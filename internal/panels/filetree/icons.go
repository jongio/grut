// Package filetree provides the file tree panel for grut.
package filetree

import (
	"path/filepath"
	"strings"
)

// Nerd font icons for expand/collapse and file types.
const (
	nfCaretRight = "\uf0da" //
	nfCaretDown  = "\uf0d7" //
	nfFolder     = "\uf07b" //
	nfFolderOpen = "\uf07c" //
	nfFile       = "\uf15b" //
	nfGo         = "\ue627" //
	nfJS         = "\ue74e" //
	nfTS         = "\ue628" //
	nfPython     = "\ue73c" //
	nfMarkdown   = "\ue73e" //
	nfJSON       = "\ue60b" //
	nfHTML       = "\ue736" //
	nfCSS        = "\ue749" //
	nfRust       = "\ue7a8" //
	nfRuby       = "\ue739" //
	nfJava       = "\ue738" //
	nfC          = "\ue61e" //
	nfCPP        = "\ue61d" //
	nfShell      = "\ue795" //
	nfDocker     = "\ue7b0" //
	nfGit        = "\uf1d3" //
	nfConfig     = "\ue615" //

	// Git status indicators (nerd font).
	nfGitModified  = "\uf111" //  circle (modified)
	nfGitAdded     = "\uf055" //  plus-circle
	nfGitDeleted   = "\uf056" //  minus-circle
	nfGitUntracked = "\uf128" //  question
	nfGitRenamed   = "\uf064" //  arrow-right
	nfGitConflict  = "\uf071" //  warning
	nfGitIgnored   = "\uf070" //  eye-slash (ignored)
)

// ASCII expand/collapse indicators.
const (
	asciiCollapsed = "▸"
	asciiExpanded  = "▾"
)

// getExpandIcon returns the expand/collapse indicator for a directory.
func getExpandIcon(expanded bool, mode string) string {
	if mode == "nerd" { //nolint:goconst // inline mode string
		if expanded {
			return nfCaretDown
		}
		return nfCaretRight
	}
	// "ascii" or "auto" → use ASCII fallback.
	if expanded {
		return asciiExpanded
	}
	return asciiCollapsed
}

// getFileIcon returns an icon string for the given file/directory.
// Returns "" when mode is "ascii" (no file icons in ASCII mode) or when
// icons are not enabled.
func getFileIcon(name string, isDir, expanded bool, mode string) string {
	if mode != "nerd" {
		return "" // ASCII mode: no file icons
	}

	if isDir {
		if expanded {
			return nfFolderOpen
		}
		return nfFolder
	}

	// Match by extension first.
	ext := strings.ToLower(filepath.Ext(name))
	if icon, ok := extIcons[ext]; ok {
		return icon
	}

	// Match by full filename.
	lower := strings.ToLower(name)
	if icon, ok := nameIcons[lower]; ok {
		return icon
	}

	return nfFile
}

// extIcons maps file extensions to nerd font icons.
var extIcons = map[string]string{
	".go":         nfGo,
	".js":         nfJS,
	".mjs":        nfJS,
	".cjs":        nfJS,
	".jsx":        nfJS,
	".ts":         nfTS,
	".tsx":        nfTS,
	".py":         nfPython,
	".md":         nfMarkdown,
	".markdown":   nfMarkdown,
	".json":       nfJSON,
	".html":       nfHTML,
	".htm":        nfHTML,
	".css":        nfCSS,
	".scss":       nfCSS,
	".less":       nfCSS,
	".rs":         nfRust,
	".rb":         nfRuby,
	".java":       nfJava,
	".c":          nfC,
	".h":          nfC,
	".cpp":        nfCPP,
	".cc":         nfCPP,
	".cxx":        nfCPP,
	".hpp":        nfCPP,
	".sh":         nfShell,
	".bash":       nfShell,
	".zsh":        nfShell,
	".fish":       nfShell,
	".ps1":        nfShell,
	".yaml":       nfConfig,
	".yml":        nfConfig,
	".toml":       nfConfig,
	".ini":        nfConfig,
	".cfg":        nfConfig,
	".conf":       nfConfig,
	".dockerfile": nfDocker,
}

// nameIcons maps specific filenames to nerd font icons.
var nameIcons = map[string]string{
	"dockerfile":  nfDocker,
	"makefile":    nfConfig,
	".gitignore":  nfGit,
	".gitmodules": nfGit,
	".gitconfig":  nfGit,
}

// gitStatusIcon returns a nerd font icon for the given git status letter.
// Falls back to the raw letter in ASCII mode.
func gitStatusIcon(status, mode string) string {
	if mode == "nerd" {
		switch status {
		case "M":
			return nfGitModified
		case "A":
			return nfGitAdded
		case "D":
			return nfGitDeleted
		case "?":
			return nfGitUntracked
		case "R":
			return nfGitRenamed
		case "C":
			return nfGitRenamed
		case "U":
			return nfGitConflict
		case "!":
			return nfGitIgnored
		}
	}
	return status
}
