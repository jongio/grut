// Package gitlog implements the git log panel with commit graph rendering.
package gitlog

import (
	"github.com/jongio/grut/internal/git"
)

// GraphEntry holds the rendered graph prefix for one commit.
type GraphEntry struct {
	Prefix     string   // graph prefix for the commit line, e.g. "* " or "| * "
	Connectors []string // connector lines drawn after this commit, e.g. ["|\\", "|/"]
}

// GraphRenderer produces ASCII commit graph prefixes by tracking active
// lanes. Commits must be fed in topological order (newest first).
type GraphRenderer struct {
	cols []string // active parent hashes, one per lane
}

// NewGraphRenderer creates a new renderer with empty lane state.
func NewGraphRenderer() *GraphRenderer {
	return &GraphRenderer{}
}

// RenderCommit processes one commit and returns its graph entry.
func (g *GraphRenderer) RenderCommit(c git.Commit) GraphEntry {
	col := g.indexOf(c.Hash)
	if col == -1 {
		col = g.alloc(c.Hash)
	}

	prefix := g.commitLine(col)
	connectors := g.updateParents(col, c.Parents)

	return GraphEntry{Prefix: prefix, Connectors: connectors}
}

// indexOf returns the lane index for the given hash, or -1 if not found.
func (g *GraphRenderer) indexOf(hash string) int {
	for i, h := range g.cols {
		if h == hash {
			return i
		}
	}
	return -1
}

// alloc places a hash in the first empty lane or appends a new one.
func (g *GraphRenderer) alloc(hash string) int {
	for i, h := range g.cols {
		if h == "" {
			g.cols[i] = hash
			return i
		}
	}
	g.cols = append(g.cols, hash)
	return len(g.cols) - 1
}

// commitLine builds the graph prefix for a commit at the given column.
// Each lane occupies two character positions: the lane char and a space
// separator. Example: "| * |" for a commit in column 1 with active
// columns 0 and 2.
func (g *GraphRenderer) commitLine(commitCol int) string {
	n := len(g.cols)
	if n == 0 {
		return "*"
	}

	buf := make([]byte, 0, n*2)
	for i := 0; i < n; i++ {
		if i > 0 {
			buf = append(buf, ' ')
		}
		if i == commitCol {
			buf = append(buf, '*')
		} else if g.cols[i] != "" {
			buf = append(buf, '|')
		} else {
			buf = append(buf, ' ')
		}
	}
	return string(buf)
}

// updateParents replaces the commit's lane with its first parent and
// allocates new lanes for additional parents (merge commits). Returns
// connector lines for branches and convergences.
func (g *GraphRenderer) updateParents(col int, parents []string) []string {
	var connectors []string

	if len(parents) == 0 {
		// Root commit — close the lane.
		g.cols[col] = ""
		g.trim()
		return nil
	}

	// First parent takes the same lane.
	g.cols[col] = parents[0]

	// Additional parents create new lanes (merge commit).
	for _, p := range parents[1:] {
		if g.indexOf(p) != -1 {
			continue // parent already has a lane
		}
		nc := g.alloc(p)
		connectors = append(connectors, g.connectorLine(col, nc, '\\'))
	}

	// Merge converging lanes: two columns tracking the same parent hash.
	for i := len(g.cols) - 1; i >= 1; i-- {
		if g.cols[i] == "" {
			continue
		}
		for j := 0; j < i; j++ {
			if g.cols[j] == g.cols[i] {
				connectors = append(connectors, g.connectorLine(j, i, '/'))
				g.cols[i] = ""
				break
			}
		}
	}

	g.trim()
	return connectors
}

// connectorLine draws a connector between two lanes. The connector
// character (typically '\' for branch-out or '/' for merge-in) is
// placed in the gap between the two columns for adjacent lanes, or
// at the right column's position for non-adjacent lanes.
func (g *GraphRenderer) connectorLine(leftCol, rightCol int, ch byte) string {
	n := len(g.cols)
	if n == 0 {
		return ""
	}

	// Total width: each column is 2 chars (char + gap), minus trailing gap.
	width := n*2 - 1
	if width < 1 {
		width = 1
	}

	buf := make([]byte, width)
	for i := range buf {
		buf[i] = ' '
	}

	// Fill active lane positions (even indices).
	for i, h := range g.cols {
		pos := i * 2
		if pos >= width {
			break
		}
		if h != "" && i != rightCol {
			buf[pos] = '|'
		}
	}

	// Place the connector character.
	if rightCol == leftCol+1 {
		// Adjacent: connector goes in the gap (odd position).
		pos := leftCol*2 + 1
		if pos < width {
			buf[pos] = ch
		}
	} else if rightCol > leftCol+1 {
		// Non-adjacent: connector goes at the right column position.
		pos := rightCol * 2
		if pos < width {
			buf[pos] = ch
		}
	}

	// Trim trailing spaces.
	end := width
	for end > 0 && buf[end-1] == ' ' {
		end--
	}
	return string(buf[:end])
}

// trim removes trailing empty lanes to keep the graph compact.
func (g *GraphRenderer) trim() {
	for len(g.cols) > 0 && g.cols[len(g.cols)-1] == "" {
		g.cols = g.cols[:len(g.cols)-1]
	}
}
