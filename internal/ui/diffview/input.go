package diffview

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// --- Tree key handling ---

func (m Model) handleTreeKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Up):
		if m.nav.treeCursor > 0 {
			m.nav.treeCursor--
			m.onTreeCursorChanged()
		}
	case key.Matches(msg, keys.Down):
		if m.nav.treeCursor < len(m.nav.treeRows)-1 {
			m.nav.treeCursor++
			m.onTreeCursorChanged()
		}
	case key.Matches(msg, keys.Enter):
		return m.handleTreeEnter()
	}

	switch msg.String() {
	case "/":
		m.search.filterActive = true
		m.search.filterInput = ""
		m.search.filterFiles = m.allFileIndices()
		m.search.filterCursor = 0
		return m, nil
	case "g":
		m.nav.pendingG = true
		return m, nil
	case "n":
		m.repeatCycler(true)
		return m, nil
	case "p":
		m.repeatCycler(false)
		return m, nil
	case "?":
		m.showHelp = true
		return m, nil
	}

	return m, nil
}

// onTreeCursorChanged updates fileCursor if on a file row and syncs viewports.
func (m *Model) onTreeCursorChanged() {
	if m.nav.treeCursor >= 0 && m.nav.treeCursor < len(m.nav.treeRows) {
		row := m.nav.treeRows[m.nav.treeCursor]
		if row.node.fileIdx >= 0 {
			m.nav.fileCursor = row.node.fileIdx
			m.nav.buildDiffLines(m.files)
			m.nav.diffCursor = 0
			m.updateDiffContent()
		} else {
			// On a folder — just refresh the tree rendering (cursor highlight)
			m.nav.treeViewport.SetContent(m.renderFileTree())
		}
	}
	m.nav.syncTreeViewportToCursor()
}

// handleTreeEnter handles enter key: file → focus diff, folder → toggle collapse.
func (m Model) handleTreeEnter() (Model, tea.Cmd) {
	if m.nav.treeCursor < 0 || m.nav.treeCursor >= len(m.nav.treeRows) {
		return m, nil
	}
	row := m.nav.treeRows[m.nav.treeCursor]
	if row.node.fileIdx >= 0 {
		// File: focus diff pane
		m.nav.focus = FocusDiff
		m.nav.diffCursor = 0
		m.nav.diffViewport.GotoTop()
		m.updateDiffContent()
	} else {
		// Folder: toggle collapse
		m.toggleCollapse(row.node)
	}
	return m, nil
}

// toggleCollapse flips the collapsed state and rebuilds tree rows, keeping cursor on the same node.
func (m *Model) toggleCollapse(node *treeNode) {
	m.nav.collapsedDirs[node.dirPath] = !m.nav.collapsedDirs[node.dirPath]
	// Remember the current node to restore cursor position
	currentNode := m.nav.treeRows[m.nav.treeCursor].node
	m.nav.rebuildTreeRows()
	// Find the same node in the new rows
	for i, row := range m.nav.treeRows {
		if row.node == currentNode {
			m.nav.treeCursor = i
			break
		}
	}
	m.nav.treeViewport.SetContent(m.renderFileTree())
	m.nav.syncTreeViewportToCursor()
}

// toggleFoldAtCursor toggles collapse on the current folder (tab in tree).
// If cursor is on a file, toggles its parent folder.
func (m Model) toggleFoldAtCursor() Model {
	if m.nav.treeCursor < 0 || m.nav.treeCursor >= len(m.nav.treeRows) {
		return m
	}
	row := m.nav.treeRows[m.nav.treeCursor]
	if row.node.fileIdx == -1 {
		// On a folder — toggle it
		m.toggleCollapse(row.node)
	} else {
		// On a file — find and toggle parent folder
		for i := m.nav.treeCursor - 1; i >= 0; i-- {
			if m.nav.treeRows[i].node.fileIdx == -1 && m.nav.treeRows[i].depth < row.depth {
				m.toggleCollapse(m.nav.treeRows[i].node)
				break
			}
		}
	}
	return m
}

// toggleFoldAll collapses all folders if any are expanded, or expands all if all are collapsed (S-tab in tree).
func (m Model) toggleFoldAll() Model {
	if m.nav.cachedTree == nil {
		return m
	}
	// Check if any folder is currently expanded (i.e. not in collapsedDirs)
	anyExpanded := false
	for _, row := range m.nav.treeRows {
		if row.node.fileIdx == -1 && !m.nav.collapsedDirs[row.node.dirPath] {
			anyExpanded = true
			break
		}
	}

	// Collect all directory paths from the full tree
	allDirs := collectAllDirPaths(m.nav.cachedTree)

	if anyExpanded {
		// Collapse all
		for _, dp := range allDirs {
			m.nav.collapsedDirs[dp] = true
		}
	} else {
		// Expand all
		for _, dp := range allDirs {
			delete(m.nav.collapsedDirs, dp)
		}
	}

	currentNode := m.nav.treeRows[m.nav.treeCursor].node
	m.nav.rebuildTreeRows()
	// Try to restore cursor on same node
	for i, row := range m.nav.treeRows {
		if row.node == currentNode {
			m.nav.treeCursor = i
			m.nav.treeViewport.SetContent(m.renderFileTree())
			m.nav.syncTreeViewportToCursor()
			return m
		}
	}
	// Fallback: clamp cursor
	if m.nav.treeCursor >= len(m.nav.treeRows) {
		m.nav.treeCursor = len(m.nav.treeRows) - 1
	}
	m.nav.treeViewport.SetContent(m.renderFileTree())
	m.nav.syncTreeViewportToCursor()
	return m
}

// moveToNextFile moves treeCursor to the next file row (skipping folders).
func (m *Model) moveToNextFile() {
	for i := m.nav.treeCursor + 1; i < len(m.nav.treeRows); i++ {
		if m.nav.treeRows[i].node.fileIdx >= 0 {
			m.nav.treeCursor = i
			m.onTreeCursorChanged()
			return
		}
	}
}

// moveToPrevFile moves treeCursor to the previous file row (skipping folders).
func (m *Model) moveToPrevFile() {
	for i := m.nav.treeCursor - 1; i >= 0; i-- {
		if m.nav.treeRows[i].node.fileIdx >= 0 {
			m.nav.treeCursor = i
			m.onTreeCursorChanged()
			return
		}
	}
}

func (m Model) handleDiffKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Up):
		if m.nav.diffCursor > 0 {
			// Check if previous line has expanded comments → enter from bottom
			if !m.nav.visualMode {
				prevRefs := m.commentRefsAtLine(m.nav.diffCursor - 1)
				if len(prevRefs) > 0 {
					m.nav.diffCursor--
					m.comments.cursor = len(prevRefs) - 1
					m.syncViewportToCursor()
					return m, nil
				}
			}
			m.nav.diffCursor--
			if m.nav.visualMode {
				m.nav.visualEnd = m.nav.diffCursor
			}
			m.syncViewportToCursor()
		}
	case key.Matches(msg, keys.Down):
		// Enter comment selection if current line has expanded comments
		if !m.nav.visualMode {
			refs := m.commentRefsAtCursor()
			if len(refs) > 0 {
				m.comments.cursor = 0
				m.updateDiffContent()
				return m, nil
			}
		}
		if m.nav.diffCursor < len(m.nav.diffLines)-1 {
			m.nav.diffCursor++
			if m.nav.visualMode {
				m.nav.visualEnd = m.nav.diffCursor
			}
			m.syncViewportToCursor()
		}
	case key.Matches(msg, keys.PageUp):
		m.nav.diffCursor -= m.nav.diffViewport.Height / 2
		if m.nav.diffCursor < 0 {
			m.nav.diffCursor = 0
		}
		if m.nav.visualMode {
			m.nav.visualEnd = m.nav.diffCursor
		}
		m.syncViewportToCursor()
	case key.Matches(msg, keys.PageDown):
		m.nav.diffCursor += m.nav.diffViewport.Height / 2
		if m.nav.diffCursor >= len(m.nav.diffLines) {
			m.nav.diffCursor = len(m.nav.diffLines) - 1
		}
		if m.nav.visualMode {
			m.nav.visualEnd = m.nav.diffCursor
		}
		m.syncViewportToCursor()
	case key.Matches(msg, keys.Comment):
		return m, m.startComment()
	case key.Matches(msg, keys.Enter):
		if len(m.files) > 0 && m.nav.diffCursor < len(m.nav.diffLines) {
			path := m.files[m.nav.fileCursor].Path
			dl := m.nav.diffLines[m.nav.diffCursor]
			if m.lineHasComments(path, dl) {
				m.openCommentPopup()
				return m, nil
			}
		}
	}

	// Trigger input modes (check raw key string)
	switch msg.String() {
	case "n":
		m.repeatCycler(true)
		return m, nil
	case "p":
		m.repeatCycler(false)
		return m, nil
	case "d":
		m.nav.pendingD = true
		return m, nil
	case "g":
		m.nav.pendingG = true
		return m, nil
	case "G":
		if len(m.nav.diffLines) > 0 {
			m.nav.diffCursor = len(m.nav.diffLines) - 1
			m.syncViewportToCursor()
		}
		return m, nil
	case "/":
		m.search.active = true
		m.search.input = ""
		return m, nil
	case "ctrl+p":
		m.search.filterActive = true
		m.search.filterInput = ""
		m.search.filterFiles = m.allFileIndices()
		m.search.filterCursor = 0
		return m, nil
	case "?":
		m.showHelp = true
		return m, nil
	}

	return m, nil
}

// --- Pending g key (for gg) ---

func (m Model) handlePendingG(msg tea.KeyMsg) (Model, tea.Cmd) {
	m.nav.pendingG = false
	s := msg.String()

	switch s {
	case "h":
		m.nav.activeCycler = 'h'
		m.navigateHunk(true)
	case "f":
		m.nav.activeCycler = 'f'
		m.navigateFile(true, false)
	case "F":
		m.nav.activeCycler = 'F'
		m.navigateFile(true, true)
	case "c":
		m.nav.activeCycler = 'c'
		m.navigateComment(true, false)
	case "C":
		m.nav.activeCycler = 'C'
		m.navigateComment(true, true)
	case "g":
		// gg = go to top
		m.nav.diffCursor = 0
		m.nav.diffViewport.GotoTop()
		m.updateDiffContent()
	default:
		// Check for digit → enter goto-line mode
		if len(s) == 1 && s[0] >= '0' && s[0] <= '9' {
			m.search.gotoActive = true
			m.search.gotoInput = s
			return m, nil
		}
		// Unknown key — ignore
	}
	return m, nil
}

// --- Pending d key (for dd delete, de edit) ---

func (m Model) handlePendingD(msg tea.KeyMsg) (Model, tea.Cmd) {
	m.nav.pendingD = false
	switch msg.String() {
	case "d":
		return m.deleteCommentAtCursor()
	case "e":
		return m.editCommentAtCursor()
	}
	return m, nil
}

// repeatCycler repeats the last go-to motion in the active cycler direction.
func (m *Model) repeatCycler(forward bool) {
	switch m.nav.activeCycler {
	case 'f':
		m.navigateFile(forward, false)
	case 'F':
		m.navigateFile(forward, true)
	case 'c':
		m.navigateComment(forward, false)
	case 'C':
		m.navigateComment(forward, true)
	case '/':
		if m.search.query != "" {
			if forward {
				m.jumpToNextSearchMatch()
			} else {
				m.jumpToPrevSearchMatch()
			}
		}
	case 'h':
		fallthrough
	default:
		m.navigateHunk(forward)
	}
}

// navigateFile moves to the next/prev file. If unviewedOnly, skip viewed files.
func (m *Model) navigateFile(forward, unviewedOnly bool) {
	n := len(m.files)
	if n == 0 {
		return
	}

	if m.nav.focus == FocusFileTree {
		// In tree view, delegate to tree navigation
		if unviewedOnly {
			m.moveToUnviewedFileInTree(forward)
		} else {
			if forward {
				m.moveToNextFile()
			} else {
				m.moveToPrevFile()
			}
		}
		return
	}

	// In diff view
	for i := 1; i < n; i++ {
		var idx int
		if forward {
			idx = (m.nav.fileCursor + i) % n
		} else {
			idx = (m.nav.fileCursor - i + n) % n
		}
		if unviewedOnly && m.review.ViewedFiles[m.files[idx].Path] {
			continue
		}
		oldIdx := m.nav.fileCursor
		m.nav.fileCursor = idx
		m.nav.buildDiffLines(m.files)
		m.nav.diffCursor = 0
		m.nav.diffViewport.GotoTop()
		m.updateDiffContent()
		m.autoFollowFile(oldIdx, m.nav.fileCursor)
		return
	}
}

// moveToUnviewedFileInTree moves treeCursor to the next/prev unviewed file row.
func (m *Model) moveToUnviewedFileInTree(forward bool) {
	n := len(m.nav.treeRows)
	if n == 0 {
		return
	}
	for i := 1; i < n; i++ {
		var idx int
		if forward {
			idx = (m.nav.treeCursor + i) % n
		} else {
			idx = (m.nav.treeCursor - i + n) % n
		}
		row := m.nav.treeRows[idx]
		if row.node.fileIdx >= 0 && !m.review.ViewedFiles[m.files[row.node.fileIdx].Path] {
			m.nav.treeCursor = idx
			m.onTreeCursorChanged()
			return
		}
	}
}

// navigateComment moves to the next/prev comment. If crossFile, jump to next file with comments.
// When landing on a comment, expands the comment block and selects the first comment.
func (m *Model) navigateComment(forward, crossFile bool) {
	if crossFile {
		m.navigateFileWithComments(forward)
		return
	}
	// Within current file: find next diff line that has comments
	if len(m.nav.diffLines) == 0 {
		m.navigateCommentCrossFile(forward)
		return
	}
	n := len(m.nav.diffLines)
	path := m.files[m.nav.fileCursor].Path
	for i := 1; i < n; i++ {
		var idx int
		if forward {
			idx = (m.nav.diffCursor + i) % n
		} else {
			idx = (m.nav.diffCursor - i + n) % n
		}
		dl := m.nav.diffLines[idx]
		if m.lineHasComments(path, dl) {
			m.nav.diffCursor = idx
			m.expandAndSelectComment(idx)
			m.syncViewportToCursorWithComments()
			return
		}
	}
	// No more comments in current file — try cross-file
	m.navigateCommentCrossFile(forward)
}

// navigateCommentCrossFile jumps to the next/prev file that has comments,
// positions the cursor on the first/last commented line, and expands+selects it.
// Skips files where comments exist but don't map to any visible diff line.
func (m *Model) navigateCommentCrossFile(forward bool) {
	nFiles := len(m.files)
	for i := 1; i < nFiles; i++ {
		var idx int
		if forward {
			idx = (m.nav.fileCursor + i) % nFiles
		} else {
			idx = (m.nav.fileCursor - i + nFiles) % nFiles
		}
		if !m.fileHasComments(m.files[idx].Path) {
			continue
		}
		oldIdx := m.nav.fileCursor
		m.nav.fileCursor = idx
		m.nav.buildDiffLines(m.files)
		// Find first/last commented diff line
		commentLine := m.findCommentedDiffLine(m.files[idx].Path, forward)
		if commentLine < 0 {
			// Comments exist but none map to visible diff lines (e.g. outdated);
			// restore and keep searching.
			m.nav.fileCursor = oldIdx
			m.nav.buildDiffLines(m.files)
			continue
		}
		m.nav.diffCursor = commentLine
		m.expandAndSelectComment(m.nav.diffCursor)
		m.nav.diffViewport.GotoTop()
		m.updateDiffContent()
		m.autoFollowFile(oldIdx, m.nav.fileCursor)
		m.syncViewportToCursorWithComments()
		return
	}
}

// expandAndSelectComment expands the comment block at the given cursor and selects the first comment.
func (m *Model) expandAndSelectComment(cursor int) {
	if cursor < 0 || cursor >= len(m.nav.diffLines) || len(m.files) == 0 {
		return
	}
	dl := m.nav.diffLines[cursor]
	path := m.files[m.nav.fileCursor].Path
	if dl.newLine > 0 {
		m.comments.expanded[commentKey(path, dl.newLine)] = true
	}
	if dl.oldLine > 0 {
		m.comments.expanded[commentKey(path, dl.oldLine)] = true
	}
	m.comments.cursor = 0
}

// navigateFileWithComments moves to the next/prev file that has comments,
// positioning the cursor on the first/last commented line.
func (m *Model) navigateFileWithComments(forward bool) {
	n := len(m.files)
	if n == 0 {
		return
	}
	for i := 1; i < n; i++ {
		var idx int
		if forward {
			idx = (m.nav.fileCursor + i) % n
		} else {
			idx = (m.nav.fileCursor - i + n) % n
		}
		if !m.fileHasComments(m.files[idx].Path) {
			continue
		}
		oldIdx := m.nav.fileCursor
		m.nav.fileCursor = idx
		m.nav.buildDiffLines(m.files)
		commentLine := m.findCommentedDiffLine(m.files[idx].Path, forward)
		if commentLine < 0 {
			// Comments exist but none map to visible diff lines; restore and keep searching.
			m.nav.fileCursor = oldIdx
			m.nav.buildDiffLines(m.files)
			continue
		}
		m.nav.diffCursor = commentLine
		m.expandAndSelectComment(m.nav.diffCursor)
		m.nav.diffViewport.GotoTop()
		m.updateDiffContent()
		m.autoFollowFile(oldIdx, m.nav.fileCursor)
		m.syncViewportToCursorWithComments()
		return
	}
}

// findCommentedDiffLine returns the index of the first (forward=true) or last (forward=false)
// diff line that has comments for the given path. Returns -1 if none found.
func (m *Model) findCommentedDiffLine(path string, forward bool) int {
	if forward {
		for j := 0; j < len(m.nav.diffLines); j++ {
			if m.lineHasComments(path, m.nav.diffLines[j]) {
				return j
			}
		}
	} else {
		for j := len(m.nav.diffLines) - 1; j >= 0; j-- {
			if m.lineHasComments(path, m.nav.diffLines[j]) {
				return j
			}
		}
	}
	return -1
}

// navigateHunk moves to the first line of the next/prev hunk.
// Crosses file boundaries when at the last/first hunk.
func (m *Model) navigateHunk(forward bool) {
	if len(m.nav.diffLines) == 0 {
		m.navigateHunkCrossFile(forward)
		return
	}
	currentHunk := m.nav.diffLines[m.nav.diffCursor].hunkIdx

	if forward {
		for i := m.nav.diffCursor + 1; i < len(m.nav.diffLines); i++ {
			if m.nav.diffLines[i].hunkIdx != currentHunk {
				m.nav.diffCursor = i
				m.syncViewportToCursor()
				return
			}
		}
		// At last hunk — cross to next file
		m.navigateHunkCrossFile(true)
	} else {
		// First, go to start of current hunk
		startOfCurrent := m.nav.diffCursor
		for startOfCurrent > 0 && m.nav.diffLines[startOfCurrent-1].hunkIdx == currentHunk {
			startOfCurrent--
		}
		if startOfCurrent < m.nav.diffCursor {
			// We weren't at the start — go there
			m.nav.diffCursor = startOfCurrent
			m.syncViewportToCursor()
			return
		}
		// Already at start — go to start of previous hunk
		if startOfCurrent > 0 {
			prevHunk := m.nav.diffLines[startOfCurrent-1].hunkIdx
			for i := startOfCurrent - 1; i >= 0; i-- {
				if i == 0 || m.nav.diffLines[i-1].hunkIdx != prevHunk {
					m.nav.diffCursor = i
					m.syncViewportToCursor()
					return
				}
			}
		}
		// At first hunk — cross to prev file
		m.navigateHunkCrossFile(false)
	}
}

// navigateHunkCrossFile moves to the next/prev file and positions at the
// first hunk (forward) or the last hunk's start (backward).
func (m *Model) navigateHunkCrossFile(forward bool) {
	nFiles := len(m.files)
	if nFiles <= 1 {
		return
	}
	var nextIdx int
	if forward {
		nextIdx = (m.nav.fileCursor + 1) % nFiles
	} else {
		nextIdx = (m.nav.fileCursor - 1 + nFiles) % nFiles
	}
	oldIdx := m.nav.fileCursor
	m.nav.fileCursor = nextIdx
	m.nav.buildDiffLines(m.files)
	if forward {
		m.nav.diffCursor = 0
	} else {
		// Go to last hunk's start
		if len(m.nav.diffLines) > 0 {
			lastHunk := m.nav.diffLines[len(m.nav.diffLines)-1].hunkIdx
			for i := len(m.nav.diffLines) - 1; i >= 0; i-- {
				if i == 0 || m.nav.diffLines[i-1].hunkIdx != lastHunk {
					m.nav.diffCursor = i
					break
				}
			}
		}
	}
	m.nav.diffViewport.GotoTop()
	m.updateDiffContent()
	m.autoFollowFile(oldIdx, m.nav.fileCursor)
	m.syncViewportToCursor()
}

// lineHasComments returns true if the given diff line has any comments.
func (m *Model) lineHasComments(path string, dl diffLineInfo) bool {
	if dl.newLine > 0 && len(m.commentsForLine(path, dl.newLine, "RIGHT"))+len(m.localPendingForLine(path, dl.newLine, "RIGHT")) > 0 {
		return true
	}
	if dl.oldLine > 0 && len(m.commentsForLine(path, dl.oldLine, "LEFT"))+len(m.localPendingForLine(path, dl.oldLine, "LEFT")) > 0 {
		return true
	}
	return false
}

// fileHasComments returns true if any comments exist for the given file path.
func (m *Model) fileHasComments(path string) bool {
	return m.comments.fileCommentIndex[path]
}

// --- Goto line handling ---

func (m Model) handleGotoKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if m.search.gotoInput != "" {
			lineNum, err := strconv.Atoi(m.search.gotoInput)
			if err == nil {
				m.jumpToLine(lineNum)
			}
		}
		m.search.gotoActive = false
		m.search.gotoInput = ""
		return m, nil
	case "esc":
		m.search.gotoActive = false
		m.search.gotoInput = ""
		return m, nil
	case "backspace":
		if len(m.search.gotoInput) > 0 {
			m.search.gotoInput = m.search.gotoInput[:len(m.search.gotoInput)-1]
		}
		return m, nil
	default:
		if len(msg.String()) == 1 && msg.String()[0] >= '0' && msg.String()[0] <= '9' {
			m.search.gotoInput += msg.String()
		}
		return m, nil
	}
}

func (m *Model) jumpToLine(lineNum int) {
	for i, dl := range m.nav.diffLines {
		if dl.newLine == lineNum || dl.oldLine == lineNum {
			m.nav.diffCursor = i
			m.syncViewportToCursor()
			return
		}
	}
}

// --- Search handling ---

func (m Model) handleSearchKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.search.query = m.search.input
		m.search.active = false
		if m.search.query != "" {
			m.nav.activeCycler = '/'
			m.jumpToNextSearchMatch()
		}
		m.updateDiffContent()
		return m, nil
	case "esc":
		m.search.active = false
		m.search.input = ""
		m.updateDiffContent()
		return m, nil
	case "backspace":
		if len(m.search.input) > 0 {
			m.search.input = m.search.input[:len(m.search.input)-1]
		}
		return m, nil
	default:
		if len(msg.Runes) > 0 {
			m.search.input += string(msg.Runes)
		}
		return m, nil
	}
}

func (m *Model) jumpToNextSearchMatch() {
	query := strings.ToLower(m.search.query)
	for i := m.nav.diffCursor + 1; i < len(m.nav.diffLines); i++ {
		if strings.Contains(strings.ToLower(m.nav.diffLines[i].content), query) {
			m.nav.diffCursor = i
			m.syncViewportToCursor()
			return
		}
	}
	// Wrap around
	for i := 0; i <= m.nav.diffCursor; i++ {
		if strings.Contains(strings.ToLower(m.nav.diffLines[i].content), query) {
			m.nav.diffCursor = i
			m.syncViewportToCursor()
			return
		}
	}
}

func (m *Model) jumpToPrevSearchMatch() {
	query := strings.ToLower(m.search.query)
	for i := m.nav.diffCursor - 1; i >= 0; i-- {
		if strings.Contains(strings.ToLower(m.nav.diffLines[i].content), query) {
			m.nav.diffCursor = i
			m.syncViewportToCursor()
			return
		}
	}
	// Wrap around
	for i := len(m.nav.diffLines) - 1; i >= m.nav.diffCursor; i-- {
		if strings.Contains(strings.ToLower(m.nav.diffLines[i].content), query) {
			m.nav.diffCursor = i
			m.syncViewportToCursor()
			return
		}
	}
}

// --- File filter handling ---

func (m Model) handleFilterKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if len(m.search.filterFiles) > 0 && m.search.filterCursor < len(m.search.filterFiles) {
			oldIdx := m.nav.fileCursor
			m.nav.fileCursor = m.search.filterFiles[m.search.filterCursor]
			m.nav.buildDiffLines(m.files)
			m.nav.diffCursor = 0
			m.nav.diffViewport.GotoTop()
			m.updateDiffContent()
			m.autoFollowFile(oldIdx, m.nav.fileCursor)
		}
		m.search.filterActive = false
		m.search.filterInput = ""
		return m, nil
	case "esc":
		m.search.filterActive = false
		m.search.filterInput = ""
		return m, nil
	case "up":
		if m.search.filterCursor > 0 {
			m.search.filterCursor--
		}
		return m, nil
	case "down":
		if m.search.filterCursor < len(m.search.filterFiles)-1 {
			m.search.filterCursor++
		}
		return m, nil
	case "backspace":
		if len(m.search.filterInput) > 0 {
			m.search.filterInput = m.search.filterInput[:len(m.search.filterInput)-1]
			m.updateFilteredFiles()
		}
		return m, nil
	default:
		if len(msg.Runes) > 0 {
			m.search.filterInput += string(msg.Runes)
			m.updateFilteredFiles()
		}
		return m, nil
	}
}

func (m *Model) updateFilteredFiles() {
	m.search.filterFiles = nil
	m.search.filterCursor = 0
	query := strings.ToLower(m.search.filterInput)
	for i, f := range m.files {
		if query == "" || strings.Contains(strings.ToLower(f.Path), query) {
			m.search.filterFiles = append(m.search.filterFiles, i)
		}
	}
}

func (m Model) allFileIndices() []int {
	indices := make([]int, len(m.files))
	for i := range m.files {
		indices[i] = i
	}
	return indices
}

// --- Inline comment key handling ---

func (m Model) handleInlineKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, inlineKeys.Cancel):
		if m.comments.inlineTextarea.Value() != "" && !m.comments.confirmDiscard {
			m.comments.confirmDiscard = true
			return m, nil
		}
		m.closeInlineComment()
		return m, nil

	case key.Matches(msg, inlineKeys.Save):
		return m.saveInlineComment()

	case key.Matches(msg, inlineKeys.ToggleMode):
		if m.comments.inlineMode == commentModeComment {
			m.comments.inlineMode = commentModeSuggestion
			if m.comments.inlineSuggestion != "" && m.comments.inlineTextarea.Value() == "" {
				m.comments.inlineTextarea.SetValue(m.comments.inlineSuggestion)
			}
		} else {
			m.comments.inlineMode = commentModeComment
		}
		return m, nil

	case key.Matches(msg, inlineKeys.OpenEditor):
		return m, m.openExternalEditorForComment()
	}

	// Any other key resets the discard confirmation
	m.comments.confirmDiscard = false

	// Forward to textarea
	var cmd tea.Cmd
	m.comments.inlineTextarea, cmd = m.comments.inlineTextarea.Update(msg)
	return m, cmd
}

// --- Comment popup key handling ---

func (m Model) handleCommentPopupKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "enter", "q":
		m.comments.popupActive = false
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	}

	// Delegate scrolling to the viewport (handles j/k, up/down, pgup/pgdn, etc.)
	var cmd tea.Cmd
	m.comments.popupViewport, cmd = m.comments.popupViewport.Update(msg)
	return m, cmd
}
