package diffview

import (
	"fmt"
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
		m.activateFileFilter()
		return m, nil
	case key.Matches(msg, keys.Help):
		m.showHelp = true
		return m, nil
	}

	return m, nil
}

// activateFileFilter opens the SearchBar in file filter mode.
func (m *Model) activateFileFilter() {
	fileNames := make([]string, len(m.files))
	for i, f := range m.files {
		fileNames[i] = f.Path
	}
	m.search.ActivateFilter(fileNames, m.filter.isIncluded)
}

// onTreeCursorChanged updates fileCursor if on a file row and syncs viewports.
func (m *Model) onTreeCursorChanged() {
	if m.nav.treeCursor >= 0 && m.nav.treeCursor < len(m.nav.treeRows) {
		row := m.nav.treeRows[m.nav.treeCursor]
		if row.node.fileIdx >= 0 {
			m.nav.fileCursor = row.node.fileIdx
			m.nav.buildDiffLines(m.files)
			m.nav.cursor = CursorTarget{Kind: CursorLine}
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
		m.nav.cursor = CursorTarget{Kind: CursorLine}
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
func (m Model) toggleFoldAtDiffCursor() Model {
	if len(m.files) == 0 || m.nav.fileCursor >= len(m.files) {
		return m
	}
	if m.nav.cursor.LineIdx >= len(m.nav.diffLines) {
		return m
	}

	if m.nav.cursor.IsComment() {
		return m.toggleCommentAtCursor()
	}
	return m.toggleHunkAtCursor()
}

// toggleHunkAtCursor collapses or expands the hunk at the current cursor position.
func (m Model) toggleHunkAtCursor() Model {
	dl := m.nav.diffLines[m.nav.cursor.LineIdx]
	path := m.files[m.nav.fileCursor].Path
	hk := hunkKey(path, dl.hunkIdx)

	if dl.collapsed {
		delete(m.nav.collapsedHunks, hk)
	} else {
		m.nav.collapsedHunks[hk] = true
	}

	m.nav.buildDiffLines(m.files)
	if m.nav.cursor.LineIdx >= len(m.nav.diffLines) {
		m.nav.cursor.LineIdx = len(m.nav.diffLines) - 1
	}
	m.updateDiffContent()
	return m
}

// toggleAllFolds toggles all comment folds and hunk folds for the current file.
func (m Model) toggleAllFolds() Model {
	m.nav.cursor = m.nav.cursor.AsLine()

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
	for _, c := range m.comments.comments {
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
	if m.nav.cursor.LineIdx >= len(m.nav.diffLines) {
		m.nav.cursor.LineIdx = len(m.nav.diffLines) - 1
	}
	if m.nav.cursor.LineIdx < 0 {
		m.nav.cursor.LineIdx = 0
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
		if m.nav.cursor.LineIdx > 0 {
			m.nav.cursor.LineIdx--
			if m.nav.visualMode {
				m.nav.visualEnd = m.nav.cursor.LineIdx
			}
			m.syncViewportToCursor()
		}
	case key.Matches(msg, keys.Down):
		if m.nav.cursor.LineIdx < len(m.nav.diffLines)-1 {
			m.nav.cursor.LineIdx++
			if m.nav.visualMode {
				m.nav.visualEnd = m.nav.cursor.LineIdx
			}
			m.syncViewportToCursor()
		}
	case key.Matches(msg, keys.PageUp):
		m.nav.cursor.LineIdx -= m.nav.diffViewport.Height() / 2
		if m.nav.cursor.LineIdx < 0 {
			m.nav.cursor.LineIdx = 0
		}
		if m.nav.visualMode {
			m.nav.visualEnd = m.nav.cursor.LineIdx
		}
		m.syncViewportToCursor()
	case key.Matches(msg, keys.PageDown):
		m.nav.cursor.LineIdx += m.nav.diffViewport.Height() / 2
		if m.nav.cursor.LineIdx >= len(m.nav.diffLines) {
			m.nav.cursor.LineIdx = len(m.nav.diffLines) - 1
		}
		if m.nav.visualMode {
			m.nav.visualEnd = m.nav.cursor.LineIdx
		}
		m.syncViewportToCursor()

	// Context-dependent Enter: enter comment select on commented line, create comment on empty line
	case key.Matches(msg, keys.Enter):
		if len(m.files) > 0 && m.nav.cursor.LineIdx < len(m.nav.diffLines) {
			path := m.files[m.nav.fileCursor].Path
			dl := m.nav.diffLines[m.nav.cursor.LineIdx]
			if m.comments.LineHasComments(path, dl) {
				// Expand the comment block and enter comment select mode
				if dl.newLine > 0 {
					m.comments.expanded[commentKey(path, dl.newLine)] = true
				}
				if dl.oldLine > 0 {
					m.comments.expanded[commentKey(path, dl.oldLine)] = true
				}
				m.nav.cursor = CursorTarget{Kind: CursorComment, LineIdx: m.nav.cursor.LineIdx, CommentIdx: 0}
				m.updateDiffContent()
				return m, nil
			}
		}
		// No comments on this line — create a new comment
		return m, m.startComment()

	// Dedicated navigation keys
	case key.Matches(msg, keys.NextHunk):
		m.nav.activeCycler = CyclerHunk
		cmd := m.navigateHunk(true)
		return m, cmd
	case key.Matches(msg, keys.PrevHunk):
		m.nav.activeCycler = CyclerHunk
		cmd := m.navigateHunk(false)
		return m, cmd
	case key.Matches(msg, keys.NextComment):
		m.nav.activeCycler = CyclerComment
		cmd := m.navigateComment(true, false)
		return m, cmd
	case key.Matches(msg, keys.PrevComment):
		m.nav.activeCycler = CyclerComment
		cmd := m.navigateComment(false, false)
		return m, cmd

	// Search
	case key.Matches(msg, keys.Search):
		m.search.ActivateSearch()
		return m, nil
	case key.Matches(msg, keys.NextSearch):
		if m.search.Query() != "" {
			m.nav.activeCycler = CyclerSearch
			return m, m.jumpToNextSearchMatch()
		}
	case key.Matches(msg, keys.PrevSearch):
		if m.search.Query() != "" {
			m.nav.activeCycler = CyclerSearch
			return m, m.jumpToPrevSearchMatch()
		}

	case key.Matches(msg, keys.FilterFile):
		m.activateFileFilter()
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
		if !m.filter.isIncluded(i) {
			return false
		}
		return !unviewedOnly || !m.pendingReview.ViewedFiles[m.files[i].Path]
	})
	if idx < 0 {
		return nil
	}
	oldIdx := m.nav.fileCursor
	m.nav.fileCursor = idx
	m.nav.buildDiffLines(m.files)
	m.nav.cursor = CursorTarget{Kind: CursorLine}
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
		return row.node.fileIdx >= 0 && !m.pendingReview.ViewedFiles[m.files[row.node.fileIdx].Path]
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
	start := m.nav.cursor.LineIdx
	path := m.files[m.nav.fileCursor].Path
	idx := cyclicSearch(start, n, forward, func(i int) bool {
		return m.comments.LineHasComments(path, m.nav.diffLines[i])
	})
	if idx >= 0 {
		m.nav.cursor.LineIdx = idx
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
		if !m.filter.isIncluded(i) {
			return false
		}
		if !m.comments.FileHasComments(m.files[i].Path) {
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
	m.nav.cursor.LineIdx = m.findCommentedDiffLine(m.files[idx].Path, forward)
	m.expandAndSelectComment(m.nav.cursor.LineIdx)
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

// expandAndSelectComment expands the comment block at the given line index and enters comment select mode.
func (m *Model) expandAndSelectComment(lineIdx int) {
	if lineIdx < 0 || lineIdx >= len(m.nav.diffLines) || len(m.files) == 0 {
		return
	}
	dl := m.nav.diffLines[lineIdx]
	path := m.files[m.nav.fileCursor].Path
	if dl.newLine > 0 {
		m.comments.expanded[commentKey(path, dl.newLine)] = true
	}
	if dl.oldLine > 0 {
		m.comments.expanded[commentKey(path, dl.oldLine)] = true
	}
	m.nav.cursor = CursorTarget{Kind: CursorComment, LineIdx: lineIdx, CommentIdx: 0}
}

// findCommentedDiffLine returns the index of the first (forward=true) or last (forward=false)
// diff line that has comments for the given path. Returns -1 if none found.
func (m *Model) findCommentedDiffLine(path string, forward bool) int {
	if forward {
		for j := 0; j < len(m.nav.diffLines); j++ {
			if m.comments.LineHasComments(path, m.nav.diffLines[j]) {
				return j
			}
		}
	} else {
		for j := len(m.nav.diffLines) - 1; j >= 0; j-- {
			if m.comments.LineHasComments(path, m.nav.diffLines[j]) {
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
	currentHunk := m.nav.diffLines[m.nav.cursor.LineIdx].hunkIdx

	if forward {
		for i := m.nav.cursor.LineIdx + 1; i < len(m.nav.diffLines); i++ {
			if m.nav.diffLines[i].hunkIdx != currentHunk {
				m.nav.cursor.LineIdx = i
				m.syncViewportToCursor()
				return nil
			}
		}
		// At last hunk — cross to next file
		return m.navigateHunkCrossFile(true)
	}

	// First, go to start of current hunk
	startOfCurrent := m.nav.cursor.LineIdx
	for startOfCurrent > 0 && m.nav.diffLines[startOfCurrent-1].hunkIdx == currentHunk {
		startOfCurrent--
	}
	if startOfCurrent < m.nav.cursor.LineIdx {
		// We weren't at the start — go there
		m.nav.cursor.LineIdx = startOfCurrent
		m.syncViewportToCursor()
		return nil
	}
	// Already at start — go to start of previous hunk
	if startOfCurrent > 0 {
		prevHunk := m.nav.diffLines[startOfCurrent-1].hunkIdx
		for i := startOfCurrent - 1; i >= 0; i-- {
			if i == 0 || m.nav.diffLines[i-1].hunkIdx != prevHunk {
				m.nav.cursor.LineIdx = i
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
	nextIdx := cyclicSearch(m.nav.fileCursor, nFiles, forward, func(i int) bool {
		return m.filter.isIncluded(i)
	})
	if nextIdx < 0 {
		return nil
	}
	m.nav.fileCursor = nextIdx
	m.nav.buildDiffLines(m.files)
	if forward {
		m.nav.cursor = CursorTarget{Kind: CursorLine}
	} else {
		// Go to last hunk's start
		if len(m.nav.diffLines) > 0 {
			lastHunk := m.nav.diffLines[len(m.nav.diffLines)-1].hunkIdx
			for i := len(m.nav.diffLines) - 1; i >= 0; i-- {
				if i == 0 || m.nav.diffLines[i-1].hunkIdx != lastHunk {
					m.nav.cursor.LineIdx = i
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

// --- SearchBar delegation ---

// handleSearchBarKey delegates key events to the SearchBar component and
// handles any outbound messages it produces.
func (m Model) handleSearchBarKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	s, outMsg := m.search.HandleKey(msg.String(), msg.Text)
	m.search = s

	switch ev := outMsg.(type) {
	case searchGotoLineMsg:
		m.jumpToLine(ev.line)
	case searchQueryConfirmedMsg:
		m.nav.activeCycler = CyclerSearch
		cmd := m.jumpToNextSearchMatch()
		m.updateDiffContent()
		return m, cmd
	case searchDismissedMsg:
		m.updateDiffContent()
	case searchFilterSelectedMsg:
		oldIdx := m.nav.fileCursor
		m.nav.fileCursor = ev.fileIdx
		m.nav.buildDiffLines(m.files)
		m.nav.cursor = CursorTarget{Kind: CursorLine}
		m.nav.diffViewport.GotoTop()
		m.updateDiffContent()
		m.autoFollowFile(oldIdx, m.nav.fileCursor)
	case searchFilterDismissedMsg:
		// nothing extra needed
	}

	return m, nil
}

func (m *Model) jumpToLine(lineNum int) {
	m.nav.pushJump()
	for i, dl := range m.nav.diffLines {
		if dl.newLine == lineNum || dl.oldLine == lineNum {
			m.nav.cursor = CursorTarget{Kind: CursorLine, LineIdx: i}
			m.syncViewportToCursor()
			return
		}
	}
}

// --- Search match navigation (used by parent for n/N keys) ---

func (m *Model) jumpToNextSearchMatch() tea.Cmd {
	m.nav.pushJump()
	query := strings.ToLower(m.search.Query())
	for i := m.nav.cursor.LineIdx + 1; i < len(m.nav.diffLines); i++ {
		if strings.Contains(strings.ToLower(m.nav.diffLines[i].content), query) {
			m.nav.cursor.LineIdx = i
			m.syncViewportToCursor()
			return nil
		}
	}
	// Wrap around
	for i := 0; i <= m.nav.cursor.LineIdx; i++ {
		if strings.Contains(strings.ToLower(m.nav.diffLines[i].content), query) {
			m.nav.cursor.LineIdx = i
			m.syncViewportToCursor()
			return m.setFlash("Wrapped to first match")
		}
	}
	return nil
}

func (m *Model) jumpToPrevSearchMatch() tea.Cmd {
	m.nav.pushJump()
	query := strings.ToLower(m.search.Query())
	for i := m.nav.cursor.LineIdx - 1; i >= 0; i-- {
		if strings.Contains(strings.ToLower(m.nav.diffLines[i].content), query) {
			m.nav.cursor.LineIdx = i
			m.syncViewportToCursor()
			return nil
		}
	}
	// Wrap around
	for i := len(m.nav.diffLines) - 1; i >= m.nav.cursor.LineIdx; i-- {
		if strings.Contains(strings.ToLower(m.nav.diffLines[i].content), query) {
			m.nav.cursor.LineIdx = i
			m.syncViewportToCursor()
			return m.setFlash("Wrapped to last match")
		}
	}
	return nil
}

// --- Narrow regex filter handling ---

// handleNarrowRegexKey delegates to the FileFilter component and handles
// any outbound messages.
func (m Model) handleNarrowRegexKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	ff, outMsg := m.filter.HandleRegexKey(msg.String(), msg.Text)
	m.filter = ff

	switch outMsg.(type) {
	case filterRegexAppliedMsg:
		m.applyFilters()
	}

	return m, nil
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
	// Skip jumps to files excluded by active filters.
	if !m.filter.isIncluded(pos.fileCursor) {
		return
	}
	if pos.fileCursor != m.nav.fileCursor {
		oldIdx := m.nav.fileCursor
		m.nav.fileCursor = pos.fileCursor
		m.nav.buildDiffLines(m.files)
		m.autoFollowFile(oldIdx, m.nav.fileCursor)
	}
	m.nav.cursor = pos.cursor
	if m.nav.cursor.LineIdx >= len(m.nav.diffLines) {
		m.nav.cursor.LineIdx = len(m.nav.diffLines) - 1
	}
	if m.nav.cursor.LineIdx < 0 {
		m.nav.cursor.LineIdx = 0
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
		m.nav.cursor = CursorTarget{Kind: CursorLine}
	}

	m.nav.syncTreeCursorToFileCursor()
	m.treeDirty = true
	m.updateViewports()
	m.updateDiffContent()
}

// --- File filter handling ---


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

// handleInlineEditorKey delegates key events to the InlineEditor and handles
// any outbound messages.
func (m Model) handleInlineEditorKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	e, cmd, outMsg := m.editor.HandleKey(msg)
	m.editor = e

	switch msg := outMsg.(type) {
	case inlineEditorSaveMsg:
		return m.handleEditorSave(msg)
	case inlineEditorCancelMsg:
		m.closeInlineComment()
		return m, nil
	case inlineEditorOpenEditorMsg:
		return m, m.openExternalEditorForComment()
	case inlineEditorPasteImageMsg:
		return m, checkClipboardImageCmd
	}

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
