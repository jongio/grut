package mcp

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// maxFileListEntries caps directory listing results to prevent excessive
// memory use. The same limit is enforced in the chat tool executor
// (internal/chat/executor.go) as maxListEntries — keep both values in sync.
const maxFileListEntries = 10000

// fileEntry represents a file or directory in a listing result.
type fileEntry struct {
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
}

// newFileEntry creates a fileEntry from a directory entry, computing the
// relative path from root and resolving file size.
func newFileEntry(root, fullPath string, d fs.DirEntry) fileEntry {
	rel, relErr := filepath.Rel(root, fullPath)
	if relErr != nil {
		rel = fullPath
	}
	rel = filepath.ToSlash(rel)
	var size int64
	if info, err := d.Info(); err == nil {
		size = info.Size()
	}
	return fileEntry{
		Path:  rel,
		IsDir: d.IsDir(),
		Size:  size,
	}
}

// registerFileTools registers file read/write/list tools on the server.
// All file operations are path-jailed to the git repository root.
func registerFileTools(s *Server) {
	// file_read
	s.addTool(
		"file_read", categoryRead,
		mcplib.NewTool(
			"file_read",
			mcplib.WithDescription("Read the content of a file within the repository"),
			mcplib.WithString("path", mcplib.Required(), mcplib.Description("File path relative to the repository root")),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			path, err := req.RequireString("path")
			if err != nil {
				return mcplib.NewToolResultError("path is required"), nil //nolint:nilerr // error returned as MCP tool result
			}
			resolved, err := s.jail.Validate(path)
			if err != nil {
				return mcplib.NewToolResultErrorf("path validation: %v", err), nil
			}
			if err := IsSensitivePath(path); err != nil {
				return mcplib.NewToolResultErrorf("path blocked: %v", err), nil
			}
			if err := IsSensitivePath(resolved); err != nil {
				return mcplib.NewToolResultErrorf("path blocked (resolved): %v", err), nil
			}
			// Open the file once and stat+read from the same fd to
			// avoid TOCTOU races (CWE-367).
			const maxFileReadSize = 10 * 1024 * 1024 // 10 MiB — keep in sync with internal/chat/executor.go
			f, openErr := os.Open(resolved)
			if openErr != nil {
				return mcplib.NewToolResultErrorf("open file: %v", openErr), nil
			}
			defer f.Close()
			info, statErr := f.Stat()
			if statErr != nil {
				return mcplib.NewToolResultErrorf("stat file: %v", statErr), nil
			}
			if info.Size() > maxFileReadSize {
				return mcplib.NewToolResultErrorf("file too large: %d bytes (max %d)", info.Size(), maxFileReadSize), nil
			}
			data, err := io.ReadAll(io.LimitReader(f, maxFileReadSize))
			if err != nil {
				return mcplib.NewToolResultErrorf("read file: %v", err), nil
			}
			return mcplib.NewToolResultText(string(data)), nil
		},
	)

	// file_write
	s.addTool(
		"file_write", categoryWrite,
		mcplib.NewTool(
			"file_write",
			mcplib.WithDescription("Write content to a file within the repository"),
			mcplib.WithString("path", mcplib.Required(), mcplib.Description("File path relative to the repository root")),
			mcplib.WithString(fieldContent, mcplib.Required(), mcplib.Description("Content to write to the file")),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			path, err := req.RequireString("path")
			if err != nil {
				return mcplib.NewToolResultError("path is required"), nil //nolint:nilerr // error returned as MCP tool result
			}
			content, err := req.RequireString(fieldContent)
			if err != nil {
				return mcplib.NewToolResultError("content is required"), nil //nolint:nilerr // error returned as MCP tool result
			}
			// Enforce write size limit matching the read-side cap.
			const maxFileWriteSize = 10 * 1024 * 1024 // 10 MiB — keep in sync with internal/chat/executor.go
			if len(content) > maxFileWriteSize {
				return mcplib.NewToolResultErrorf("content too large: %d bytes (max %d)", len(content), maxFileWriteSize), nil
			}
			resolved, err := s.jail.Validate(path)
			if err != nil {
				return mcplib.NewToolResultErrorf("path validation: %v", err), nil
			}
			if err := IsSensitivePath(path); err != nil {
				return mcplib.NewToolResultErrorf("path blocked: %v", err), nil
			}
			if err := IsSensitivePath(resolved); err != nil {
				return mcplib.NewToolResultErrorf("path blocked (resolved): %v", err), nil
			}

			// Ensure parent directory exists.
			// SA-004: Validate that the parent directory is within jail
			// before creating it to prevent directory creation outside
			// the repository root.
			dir := filepath.Dir(resolved)
			if _, err := s.jail.Validate(dir); err != nil {
				return mcplib.NewToolResultErrorf("parent directory escapes jail: %v", err), nil
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return mcplib.NewToolResultErrorf("create directory: %v", err), nil
			}
			// Use open-stat-write-on-fd pattern to prevent TOCTOU races
			// (CWE-367). An attacker could swap a symlink between Validate
			// and write; by opening first, then stat'ing the fd, we ensure
			// the path checked is the same file written.
			f, openErr := os.OpenFile(resolved, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
			if openErr != nil {
				return mcplib.NewToolResultErrorf("open file for write: %v", openErr), nil
			}
			closeFile := true
			defer func() {
				if closeFile {
					f.Close()
				}
			}()
			// Re-verify the opened fd is still inside the jail by checking
			// that it is a regular file (not a symlink target outside jail).
			fi, statErr := f.Stat()
			if statErr != nil {
				return mcplib.NewToolResultErrorf("stat opened file: %v", statErr), nil
			}
			if fi.Mode()&os.ModeType != 0 {
				return mcplib.NewToolResultErrorf("refusing to write: not a regular file"), nil
			}
			if _, writeErr := f.Write([]byte(content)); writeErr != nil {
				return mcplib.NewToolResultErrorf("write file: %v", writeErr), nil
			}
			closeFile = false
			if closeErr := f.Close(); closeErr != nil {
				return mcplib.NewToolResultErrorf("close file: %v", closeErr), nil
			}
			return mcplib.NewToolResultText("file written: " + path), nil
		},
	)

	// file_list
	s.addTool(
		"file_list", categoryRead,
		mcplib.NewTool(
			"file_list",
			mcplib.WithDescription("List files and directories within the repository"),
			mcplib.WithString("path", mcplib.Description("Directory path relative to repo root (default root)")),
			mcplib.WithBoolean("recursive", mcplib.Description("List recursively")),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			path := req.GetString("path", ".")
			recursive := req.GetBool("recursive", false)

			resolved, err := s.jail.Validate(path)
			if err != nil {
				return mcplib.NewToolResultErrorf("path validation: %v", err), nil
			}

			entries := make([]fileEntry, 0)
			if recursive {
				err = filepath.WalkDir(resolved, func(p string, d fs.DirEntry, walkErr error) error {
					if walkErr != nil {
						return nil //nolint:nilerr // skip inaccessible entries
					}
					// Skip .git directory entirely.
					if d.IsDir() && d.Name() == ".git" { //nolint:goconst // inline string is more readable here
						return filepath.SkipDir
					}
					if len(entries) >= maxFileListEntries {
						return fmt.Errorf("listing capped at %d entries", maxFileListEntries)
					}
					entries = append(entries, newFileEntry(s.jail.Root(), p, d))
					return nil
				})
			} else {
				dirEntries, readErr := os.ReadDir(resolved)
				if readErr != nil {
					return mcplib.NewToolResultErrorf("list directory: %v", readErr), nil
				}
				for _, d := range dirEntries {
					// Skip .git directory.
					if d.Name() == ".git" {
						continue
					}
					entries = append(entries, newFileEntry(s.jail.Root(), filepath.Join(resolved, d.Name()), d))
				}
			}

			if err != nil {
				return mcplib.NewToolResultErrorf("walk directory: %v", err), nil
			}
			// Filter out entries starting with ".git/" for safety.
			filtered := entries[:0]
			for _, e := range entries {
				if !strings.HasPrefix(e.Path, ".git/") && e.Path != ".git" {
					filtered = append(filtered, e)
				}
			}
			return jsonResult(filtered)
		},
	)
}
