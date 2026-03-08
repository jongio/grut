// Package layout implements the layout engine for grut's TUI.
// It manages a binary split tree where each leaf holds a panel name,
// and computes absolute sizes from terminal dimensions.
package layout

// Direction represents the orientation of a split.
type Direction int

const (
	// Horizontal splits the space into left and right children.
	Horizontal Direction = iota
	// Vertical splits the space into top and bottom children.
	Vertical
)

// String returns the human-readable direction name.
func (d Direction) String() string {
	switch d {
	case Horizontal:
		return "horizontal"
	case Vertical:
		return "vertical"
	default:
		return "unknown"
	}
}

// Rect represents a rectangular area with position and size.
type Rect struct {
	X      int
	Y      int
	Width  int
	Height int
}

// Node is the interface for layout tree nodes. A node is either a SplitNode
// (with two children) or a LeafNode (holding a panel name).
type Node interface {
	// PanelNames returns all panel names contained in this subtree.
	PanelNames() []string

	// Clone returns a deep copy of the node.
	Clone() Node

	// isNode is a sealed interface marker.
	isNode()
}

// SplitNode divides space between two children according to a direction
// and ratio. Ratio is the fraction of space allocated to the first (left/top)
// child, in the range (0, 1).
type SplitNode struct {
	Direction Direction
	Ratio     float64
	First     Node // left or top child
	Second    Node // right or bottom child
}

// PanelNames implements Node.
func (s *SplitNode) PanelNames() []string {
	names := s.First.PanelNames()
	names = append(names, s.Second.PanelNames()...)
	return names
}

// Clone implements Node.
func (s *SplitNode) Clone() Node {
	return &SplitNode{
		Direction: s.Direction,
		Ratio:     s.Ratio,
		First:     s.First.Clone(),
		Second:    s.Second.Clone(),
	}
}

func (*SplitNode) isNode() {}

// LeafNode is a terminal node in the layout tree, holding a panel name.
type LeafNode struct {
	Panel string
}

// PanelNames implements Node.
func (l *LeafNode) PanelNames() []string {
	return []string{l.Panel}
}

// Clone implements Node.
func (l *LeafNode) Clone() Node {
	return &LeafNode{Panel: l.Panel}
}

func (*LeafNode) isNode() {}

// FirstPanelOf returns the name of the first (top-left-most) panel
// in the subtree rooted at node.
func FirstPanelOf(node Node) string {
	switch n := node.(type) {
	case *LeafNode:
		return n.Panel
	case *SplitNode:
		return FirstPanelOf(n.First)
	}
	return ""
}

// Resolve walks the layout tree and computes the absolute Rect for each
// leaf panel given the available area. Returns a map of panel name → Rect.
func Resolve(node Node, area Rect) map[string]Rect {
	result := make(map[string]Rect)
	resolve(node, area, result)
	return result
}

func resolve(node Node, area Rect, result map[string]Rect) {
	switch n := node.(type) {
	case *LeafNode:
		result[n.Panel] = area
	case *SplitNode:
		first, second := SplitRect(area, n.Direction, n.Ratio)
		resolve(n.First, first, result)
		resolve(n.Second, second, result)
	}
}

// SplitRect divides a Rect into two sub-rects based on direction and ratio.
// One column (horizontal) or row (vertical) is reserved between the two
// sub-rects for the separator line that visually divides panels.
func SplitRect(area Rect, dir Direction, ratio float64) (Rect, Rect) {
	switch dir {
	case Horizontal:
		// Reserve 1 column for the vertical separator between panels.
		usable := area.Width - 1
		if usable < 2 {
			usable = 2
		}
		firstW := int(float64(usable) * ratio)
		if firstW < 1 {
			firstW = 1
		}
		secondW := usable - firstW
		if secondW < 1 {
			secondW = 1
			firstW = usable - 1
		}
		first := Rect{X: area.X, Y: area.Y, Width: firstW, Height: area.Height}
		second := Rect{X: area.X + firstW + 1, Y: area.Y, Width: secondW, Height: area.Height}
		return first, second
	case Vertical:
		// Reserve 1 row for the horizontal separator between panels.
		usable := area.Height - 1
		if usable < 2 {
			usable = 2
		}
		firstH := int(float64(usable) * ratio)
		if firstH < 1 {
			firstH = 1
		}
		secondH := usable - firstH
		if secondH < 1 {
			secondH = 1
			firstH = usable - 1
		}
		first := Rect{X: area.X, Y: area.Y, Width: area.Width, Height: firstH}
		second := Rect{X: area.X, Y: area.Y + firstH + 1, Width: area.Width, Height: secondH}
		return first, second
	default:
		return area, Rect{}
	}
}

// SplitLeaf replaces the leaf named targetPanel with a new SplitNode
// containing the original leaf as the first child and a new leaf (newPanel)
// as the second child, using the given direction and a 0.5 ratio.
// Returns the (possibly new) root node. The caller must assign the result
// back to their tree reference:
//
//	tab.Tree = SplitLeaf(tab.Tree, "preview", Vertical, "terminal")
func SplitLeaf(root Node, targetPanel string, dir Direction, newPanel string) Node {
	switch n := root.(type) {
	case *LeafNode:
		if n.Panel == targetPanel {
			return &SplitNode{
				Direction: dir,
				Ratio:     0.5,
				First:     n,
				Second:    &LeafNode{Panel: newPanel},
			}
		}
		return root
	case *SplitNode:
		result := SplitLeaf(n.First, targetPanel, dir, newPanel)
		if result != n.First {
			n.First = result
			return root
		}
		result = SplitLeaf(n.Second, targetPanel, dir, newPanel)
		if result != n.Second {
			n.Second = result
		}
		return root
	}
	return root
}

// RemoveLeaf removes the leaf named targetPanel from the tree, collapsing
// its parent SplitNode so the sibling takes the parent's place. Returns
// the new root and whether the panel was found.
//
//	newTree, ok := RemoveLeaf(tab.Tree, "terminal")
func RemoveLeaf(root Node, targetPanel string) (Node, bool) {
	switch n := root.(type) {
	case *LeafNode:
		if n.Panel == targetPanel {
			return nil, true
		}
		return root, false
	case *SplitNode:
		newFirst, foundFirst := RemoveLeaf(n.First, targetPanel)
		if foundFirst {
			if newFirst == nil {
				// Target was the direct first child; collapse to sibling.
				return n.Second, true
			}
			n.First = newFirst
			return root, true
		}
		newSecond, foundSecond := RemoveLeaf(n.Second, targetPanel)
		if foundSecond {
			if newSecond == nil {
				// Target was the direct second child; collapse to sibling.
				return n.First, true
			}
			n.Second = newSecond
			return root, true
		}
		return root, false
	}
	return root, false
}

// FindSplitContaining finds the innermost SplitNode whose First or Second
// child directly contains the given panel name. Returns the split node and
// which child ("first" or "second") contains it, or nil if not found.
// For nested trees, this returns the deepest split that directly parents
// the leaf — the one whose ratio directly affects the panel's size.
func FindSplitContaining(root Node, panelName string) (*SplitNode, string) {
	split, ok := root.(*SplitNode)
	if !ok {
		return nil, ""
	}

	// Recurse into children first to find deeper (more specific) matches.
	if s, side := FindSplitContaining(split.First, panelName); s != nil {
		return s, side
	}
	if s, side := FindSplitContaining(split.Second, panelName); s != nil {
		return s, side
	}

	// Check current level
	for _, name := range split.First.PanelNames() {
		if name == panelName {
			return split, "first"
		}
	}
	for _, name := range split.Second.PanelNames() {
		if name == panelName {
			return split, "second"
		}
	}

	return nil, ""
}

// FindSplitAtBorder walks the layout tree and returns the SplitNode whose
// border is at or near the given (x, y) coordinates within the specified
// hit zone tolerance. Coordinates are relative to the panel area (not
// terminal). Inner (deeper) splits take precedence over outer ones.
// Returns the split, its direction, and the total area the split occupies.
func FindSplitAtBorder(root Node, x, y int, area Rect, hitZone int) (*SplitNode, Direction, Rect) {
	split, ok := root.(*SplitNode)
	if !ok {
		return nil, 0, Rect{}
	}

	firstArea, secondArea := SplitRect(area, split.Direction, split.Ratio)

	// Recurse into children first — deeper splits take precedence.
	if s, d, a := FindSplitAtBorder(split.First, x, y, firstArea, hitZone); s != nil {
		return s, d, a
	}
	if s, d, a := FindSplitAtBorder(split.Second, x, y, secondArea, hitZone); s != nil {
		return s, d, a
	}

	// Check if (x, y) is near this split's border.
	switch split.Direction {
	case Horizontal:
		borderX := firstArea.X + firstArea.Width
		if intAbs(x-borderX) <= hitZone && y >= area.Y && y < area.Y+area.Height {
			return split, Horizontal, area
		}
	case Vertical:
		borderY := firstArea.Y + firstArea.Height
		if intAbs(y-borderY) <= hitZone && x >= area.X && x < area.X+area.Width {
			return split, Vertical, area
		}
	}

	return nil, 0, Rect{}
}

// intAbs returns the absolute value of an int.
func intAbs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
