package mcp

import (
	"slices"

	mcpserver "github.com/mark3labs/mcp-go/server"
)

type ToolInfo struct {
	Name        string `json:"name"`
	Category    string `json:"category"`
	Description string `json:"description"`
}

func ToolInventory() []ToolInfo {
	s := &Server{
		mcp: mcpserver.NewMCPServer("grut", "inventory"),
	}
	registerGitReadTools(s)
	registerGitWriteTools(s)
	registerGitOpsTools(s)
	registerFileTools(s)
	return s.Tools()
}

func (s *Server) Tools() []ToolInfo {
	out := slices.Clone(s.tools)
	return out
}
