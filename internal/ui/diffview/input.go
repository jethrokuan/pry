package diffview

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// --- Tree key handling ---

func (m Model) handleTreeKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
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
	case key.Matches(msg, keys.Search):
		m.search.filterActive = true
		m.search.filterInput = ""
		m.search.filterFiles = m.allFileIndices()
		m.search.filterCursor = 0
		return m, nil
	case key.Matches(msg, keys.Help):
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

// toggleFoldAtDiffCursor toggles folding based on what's at the cursor in the diff view.
// If the cursor is on a collapsed hunk placeholder, expand it.
// If the cursor line has comments, toggle comment fold.
// Otherwise, collapse the current hunk.
func (m Model) toggleFoldAtDiffCursor() Model {
	if len(m.files) == 0 || m.nav.fileCursor >= len(m.files) {
		return m
	}
	if m.nav.diffCursor >= len(m.nav.diffLines) {
		return m
	}

	dl := m.nav.diffLines[m.nav.diffCursor]
	path := m.files[m.nav.fileCursor].Path

	// If it's a collapsed hunk placeholder, expand it
	if dl.collapsed {
		hk := hunkKey(path, dl.hunkIdx)
		delete(m.nav.collapsedHunks, hk)
		m.nav.buildDiffLines(m.files)
		m.updateDiffContent()
		return m
	}

	// If cursor line has comments, toggle comment fold
	if m.lineHasComments(path, dl) {
		return m.toggleCommentAtCursor()
	}

	// Otherwise, collapse the current hunk
	hk := hunkKey(path, dl.hunkIdx)
	m.nav.collapsedHunks[hk] = true
	m.nav.buildDiffLines(m.files)
	// Clamp cursor if it's now past the end
	if m.nav.diffCursor >= len(m.nav.diffLines) {
		m.nav.diffCursor = len(m.nav.diffLines) - 1
	}
	m.updateDiffContent()
	return m
}

// toggleAllFolds toggles all comment folds and hunk folds for the current file.
func (m Model) toggleAllFolds() Model {
	m.comments.cursor = -1

	// Check if anything is expanded (comments or hunks)
	anyCommentExpanded := false
	for _, v := range m.comments.expanded {
		if v {
			anyCommentExpanded = true
			break
		}
	}

	anyHunkExpanded := false
	if len(m.files) > 0 && m.nav.fileCursor < len(m.files) {
		file := m.files[m.nav.fileCursor]
		for hi := range file.Hunks {
			hk := hunkKey(file.Path, hi)
			if !m.nav.collapsedHunks[hk] {
				anyHunkExpanded = true
				break
			}
		}
	}

	anyExpanded := anyCommentExpanded || anyHunkExpanded

	// Toggle all comments
	allCommentKeys := make(map[string]bool)
	for _, c := range m.comments.existing {
		allCommentKeys[commentKey(c.Path, c.Line)] = true
	}
	for _, c := range m.comments.forgeComments {
		allCommentKeys[commentKey(c.Path, c.Line)] = true
	}
	for _, c := range m.review.Comments {
		allCommentKeys[commentKey(c.Path, c.Line)] = true
	}
	for ck := range allCommentKeys {
		m.comments.expanded[ck] = !anyExpanded
	}

	// Toggle all hunks for current file
	if len(m.files) > 0 && m.nav.fileCursor < len(m.files) {
		file := m.files[m.nav.fileCursor]
		for hi := range file.Hunks {
			hk := hunkKey(file.Path, hi)
			if anyExpanded {
				m.nav.collapsedHunks[hk] = true
			} else {
				delete(m.nav.collapsedHunks, hk)
			}
		}
	}

	m.nav.buildDiffLines(m.files)
	if m.nav.diffCursor >= len(m.nav.diffLines) {
		m.nav.diffCursor = len(m.nav.diffLines) - 1
	}
	if m.nav.diffCursor < 0 {
		m.nav.diffCursor = 0
	}
	m.updateDiffContent()
	return m
}

// moveToNextFile moves treeCursor to the next file row (skipping folders).
// Wraps around to the first file when at the end.
func (m *Model) moveToNextFile() tea.Cmd {
	n := len(m.nav.treeRows)
	start := m.nav.treeCursor
	for i := 1; i < n; i++ {
		idx := (start + i) % n
		if m.nav.treeRows[idx].node.fileIdx >= 0 {
			m.nav.treeCursor = idx
			m.onTreeCursorChanged()
			if idx <= start {
				return m.setFlash("Wrapped to first file")
			}
			return nil
		}
	}
	return nil
}

// moveToPrevFile moves treeCursor to the previous file row (skipping folders).
// Wraps around to the last file when at the beginning.
func (m *Model) moveToPrevFile() tea.Cmd {
	n := len(m.nav.treeRows)
	start := m.nav.treeCursor
	for i := 1; i < n; i++ {
		idx := (start - i + n) % n
		if m.nav.treeRows[idx].node.fileIdx >= 0 {
			m.nav.treeCursor = idx
			m.onTreeCursorChanged()
			if idx >= start {
				return m.setFlash("Wrapped to last file")
			}
			return nil
		}
	}
	return nil
}

func (m Model) handleDiffKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
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
		m.nav.diffCursor -= m.nav.diffViewport.Height() / 2
		if m.nav.diffCursor < 0 {
			m.nav.diffCursor = 0
		}
		if m.nav.visualMode {
			m.nav.visualEnd = m.nav.diffCursor
		}
		m.syncViewportToCursor()
	case key.Matches(msg, keys.PageDown):
		m.nav.diffCursor += m.nav.diffViewport.Height() / 2
		if m.nav.diffCursor >= len(m.nav.diffLines) {
			m.nav.diffCursor = len(m.nav.diffLines) - 1
		}
		if m.nav.visualMode {
			m.nav.visualEnd = m.nav.diffCursor
		}
		m.syncViewportToCursor()

	// Context-dependent Enter: create comment on empty line, open popup on commented line
	case key.Matches(msg, keys.Enter):
		if len(m.files) > 0 && m.nav.diffCursor < len(m.nav.diffLines) {
			path := m.files[m.nav.fileCursor].Path
			dl := m.nav.diffLines[m.nav.diffCursor]
			if m.lineHasComments(path, dl) {
				m.openCommentPopup()
				return m, nil
			}
		}
		// No comments on this line — create a new comment
		return m, m.startComment()

	// Dedicated navigation keys
	case key.Matches(msg, keys.NextHunk):
		m.nav.activeCycler = 'h'
		cmd := m.navigateHunk(true)
		return m, cmd
	case key.Matches(msg, keys.PrevHunk):
		m.nav.activeCycler = 'h'
		cmd := m.navigateHunk(false)
		return m, cmd
	case key.Matches(msg, keys.NextComment):
		m.nav.activeCycler = 'c'
		cmd := m.navigateComment(true, false)
		return m, cmd
	case key.Matches(msg, keys.PrevComment):
		m.nav.activeCycler = 'C'
		cmd := m.navigateComment(false, false)
		return m, cmd

	// Search
	case key.Matches(msg, keys.Search):
		m.search.active = true
		m.search.input = ""
		return m, nil
	case key.Matches(msg, keys.NextSearch):
		if m.search.query != "" {
			m.nav.activeCycler = '/'
			return m, m.jumpToNextSearchMatch()
		}
	case key.Matches(msg, keys.PrevSearch):
		if m.search.query != "" {
			m.nav.activeCycler = '/'
			return m, m.jumpToPrevSearchMatch()
		}

	case key.Matches(msg, keys.FilterFile):
		m.search.filterActive = true
		m.search.filterInput = ""
		m.search.filterFiles = m.allFileIndices()
		m.search.filterCursor = 0
		return m, nil
	case key.Matches(msg, keys.Help):
		m.showHelp = true
		return m, nil
	}

	return m, nil
}

// navigateFile moves to the next/prev file. If unviewedOnly, skip viewed files.
// Returns a tea.Cmd for flash messages when wrapping occurs.
func (m *Model) navigateFile(forward, unviewedOnly bool) tea.Cmd {
	n := len(m.files)
	if n == 0 {
		return nil
	}
	m.nav.pushJump()

	if m.nav.focus == FocusFileTree {
		// In tree view, delegate to tree navigation
		if unviewedOnly {
			return m.moveToUnviewedFileInTree(forward)
		}
		if forward {
			return m.moveToNextFile()
		}
		return m.moveToPrevFile()
	}

	// In diff view
	start := m.nav.fileCursor
	idx := cyclicSearch(start, n, forward, func(i int) bool {
		return !unviewedOnly || !m.review.ViewedFiles[m.files[i].Path]
	})
	if idx < 0 {
		return nil
	}
	oldIdx := m.nav.fileCursor
	m.nav.fileCursor = idx
	m.nav.buildDiffLines(m.files)
	m.nav.diffCursor = 0
	m.nav.diffViewport.GotoTop()
	m.updateDiffContent()
	m.autoFollowFile(oldIdx, m.nav.fileCursor)

	wrapped := (forward && idx <= start) || (!forward && idx >= start)
	if wrapped {
		label := "file"
		if unviewedOnly {
			label = "unviewed file"
		}
		if forward {
			return m.setFlash(fmt.Sprintf("Wrapped to first %s", label))
		}
		return m.setFlash(fmt.Sprintf("Wrapped to last %s", label))
	}
	return nil
}

// moveToUnviewedFileInTree moves treeCursor to the next/prev unviewed file row.
func (m *Model) moveToUnviewedFileInTree(forward bool) tea.Cmd {
	n := len(m.nav.treeRows)
	if n == 0 {
		return nil
	}
	start := m.nav.treeCursor
	idx := cyclicSearch(start, n, forward, func(i int) bool {
		row := m.nav.treeRows[i]
		return row.node.fileIdx >= 0 && !m.review.ViewedFiles[m.files[row.node.fileIdx].Path]
	})
	if idx < 0 {
		return nil
	}
	m.nav.treeCursor = idx
	m.onTreeCursorChanged()
	wrapped := (forward && idx <= start) || (!forward && idx >= start)
	if wrapped {
		if forward {
			return m.setFlash("Wrapped to first unviewed file")
		}
		return m.setFlash("Wrapped to last unviewed file")
	}
	return nil
}

// navigateComment moves to the next/prev comment. If crossFile, jump to next file with comments.
// When landing on a comment, expands the comment block and selects the first comment.
func (m *Model) navigateComment(forward, crossFile bool) tea.Cmd {
	m.nav.pushJump()
	if crossFile {
		return m.navigateCommentToFile(forward)
	}
	// Within current file: find next diff line that has comments
	if len(m.nav.diffLines) == 0 {
		return m.navigateCommentToFile(forward)
	}
	n := len(m.nav.diffLines)
	start := m.nav.diffCursor
	path := m.files[m.nav.fileCursor].Path
	idx := cyclicSearch(start, n, forward, func(i int) bool {
		return m.lineHasComments(path, m.nav.diffLines[i])
	})
	if idx >= 0 {
		m.nav.diffCursor = idx
		m.expandAndSelectComment(idx)
		m.syncViewportToCursorWithComments()
		wrapped := (forward && idx <= start) || (!forward && idx >= start)
		if wrapped {
			if forward {
				return m.setFlash("Wrapped to first comment")
			}
			return m.setFlash("Wrapped to last comment")
		}
		return nil
	}
	// No more comments in current file — try cross-file
	return m.navigateCommentToFile(forward)
}

// navigateCommentToFile jumps to the next/prev file that has comments,
// positions the cursor on the first/last commented line, and expands+selects it.
// Skips files where comments exist but don't map to any visible diff line.
func (m *Model) navigateCommentToFile(forward bool) tea.Cmd {
	nFiles := len(m.files)
	if nFiles == 0 {
		return nil
	}
	start := m.nav.fileCursor
	oldIdx := m.nav.fileCursor
	idx := cyclicSearch(start, nFiles, forward, func(i int) bool {
		if !m.fileHasComments(m.files[i].Path) {
			return false
		}
		// Temporarily switch to this file to check if comments map to diff lines
		m.nav.fileCursor = i
		m.nav.buildDiffLines(m.files)
		if m.findCommentedDiffLine(m.files[i].Path, forward) < 0 {
			// Comments exist but none map to visible diff lines; restore and skip
			m.nav.fileCursor = oldIdx
			m.nav.buildDiffLines(m.files)
			return false
		}
		return true
	})
	if idx < 0 {
		return m.setFlash("No comments found")
	}
	// m.nav.fileCursor and diffLines are already set to the found file by the match function
	m.nav.diffCursor = m.findCommentedDiffLine(m.files[idx].Path, forward)
	m.expandAndSelectComment(m.nav.diffCursor)
	m.nav.diffViewport.GotoTop()
	m.updateDiffContent()
	m.autoFollowFile(oldIdx, m.nav.fileCursor)
	m.syncViewportToCursorWithComments()
	wrapped := (forward && idx <= start) || (!forward && idx >= start)
	if wrapped {
		if forward {
			return m.setFlash("Wrapped to first commented file")
		}
		return m.setFlash("Wrapped to last commented file")
	}
	return nil
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
func (m *Model) navigateHunk(forward bool) tea.Cmd {
	m.nav.pushJump()
	if len(m.nav.diffLines) == 0 {
		return m.navigateHunkCrossFile(forward)
	}
	currentHunk := m.nav.diffLines[m.nav.diffCursor].hunkIdx

	if forward {
		for i := m.nav.diffCursor + 1; i < len(m.nav.diffLines); i++ {
			if m.nav.diffLines[i].hunkIdx != currentHunk {
				m.nav.diffCursor = i
				m.syncViewportToCursor()
				return nil
			}
		}
		// At last hunk — cross to next file
		return m.navigateHunkCrossFile(true)
	}

	// First, go to start of current hunk
	startOfCurrent := m.nav.diffCursor
	for startOfCurrent > 0 && m.nav.diffLines[startOfCurrent-1].hunkIdx == currentHunk {
		startOfCurrent--
	}
	if startOfCurrent < m.nav.diffCursor {
		// We weren't at the start — go there
		m.nav.diffCursor = startOfCurrent
		m.syncViewportToCursor()
		return nil
	}
	// Already at start — go to start of previous hunk
	if startOfCurrent > 0 {
		prevHunk := m.nav.diffLines[startOfCurrent-1].hunkIdx
		for i := startOfCurrent - 1; i >= 0; i-- {
			if i == 0 || m.nav.diffLines[i-1].hunkIdx != prevHunk {
				m.nav.diffCursor = i
				m.syncViewportToCursor()
				return nil
			}
		}
	}
	// At first hunk — cross to prev file
	return m.navigateHunkCrossFile(false)
}

// navigateHunkCrossFile moves to the next/prev file and positions at the
// first hunk (forward) or the last hunk's start (backward).
func (m *Model) navigateHunkCrossFile(forward bool) tea.Cmd {
	nFiles := len(m.files)
	if nFiles <= 1 {
		return nil
	}
	oldIdx := m.nav.fileCursor
	var nextIdx int
	if forward {
		nextIdx = (m.nav.fileCursor + 1) % nFiles
	} else {
		nextIdx = (m.nav.fileCursor - 1 + nFiles) % nFiles
	}
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
	wrapped := (forward && nextIdx <= oldIdx) || (!forward && nextIdx >= oldIdx)
	if wrapped {
		if forward {
			return m.setFlash("Wrapped to first file")
		}
		return m.setFlash("Wrapped to last file")
	}
	return nil
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

func (m Model) handleGotoKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
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
	case "esc", "ctrl+c":
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
	m.nav.pushJump()
	for i, dl := range m.nav.diffLines {
		if dl.newLine == lineNum || dl.oldLine == lineNum {
			m.nav.diffCursor = i
			m.syncViewportToCursor()
			return
		}
	}
}

// --- Search handling ---

func (m Model) handleSearchKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.search.query = m.search.input
		m.search.active = false
		var cmd tea.Cmd
		if m.search.query != "" {
			m.nav.activeCycler = '/'
			cmd = m.jumpToNextSearchMatch()
		}
		m.updateDiffContent()
		return m, cmd
	case "esc", "ctrl+c":
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
		if msg.Text != "" {
			m.search.input += msg.Text
		}
		return m, nil
	}
}

func (m *Model) jumpToNextSearchMatch() tea.Cmd {
	m.nav.pushJump()
	query := strings.ToLower(m.search.query)
	for i := m.nav.diffCursor + 1; i < len(m.nav.diffLines); i++ {
		if strings.Contains(strings.ToLower(m.nav.diffLines[i].content), query) {
			m.nav.diffCursor = i
			m.syncViewportToCursor()
			return nil
		}
	}
	// Wrap around
	for i := 0; i <= m.nav.diffCursor; i++ {
		if strings.Contains(strings.ToLower(m.nav.diffLines[i].content), query) {
			m.nav.diffCursor = i
			m.syncViewportToCursor()
			return m.setFlash("Wrapped to first match")
		}
	}
	return nil
}

func (m *Model) jumpToPrevSearchMatch() tea.Cmd {
	m.nav.pushJump()
	query := strings.ToLower(m.search.query)
	for i := m.nav.diffCursor - 1; i >= 0; i-- {
		if strings.Contains(strings.ToLower(m.nav.diffLines[i].content), query) {
			m.nav.diffCursor = i
			m.syncViewportToCursor()
			return nil
		}
	}
	// Wrap around
	for i := len(m.nav.diffLines) - 1; i >= m.nav.diffCursor; i-- {
		if strings.Contains(strings.ToLower(m.nav.diffLines[i].content), query) {
			m.nav.diffCursor = i
			m.syncViewportToCursor()
			return m.setFlash("Wrapped to last match")
		}
	}
	return nil
}

// --- Narrow regex filter handling ---

func (m Model) handleNarrowRegexKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.filter.setRegex(m.filter.regexInput)
		m.filter.regexActive = false
		m.applyFilters()
		return m, nil
	case "esc", "ctrl+c":
		m.filter.regexActive = false
		m.filter.regexInput = ""
		return m, nil
	case "backspace":
		if len(m.filter.regexInput) > 0 {
			m.filter.regexInput = m.filter.regexInput[:len(m.filter.regexInput)-1]
		}
		return m, nil
	default:
		if msg.Text != "" {
			m.filter.regexInput += msg.Text
		}
		return m, nil
	}
}

// --- Narrow prefix (T) handling ---

// handleNarrowPrefixKey handles the second key after pressing 'T' for filter commands.
func (m Model) handleNarrowPrefixKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	m.narrowPrefixActive = false
	switch msg.String() {
	case "o":
		return m.toggleOwnerFilter()
	case "f":
		m.filter.regexActive = true
		m.filter.regexInput = m.filter.regexPattern
		return m, nil
	case "x":
		return m.clearAllFilters()
	default:
		return m, nil
	}
}

// toggleOwnerFilter toggles the CODEOWNERS-based owner filter.
func (m Model) toggleOwnerFilter() (Model, tea.Cmd) {
	label := m.filter.toggleOwner()
	m.applyFilters()
	cmd := m.setFlash(fmt.Sprintf("Owner filter: %s", label))
	return m, cmd
}

// --- Jump list navigation ---

// jumpBack navigates to the previous position in the jump list.
func (m Model) jumpBack() (Model, tea.Cmd) {
	// Before first jump back, save current position so we can return to it
	if m.nav.jumpCursor == len(m.nav.jumpList)-1 || len(m.nav.jumpList) == 0 {
		m.nav.pushJump()
	}
	pos, ok := m.nav.jumpBack()
	if !ok {
		return m, m.setFlash("Already at oldest position")
	}
	m.applyJumpPos(pos)
	return m, nil
}

// jumpForward navigates to the next position in the jump list.
func (m Model) jumpForward() (Model, tea.Cmd) {
	pos, ok := m.nav.jumpForward()
	if !ok {
		return m, m.setFlash("Already at newest position")
	}
	m.applyJumpPos(pos)
	return m, nil
}

// applyJumpPos moves the cursor to the given jump position.
func (m *Model) applyJumpPos(pos jumpPos) {
	if pos.fileCursor != m.nav.fileCursor {
		oldIdx := m.nav.fileCursor
		m.nav.fileCursor = pos.fileCursor
		m.nav.buildDiffLines(m.files)
		m.autoFollowFile(oldIdx, m.nav.fileCursor)
	}
	m.nav.diffCursor = pos.diffCursor
	if m.nav.diffCursor >= len(m.nav.diffLines) {
		m.nav.diffCursor = len(m.nav.diffLines) - 1
	}
	if m.nav.diffCursor < 0 {
		m.nav.diffCursor = 0
	}
	m.updateDiffContent()
	m.syncViewportToCursor()
}

// clearAllFilters removes all narrowing filters.
func (m Model) clearAllFilters() (Model, tea.Cmd) {
	if !m.filter.isActive() {
		return m, nil
	}
	m.filter.clearAll()
	m.applyFilters()
	cmd := m.setFlash("Filters cleared")
	return m, cmd
}

// applyFilters recomputes the file filter and rebuilds the tree.
func (m *Model) applyFilters() {
	m.filter.recompute(m.files)
	m.nav.cachedTree = buildTree(m.files, m.filter.includedFiles)
	m.nav.rebuildTreeRows()

	// Ensure fileCursor points to an included file
	if !m.filter.isIncluded(m.nav.fileCursor) {
		// Find the first included file
		found := false
		for i := range m.files {
			if m.filter.isIncluded(i) {
				m.nav.fileCursor = i
				found = true
				break
			}
		}
		if !found {
			m.nav.fileCursor = 0
		}
		m.nav.buildDiffLines(m.files)
		m.nav.diffCursor = 0
	}

	m.nav.syncTreeCursorToFileCursor()
	m.treeDirty = true
	m.updateViewports()
	m.updateDiffContent()
}

// --- File filter handling ---

func (m Model) handleFilterKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
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
	case "esc", "ctrl+c":
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
		if msg.Text != "" {
			m.search.filterInput += msg.Text
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
		// Respect active narrowing filters
		if !m.filter.isIncluded(i) {
			continue
		}
		if query == "" || strings.Contains(strings.ToLower(f.Path), query) {
			m.search.filterFiles = append(m.search.filterFiles, i)
		}
	}
}

func (m Model) allFileIndices() []int {
	var indices []int
	for i := range m.files {
		if m.filter.isIncluded(i) {
			indices = append(indices, i)
		}
	}
	return indices
}

// --- Inline comment key handling ---

func (m Model) handleInlineKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
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

// --- PR info popup key handling ---

func (m Model) handlePRInfoKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "i", "ctrl+c":
		m.prInfoActive = false
		return m, nil
	}

	// Delegate scrolling to the viewport
	var cmd tea.Cmd
	m.prInfoViewport, cmd = m.prInfoViewport.Update(msg)
	return m, cmd
}

// --- Comment popup key handling ---

func (m Model) handleCommentPopupKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "enter", "ctrl+c":
		m.comments.popupActive = false
		return m, nil
	}

	// Delegate scrolling to the viewport (handles j/k, up/down, pgup/pgdn, etc.)
	var cmd tea.Cmd
	m.comments.popupViewport, cmd = m.comments.popupViewport.Update(msg)
	return m, cmd
}
