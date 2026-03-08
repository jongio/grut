package context

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ContextFile describes a single file selected for inclusion in the
// AI context window.
type ContextFile struct {
	Path    string // repo-relative path
	Content string // raw file content
	Tokens  int    // estimated token count
}

// Builder tracks a set of selected files and provides export functionality
// for AI chat workflows. All paths are validated against a repository root
// to prevent directory traversal.
type Builder struct {
	root  string         // canonical repo root (absolute)
	files []ContextFile  // ordered list of selected files
	index map[string]int // path → index into files (for O(1) lookup)
}

// NewBuilder creates a Builder anchored at the given repository root.
// The root is resolved to an absolute path; an error is returned if
// resolution fails.
func NewBuilder(root string) (*Builder, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve repo root: %w", err)
	}
	return &Builder{
		root:  filepath.Clean(absRoot),
		files: nil,
		index: make(map[string]int),
	}, nil
}

// Add reads the file at path and adds it to the context. Relative paths
// are resolved against the repo root. Paths that escape the root are
// rejected. Adding a file that is already present is a no-op.
func (b *Builder) Add(path string) error {
	resolved, relPath, err := b.resolve(path)
	if err != nil {
		return fmt.Errorf("resolving path %q: %w", path, err)
	}

	// Duplicate check.
	if _, exists := b.index[relPath]; exists {
		return nil
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	content := string(data)
	cf := ContextFile{
		Path:    relPath,
		Content: content,
		Tokens:  CountTokens(content),
	}

	b.index[relPath] = len(b.files)
	b.files = append(b.files, cf)
	return nil
}

// Remove removes a file from the context by its path. If the file is not
// present, this is a no-op.
func (b *Builder) Remove(path string) {
	_, relPath, err := b.resolve(path)
	if err != nil {
		return
	}

	idx, exists := b.index[relPath]
	if !exists {
		return
	}

	// Remove from slice, maintaining order.
	b.files = append(b.files[:idx], b.files[idx+1:]...)

	// Rebuild index.
	delete(b.index, relPath)
	for i := idx; i < len(b.files); i++ {
		b.index[b.files[i].Path] = i
	}
}

// Clear removes all files from the context.
func (b *Builder) Clear() {
	b.files = nil
	b.index = make(map[string]int)
}

// Files returns a copy of the selected files.
func (b *Builder) Files() []ContextFile {
	out := make([]ContextFile, len(b.files))
	copy(out, b.files)
	return out
}

// TotalTokens returns the sum of token counts across all selected files.
func (b *Builder) TotalTokens() int {
	total := 0
	for _, f := range b.files {
		total += f.Tokens
	}
	return total
}

// Export renders the context as structured markdown suitable for pasting
// into an AI chat window. Each file is preceded by a heading and wrapped
// in a fenced code block with the appropriate language tag.
func (b *Builder) Export() string {
	if len(b.files) == 0 {
		return ""
	}

	var sb strings.Builder
	_, _ = fmt.Fprintf(&sb, "# Context (%d files, %d tokens)\n", len(b.files), b.TotalTokens())

	for _, f := range b.files {
		_, _ = fmt.Fprintf(&sb, "\n## %s\n", f.Path)
		lang := langFromExt(filepath.Ext(f.Path))
		sb.WriteString("```")
		sb.WriteString(lang)
		sb.WriteString("\n")
		sb.WriteString(f.Content)
		// Ensure trailing newline before closing fence.
		if len(f.Content) > 0 && f.Content[len(f.Content)-1] != '\n' {
			sb.WriteString("\n")
		}
		sb.WriteString("```\n")
	}

	return sb.String()
}

// resolve validates and resolves a path against the repo root. It returns
// the absolute resolved path and the repo-relative path, or an error if
// the path escapes the root or is invalid.
func (b *Builder) resolve(path string) (abs string, rel string, err error) {
	if path == "" {
		return "", "", fmt.Errorf("path must not be empty")
	}

	// Reject explicit ".." components.
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == ".." {
			return "", "", fmt.Errorf("path escapes repository root: %s", path)
		}
	}

	var absPath string
	if filepath.IsAbs(path) {
		absPath = filepath.Clean(path)
	} else {
		absPath = filepath.Clean(filepath.Join(b.root, path))
	}

	// Verify the path is within root.
	relPath, err := filepath.Rel(b.root, absPath)
	if err != nil {
		return "", "", fmt.Errorf("compute relative path: %w", err)
	}
	if strings.HasPrefix(relPath, "..") {
		return "", "", fmt.Errorf("path escapes repository root: %s", path)
	}

	return absPath, filepath.ToSlash(relPath), nil
}

// langFromExt maps a file extension to a code-fence language tag.
func langFromExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".go":
		return "go"
	case ".js":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".jsx":
		return "jsx"
	case ".tsx":
		return "tsx"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".rb":
		return "ruby"
	case ".java":
		return "java"
	case ".c":
		return "c"
	case ".cpp", ".cc", ".cxx":
		return "cpp"
	case ".cs":
		return "csharp"
	case ".sh", ".bash":
		return "bash"
	case ".ps1":
		return "powershell"
	case ".sql":
		return "sql"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".toml":
		return "toml"
	case ".xml":
		return "xml"
	case ".html", ".htm":
		return "html"
	case ".css":
		return "css"
	case ".md":
		return "markdown"
	case ".dockerfile":
		return "dockerfile"
	default:
		return ""
	}
}
