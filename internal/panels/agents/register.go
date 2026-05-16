package agents

import (
	"github.com/jongio/grut/internal/mcp"
	"github.com/jongio/grut/internal/panelreg"
	"github.com/jongio/grut/internal/panels"
)

func init() {
	panelreg.Register("agents", func(deps panelreg.Deps) panels.Panel {
		maxProcs := deps.Config.MCP.Security.MaxAgentProcesses
		timeout := deps.Config.MCP.Security.AgentTimeout
		tracker := mcp.NewAgentTracker(maxProcs, timeout)
		return deps.ApplyActionsCfg(New(tracker, deps.Theme))
	})
}
