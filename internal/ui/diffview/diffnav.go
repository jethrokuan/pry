package diffview

import (
	"github.com/charmbracelet/bubbles/viewport"

	"github.com/jkuan/pr-review/internal/diff"
)

// DiffNav manages cursor positions, viewport scrolling, diff line mapping,
// and file tree navigation state.
type DiffNav struct {
	fileCursor   int
	diffCursor   int
	diffViewport viewport.Model
	treeViewport viewport.Model
	focus        Focus
	showTree     bool

	// File tree
	cachedTree    *treeNode
	treeRows      []treeRow
	treeCursor    int
	collapsedDirs map[string]bool

	// Visual selection
	visualMode  bool
	visualStart int
	visualEnd   int

	// Diff line mapping
	diffLines []diffLineInfo

	// Multi-key state
	pendingG     bool
	pendingD     bool
	activeCycler rune // 'h' = hunk (default); 'f','F','c','C' = go-to motion; '/' = search
}

// buildDiffLines flattens the hunks of the current file into a flat diffLines slice.
func (n *DiffNav) buildDiffLines(files []diff.DiffFile) {
	n.diffLines = nil
	if len(files) == 0 || n.fileCursor >= len(files) {
		return
	}
	file := files[n.fileCursor]
	for hi, hunk := range file.Hunks {
		for li, line := range hunk.Lines {
			n.diffLines = append(n.diffLines, diffLineInfo{
				fileIdx:  n.fileCursor,
				hunkIdx:  hi,
				lineIdx:  li,
				newLine:  line.NewNum,
				oldLine:  line.OldNum,
				lineType: line.Type,
				content:  line.Content,
			})
		}
	}
}

// rebuildTreeRows flattens the cached tree with current collapse state.
func (n *DiffNav) rebuildTreeRows() {
	if n.cachedTree == nil {
		n.treeRows = nil
		return
	}
	n.treeRows = flattenTree(n.cachedTree, n.collapsedDirs)
}

// syncTreeCursorToFileCursor finds the tree row matching fileCursor and updates treeCursor.
func (n *DiffNav) syncTreeCursorToFileCursor() {
	for i, row := range n.treeRows {
		if row.node.fileIdx == n.fileCursor {
			n.treeCursor = i
			n.syncTreeViewportToCursor()
			return
		}
	}
}

// cyclicSearch finds the next index starting from start that satisfies match,
// scanning forward or backward with wraparound over count elements.
// Returns -1 if no match found.
func cyclicSearch(start, count int, forward bool, match func(idx int) bool) int {
	for i := 1; i < count; i++ {
		var idx int
		if forward {
			idx = (start + i) % count
		} else {
			idx = (start - i + count) % count
		}
		if match(idx) {
			return idx
		}
	}
	return -1
}

// syncTreeViewportToCursor ensures the treeCursor row is visible in the tree viewport.
func (n *DiffNav) syncTreeViewportToCursor() {
	// Account for the 2-line header (title + blank line)
	headerLines := 2
	visibleRow := n.treeCursor + headerLines
	if visibleRow < n.treeViewport.YOffset {
		n.treeViewport.SetYOffset(visibleRow)
	} else if visibleRow >= n.treeViewport.YOffset+n.treeViewport.Height {
		n.treeViewport.SetYOffset(visibleRow - n.treeViewport.Height + 1)
	}
}
