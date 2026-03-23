package diffview

import (
	"strconv"

	"charm.land/bubbles/v2/viewport"

	"github.com/jethrokuan/pry/internal/diff"
)

// DiffNav manages cursor positions, viewport scrolling, diff line mapping,
// and file tree navigation state.
type DiffNav struct {
	fileCursor   int
	cursor       CursorTarget
	diffViewport viewport.Model
	treeViewport viewport.Model
	focus        Focus
	showTree     bool

	// File tree
	cachedTree    *treeNode
	treeRows      []treeRow
	treeCursor    int
	collapsedDirs map[string]bool

	// Hunk folding (key: "filePath:hunkIdx")
	collapsedHunks map[string]bool

	// Visual selection
	visualMode  bool
	visualStart int
	visualEnd   int

	// Diff line mapping
	diffLines []diffLineInfo

	// Active navigation type for the always-visible position counter.
	// 'h' = hunk (default); 'f','F' = file/unviewed; 'c','C' = comment; '/' = search
	activeCycler rune

	// Jump list for Ctrl-o / Ctrl-i navigation
	jumpList    []jumpPos
	jumpCursor  int // points to current position in jumpList (-1 = at head)
}

// jumpPos records a cursor position for the jump list.
type jumpPos struct {
	fileCursor int
	lineIdx    int // diff line index (always restored as CursorLine)
}

// buildDiffLines flattens the hunks of the current file into a flat diffLines slice.
// Collapsed hunks are represented by a single placeholder entry.
func (n *DiffNav) buildDiffLines(files []diff.DiffFile) {
	n.diffLines = nil
	if len(files) == 0 || n.fileCursor >= len(files) {
		return
	}
	file := files[n.fileCursor]
	for hi, hunk := range file.Hunks {
		hk := hunkKey(file.Path, hi)
		if n.collapsedHunks[hk] {
			// Single placeholder for the collapsed hunk
			n.diffLines = append(n.diffLines, diffLineInfo{
				fileIdx:   n.fileCursor,
				hunkIdx:   hi,
				lineIdx:   0,
				collapsed: true,
			})
			continue
		}
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

// hunkKey returns a map key for hunk collapse state.
func hunkKey(filePath string, hunkIdx int) string {
	return filePath + ":" + strconv.Itoa(hunkIdx)
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
	if visibleRow < n.treeViewport.YOffset() {
		n.treeViewport.SetYOffset(visibleRow)
	} else if visibleRow >= n.treeViewport.YOffset()+n.treeViewport.Height() {
		n.treeViewport.SetYOffset(visibleRow - n.treeViewport.Height() + 1)
	}
}

// pushJump records the current position in the jump list before a navigation jump.
// Truncates any forward history if we're not at the head.
func (n *DiffNav) pushJump() {
	pos := jumpPos{fileCursor: n.fileCursor, lineIdx: n.cursor.LineIdx}

	// If we have forward history, truncate it
	if n.jumpCursor >= 0 && n.jumpCursor < len(n.jumpList)-1 {
		n.jumpList = n.jumpList[:n.jumpCursor+1]
	}

	// Deduplicate: don't push if identical to the last entry
	if len(n.jumpList) > 0 {
		last := n.jumpList[len(n.jumpList)-1]
		if last.fileCursor == pos.fileCursor && last.lineIdx == pos.lineIdx {
			n.jumpCursor = len(n.jumpList) - 1
			return
		}
	}

	// Cap the jump list at 100 entries
	const maxJumps = 100
	if len(n.jumpList) >= maxJumps {
		n.jumpList = n.jumpList[1:]
	}

	n.jumpList = append(n.jumpList, pos)
	n.jumpCursor = len(n.jumpList) - 1
}

// jumpBack moves to the previous position in the jump list.
// Returns the position and true if a jump occurred.
func (n *DiffNav) jumpBack() (jumpPos, bool) {
	if len(n.jumpList) == 0 || n.jumpCursor <= 0 {
		return jumpPos{}, false
	}
	n.jumpCursor--
	return n.jumpList[n.jumpCursor], true
}

// jumpForward moves to the next position in the jump list.
// Returns the position and true if a jump occurred.
func (n *DiffNav) jumpForward() (jumpPos, bool) {
	if n.jumpCursor >= len(n.jumpList)-1 {
		return jumpPos{}, false
	}
	n.jumpCursor++
	return n.jumpList[n.jumpCursor], true
}
