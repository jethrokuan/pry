package diffview

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/jethrokuan/pry/internal/review"
	"github.com/jethrokuan/pry/internal/ui/components/flash"
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

// onTreeCursorChanged updates cursor.FileIdx if on a file row and syncs viewports.
func (m *Model) onTreeCursorChanged() {
	if m.nav.treeCursor >= 0 && m.nav.treeCursor < len(m.nav.treeRows) {
		row := m.nav.treeRows[m.nav.treeCursor]
		if row.node.fileIdx >= 0 {
			m.nav.cursor = CursorTarget{Kind: CursorLine, FileIdx: row.node.fileIdx}
			m.nav.buildDiffLines(m.files)
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
		m.nav.cursor = CursorTarget{Kind: CursorLine, FileIdx: m.nav.cursor.FileIdx}
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
	if len(m.files) == 0 || m.nav.cursor.FileIdx >= len(m.files) {
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
	path := m.files[m.nav.cursor.FileIdx].Path
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
	if len(m.files) > 0 && m.nav.cursor.FileIdx < len(m.files) {
		file := m.files[m.nav.cursor.FileIdx]
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
	for _, t := range m.comments.threads {
		allCommentKeys[commentKey(t.Path, t.Line)] = true
	}
	for ck := range allCommentKeys {
		m.comments.expanded[ck] = !anyExpanded
	}

	// Toggle all hunks for current file
	if len(m.files) > 0 && m.nav.cursor.FileIdx < len(m.files) {
		file := m.files[m.nav.cursor.FileIdx]
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
				return flash.ShowMsg{ID: "diffview", Text: "Wrapped to first file", Expires: 1500 * time.Millisecond}.Cmd()
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
				return flash.ShowMsg{ID: "diffview", Text: "Wrapped to last file", Expires: 1500 * time.Millisecond}.Cmd()
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
				m.nav.visualEnd = m.nav.cursor
			}
			m.syncViewportToCursor()
		}
	case key.Matches(msg, keys.Down):
		if m.nav.cursor.LineIdx < len(m.nav.diffLines)-1 {
			m.nav.cursor.LineIdx++
			if m.nav.visualMode {
				m.nav.visualEnd = m.nav.cursor
			}
			m.syncViewportToCursor()
		}
	case key.Matches(msg, keys.PageUp):
		m.nav.cursor.LineIdx -= m.nav.diffViewport.Height() / 2
		if m.nav.cursor.LineIdx < 0 {
			m.nav.cursor.LineIdx = 0
		}
		if m.nav.visualMode {
			m.nav.visualEnd = m.nav.cursor
		}
		m.syncViewportToCursor()
	case key.Matches(msg, keys.PageDown):
		m.nav.cursor.LineIdx += m.nav.diffViewport.Height() / 2
		if m.nav.cursor.LineIdx >= len(m.nav.diffLines) {
			m.nav.cursor.LineIdx = len(m.nav.diffLines) - 1
		}
		if m.nav.visualMode {
			m.nav.visualEnd = m.nav.cursor
		}
		m.syncViewportToCursor()

	// Context-dependent Enter: enter comment select on commented line, create comment on empty line
	case key.Matches(msg, keys.Enter):
		if len(m.files) > 0 && m.nav.cursor.LineIdx < len(m.nav.diffLines) {
			path := m.files[m.nav.cursor.FileIdx].Path
			dl := m.nav.diffLines[m.nav.cursor.LineIdx]
			if m.comments.LineHasComments(path, dl) {
				// Expand the comment block and enter comment select mode
				if dl.newLine > 0 {
					m.comments.expanded[commentKey(path, dl.newLine)] = true
				}
				if dl.oldLine > 0 {
					m.comments.expanded[commentKey(path, dl.oldLine)] = true
				}
				m.nav.cursor = CursorTarget{Kind: CursorComment, FileIdx: m.nav.cursor.FileIdx, LineIdx: m.nav.cursor.LineIdx, ThreadIdx: 0, CommentIdx: 0}
				m.updateDiffContent()
				return m, nil
			}
		}
		// No comments on this line — create a new comment
		return m, m.startComment(commentModeComment)

	case key.Matches(msg, keys.Suggest):
		return m, m.startComment(commentModeSuggestion)

	// Dedicated navigation keys
	case key.Matches(msg, keys.NextHunk):
		m.nav.activeCycler = CyclerHunk
		cmd := m.navigateHunk(true)
		return m, cmd
	case key.Matches(msg, keys.PrevHunk):
		m.nav.activeCycler = CyclerHunk
		cmd := m.navigateHunk(false)
		return m, cmd
	case key.Matches(msg, keys.NextThread):
		m.nav.activeCycler = CyclerThread
		cmd := m.navigateThread(true)
		return m, cmd
	case key.Matches(msg, keys.PrevThread):
		m.nav.activeCycler = CyclerThread
		cmd := m.navigateThread(false)
		return m, cmd
	case key.Matches(msg, keys.NextComment):
		m.nav.activeCycler = CyclerComment
		cmd := m.navigateComment(true)
		return m, cmd
	case key.Matches(msg, keys.PrevComment):
		m.nav.activeCycler = CyclerComment
		cmd := m.navigateComment(false)
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

// navigateFile moves to the next/prev file.
// Returns a tea.Cmd for flash messages when wrapping occurs.
func (m *Model) navigateFile(forward bool) tea.Cmd {
	n := len(m.files)
	if n == 0 {
		return nil
	}
	m.nav.pushJump()

	if m.nav.focus == FocusFileTree {
		if forward {
			return m.moveToNextFile()
		}
		return m.moveToPrevFile()
	}

	// In diff view
	start := m.nav.cursor.FileIdx
	idx := cyclicSearch(start, n, forward, func(i int) bool {
		return m.filter.isIncluded(i)
	})
	if idx < 0 {
		return nil
	}
	oldIdx := m.nav.cursor.FileIdx
	m.nav.cursor = CursorTarget{Kind: CursorLine, FileIdx: idx}
	m.nav.buildDiffLines(m.files)
	m.nav.diffViewport.GotoTop()
	m.updateDiffContent()
	m.autoFollowFile(oldIdx, idx)

	wrapped := (forward && idx <= start) || (!forward && idx >= start)
	if wrapped {
		if forward {
			return flash.ShowMsg{ID: "diffview", Text: "Wrapped to first file", Expires: 1500 * time.Millisecond}.Cmd()
		}
		return flash.ShowMsg{ID: "diffview", Text: "Wrapped to last file", Expires: 1500 * time.Millisecond}.Cmd()
	}
	return nil
}

// threadPosition identifies a thread's location in the diff view for navigation.
type threadPosition struct {
	fileIdx int
	line    int
	side    string
}

// buildThreadPositions returns all thread positions sorted by file index and line,
// mapping each thread to the diff line where it would render.
func (m *Model) buildThreadPositions() []threadPosition {
	// Build a file-path-to-index map
	fileIdxByPath := make(map[string]int, len(m.files))
	for i, f := range m.files {
		fileIdxByPath[f.Path] = i
	}

	var positions []threadPosition
	seen := make(map[threadPosition]bool)
	for _, t := range m.comments.threads {
		fi, ok := fileIdxByPath[t.Path]
		if !ok {
			continue
		}
		pos := threadPosition{fileIdx: fi, line: t.Line, side: t.Side}
		if !seen[pos] {
			seen[pos] = true
			positions = append(positions, pos)
		}
	}

	// Sort by file index, then line number
	sort.Slice(positions, func(i, j int) bool {
		if positions[i].fileIdx != positions[j].fileIdx {
			return positions[i].fileIdx < positions[j].fileIdx
		}
		return positions[i].line < positions[j].line
	})
	return positions
}

// diffLineForThread finds the diff line index that matches a thread position
// in the current file's diff lines. Returns -1 if not found.
func (m *Model) diffLineForThread(pos threadPosition) int {
	for i, dl := range m.nav.diffLines {
		if pos.side == "RIGHT" && dl.newLine == pos.line {
			return i
		}
		if pos.side == "LEFT" && dl.oldLine == pos.line {
			return i
		}
		// Empty side: match either
		if pos.side == "" && (dl.newLine == pos.line || dl.oldLine == pos.line) {
			return i
		}
	}
	return -1
}

// expandHunkForThread finds the collapsed hunk containing the thread's line
// and expands it. Returns true if a hunk was expanded.
func (m *Model) expandHunkForThread(pos threadPosition) bool {
	if pos.fileIdx >= len(m.files) {
		return false
	}
	file := m.files[pos.fileIdx]
	for hi, hunk := range file.Hunks {
		hk := hunkKey(file.Path, hi)
		if !m.nav.collapsedHunks[hk] {
			continue
		}
		inRange := false
		if pos.side == "RIGHT" || pos.side == "" {
			inRange = pos.line >= hunk.NewStart && pos.line < hunk.NewStart+hunk.NewLines
		}
		if !inRange && (pos.side == "LEFT" || pos.side == "") {
			inRange = pos.line >= hunk.OldStart && pos.line < hunk.OldStart+hunk.OldLines
		}
		if inRange {
			delete(m.nav.collapsedHunks, hk)
			return true
		}
	}
	return false
}

// navigateThread moves to the next/prev thread across all files.
// When landing on a thread, expands the comment block and selects the first comment.
func (m *Model) navigateThread(forward bool) tea.Cmd {
	m.nav.pushJump()

	positions := m.buildThreadPositions()
	if len(positions) == 0 {
		return flash.ShowMsg{ID: "diffview", Text: "No threads found", Expires: 1500 * time.Millisecond}.Cmd()
	}

	// Find current position in the sorted list
	curFileIdx := m.nav.cursor.FileIdx
	curLine := 0
	if m.nav.cursor.LineIdx < len(m.nav.diffLines) {
		dl := m.nav.diffLines[m.nav.cursor.LineIdx]
		if dl.newLine > 0 {
			curLine = dl.newLine
		} else {
			curLine = dl.oldLine
		}
	}

	// Find the next/prev thread position
	var target *threadPosition
	wrapped := false

	if forward {
		for i := range positions {
			if positions[i].fileIdx > curFileIdx || (positions[i].fileIdx == curFileIdx && positions[i].line > curLine) {
				target = &positions[i]
				break
			}
		}
		if target == nil {
			target = &positions[0]
			wrapped = true
		}
	} else {
		for i := len(positions) - 1; i >= 0; i-- {
			if positions[i].fileIdx < curFileIdx || (positions[i].fileIdx == curFileIdx && positions[i].line < curLine) {
				target = &positions[i]
				break
			}
		}
		if target == nil {
			target = &positions[len(positions)-1]
			wrapped = true
		}
	}

	// Switch to the target file if needed
	if target.fileIdx != m.nav.cursor.FileIdx {
		oldIdx := m.nav.cursor.FileIdx
		m.nav.cursor.FileIdx = target.fileIdx
		m.nav.buildDiffLines(m.files)
		m.nav.diffViewport.GotoTop()
		m.autoFollowFile(oldIdx, target.fileIdx)
	}

	// Find the diff line for this thread, expanding collapsed hunks if needed
	diffIdx := m.diffLineForThread(*target)
	if diffIdx < 0 {
		// Thread's line not visible — try expanding the collapsed hunk that contains it
		if m.expandHunkForThread(*target) {
			m.nav.buildDiffLines(m.files)
			diffIdx = m.diffLineForThread(*target)
		}
		if diffIdx < 0 {
			m.updateDiffContent()
			return flash.ShowMsg{ID: "diffview", Text: "Thread not visible in diff", Expires: 1500 * time.Millisecond}.Cmd()
		}
	}

	m.nav.cursor.LineIdx = diffIdx
	m.expandAndSelectComment(diffIdx, 0, -1)
	m.updateDiffContent()
	m.syncViewportToCursorWithComments()

	if wrapped {
		dir := "first"
		if !forward {
			dir = "last"
		}
		return flash.ShowMsg{ID: "diffview", Text: "Wrapped to " + dir + " thread", Expires: 1500 * time.Millisecond}.Cmd()
	}
	return nil
}

// expandAndSelectComment expands the comment block at the given line index and enters comment select mode.
// threadIdx selects which thread; commentIdx selects which comment within the thread (-1 means last).
func (m *Model) expandAndSelectComment(lineIdx int, threadIdx int, commentIdx int) {
	if lineIdx < 0 || lineIdx >= len(m.nav.diffLines) || len(m.files) == 0 {
		return
	}
	dl := m.nav.diffLines[lineIdx]
	path := m.files[m.nav.cursor.FileIdx].Path
	if dl.newLine > 0 {
		m.comments.expanded[commentKey(path, dl.newLine)] = true
	}
	if dl.oldLine > 0 {
		m.comments.expanded[commentKey(path, dl.oldLine)] = true
	}
	threads := m.threadsAtLine(lineIdx)
	if threadIdx < 0 || threadIdx >= len(threads) {
		threadIdx = 0
	}
	if commentIdx < 0 && threadIdx < len(threads) {
		commentIdx = len(threads[threadIdx].Comments) - 1
		if commentIdx < 0 {
			commentIdx = 0
		}
	}
	m.nav.cursor = CursorTarget{Kind: CursorComment, FileIdx: m.nav.cursor.FileIdx, LineIdx: lineIdx, ThreadIdx: threadIdx, CommentIdx: commentIdx}
}

// commentPosition identifies a single comment's location across all files for flat navigation.
type commentPosition struct {
	fileIdx    int
	line       int
	side       string
	threadIdx  int // index into threads at this line (cross-side order)
	commentIdx int // index within the thread's Comments slice
}

// buildCommentPositions returns all individual comment positions across all files,
// sorted by file index, line, then thread/comment index.
// ThreadIdx is the cross-side index (RIGHT threads first, then LEFT) matching threadsAtLine.
func (m *Model) buildCommentPositions() []commentPosition {
	fileIdxByPath := make(map[string]int, len(m.files))
	for i, f := range m.files {
		fileIdxByPath[f.Path] = i
	}

	// Group threads by (file, line) to compute cross-side threadIdx.
	type locKey struct {
		fileIdx int
		line    int
	}
	// Collect unique locations in sorted order.
	locThreads := make(map[locKey][]review.Thread)
	for _, t := range m.comments.threads {
		fi, ok := fileIdxByPath[t.Path]
		if !ok {
			continue
		}
		lk := locKey{fi, t.Line}
		locThreads[lk] = append(locThreads[lk], t)
	}

	var positions []commentPosition
	for lk, threads := range locThreads {
		// Sort threads in cross-side order: RIGHT first, then LEFT (matching threadsAtLine).
		sort.SliceStable(threads, func(i, j int) bool {
			if threads[i].Side == "RIGHT" && threads[j].Side != "RIGHT" {
				return true
			}
			if threads[i].Side != "RIGHT" && threads[j].Side == "RIGHT" {
				return false
			}
			return false
		})
		for ti, t := range threads {
			for ci := range t.Comments {
				positions = append(positions, commentPosition{
					fileIdx:    lk.fileIdx,
					line:       lk.line,
					side:       t.Side,
					threadIdx:  ti,
					commentIdx: ci,
				})
			}
		}
	}

	// Sort by file index, then line, then threadIdx, then commentIdx.
	sort.Slice(positions, func(i, j int) bool {
		if positions[i].fileIdx != positions[j].fileIdx {
			return positions[i].fileIdx < positions[j].fileIdx
		}
		if positions[i].line != positions[j].line {
			return positions[i].line < positions[j].line
		}
		if positions[i].threadIdx != positions[j].threadIdx {
			return positions[i].threadIdx < positions[j].threadIdx
		}
		return positions[i].commentIdx < positions[j].commentIdx
	})

	return positions
}

// navigateComment moves to the next/prev individual comment across all files.
// Unlike navigateThread which jumps between threads, this walks through every
// comment including within multi-comment threads.
func (m *Model) navigateComment(forward bool) tea.Cmd {
	m.nav.pushJump()

	positions := m.buildCommentPositions()
	if len(positions) == 0 {
		return flash.ShowMsg{ID: "diffview", Text: "No comments found", Expires: 1500 * time.Millisecond}.Cmd()
	}

	// Find current position in the list
	curFileIdx := m.nav.cursor.FileIdx
	curLine := 0
	if m.nav.cursor.LineIdx < len(m.nav.diffLines) {
		dl := m.nav.diffLines[m.nav.cursor.LineIdx]
		if dl.newLine > 0 {
			curLine = dl.newLine
		} else {
			curLine = dl.oldLine
		}
	}
	curThreadIdx := m.nav.cursor.ThreadIdx
	curCommentIdx := 0
	if m.nav.cursor.IsComment() {
		curCommentIdx = m.nav.cursor.CommentIdx
	}

	curIdx := -1
	for i, p := range positions {
		if p.fileIdx == curFileIdx && p.line == curLine && p.threadIdx == curThreadIdx && p.commentIdx == curCommentIdx {
			curIdx = i
			break
		}
	}

	var targetIdx int
	wrapped := false

	if forward {
		if curIdx >= 0 && curIdx < len(positions)-1 {
			targetIdx = curIdx + 1
		} else {
			targetIdx = 0
			wrapped = curIdx >= 0
		}
	} else {
		if curIdx > 0 {
			targetIdx = curIdx - 1
		} else {
			targetIdx = len(positions) - 1
			wrapped = curIdx >= 0 || curIdx == 0
		}
	}

	target := positions[targetIdx]

	// Switch file if needed
	if target.fileIdx != m.nav.cursor.FileIdx {
		oldIdx := m.nav.cursor.FileIdx
		m.nav.cursor.FileIdx = target.fileIdx
		m.nav.buildDiffLines(m.files)
		m.nav.diffViewport.GotoTop()
		m.autoFollowFile(oldIdx, target.fileIdx)
	}

	// Find the diff line for this comment
	tp := threadPosition{fileIdx: target.fileIdx, line: target.line, side: target.side}
	diffIdx := m.diffLineForThread(tp)
	if diffIdx < 0 {
		if m.expandHunkForThread(tp) {
			m.nav.buildDiffLines(m.files)
			diffIdx = m.diffLineForThread(tp)
		}
		if diffIdx < 0 {
			m.updateDiffContent()
			return flash.ShowMsg{ID: "diffview", Text: "Comment not visible in diff", Expires: 1500 * time.Millisecond}.Cmd()
		}
	}

	m.nav.cursor.LineIdx = diffIdx
	m.expandAndSelectComment(diffIdx, target.threadIdx, target.commentIdx)
	m.updateDiffContent()
	m.syncViewportToCursorWithComments()

	if wrapped {
		dir := "first"
		if !forward {
			dir = "last"
		}
		return flash.ShowMsg{ID: "diffview", Text: "Wrapped to " + dir + " comment", Expires: 1500 * time.Millisecond}.Cmd()
	}
	return nil
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
	oldIdx := m.nav.cursor.FileIdx
	nextIdx := cyclicSearch(m.nav.cursor.FileIdx, nFiles, forward, func(i int) bool {
		return m.filter.isIncluded(i)
	})
	if nextIdx < 0 {
		return nil
	}
	m.nav.cursor = CursorTarget{Kind: CursorLine, FileIdx: nextIdx}
	m.nav.buildDiffLines(m.files)
	if !forward {
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
	m.autoFollowFile(oldIdx, m.nav.cursor.FileIdx)
	m.syncViewportToCursor()
	wrapped := (forward && nextIdx <= oldIdx) || (!forward && nextIdx >= oldIdx)
	if wrapped {
		if forward {
			return flash.ShowMsg{ID: "diffview", Text: "Wrapped to first file", Expires: 1500 * time.Millisecond}.Cmd()
		}
		return flash.ShowMsg{ID: "diffview", Text: "Wrapped to last file", Expires: 1500 * time.Millisecond}.Cmd()
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
		oldIdx := m.nav.cursor.FileIdx
		m.nav.cursor = CursorTarget{Kind: CursorLine, FileIdx: ev.fileIdx}
		m.nav.buildDiffLines(m.files)
		m.nav.diffViewport.GotoTop()
		m.updateDiffContent()
		m.autoFollowFile(oldIdx, ev.fileIdx)
	case searchFilterDismissedMsg:
		// nothing extra needed
	}

	return m, nil
}

func (m *Model) jumpToLine(lineNum int) {
	m.nav.pushJump()
	for i, dl := range m.nav.diffLines {
		if dl.newLine == lineNum || dl.oldLine == lineNum {
			m.nav.cursor = CursorTarget{Kind: CursorLine, FileIdx: m.nav.cursor.FileIdx, LineIdx: i}
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
			return flash.ShowMsg{ID: "diffview", Text: "Wrapped to first match", Expires: 1500 * time.Millisecond}.Cmd()
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
			return flash.ShowMsg{ID: "diffview", Text: "Wrapped to last match", Expires: 1500 * time.Millisecond}.Cmd()
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

// handleNarrowPrefixKey handles the second key after pressing 'x' for filter commands.
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
	cmd := flash.ShowMsg{ID: "diffview", Text: fmt.Sprintf("Owner filter: %s", label), Expires: 1500 * time.Millisecond}.Cmd()
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
		return m, flash.ShowMsg{ID: "diffview", Text: "Already at oldest position", Expires: 1500 * time.Millisecond}.Cmd()
	}
	m.applyJumpPos(pos)
	return m, nil
}

// jumpForward navigates to the next position in the jump list.
func (m Model) jumpForward() (Model, tea.Cmd) {
	pos, ok := m.nav.jumpForward()
	if !ok {
		return m, flash.ShowMsg{ID: "diffview", Text: "Already at newest position", Expires: 1500 * time.Millisecond}.Cmd()
	}
	m.applyJumpPos(pos)
	return m, nil
}

// applyJumpPos moves the cursor to the given jump position.
func (m *Model) applyJumpPos(pos CursorTarget) {
	// Skip jumps to files excluded by active filters.
	if !m.filter.isIncluded(pos.FileIdx) {
		return
	}
	if pos.FileIdx != m.nav.cursor.FileIdx {
		oldIdx := m.nav.cursor.FileIdx
		m.nav.cursor.FileIdx = pos.FileIdx
		m.nav.buildDiffLines(m.files)
		m.autoFollowFile(oldIdx, m.nav.cursor.FileIdx)
	}
	m.nav.cursor = pos
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
	cmd := flash.ShowMsg{ID: "diffview", Text: "Filters cleared", Expires: 1500 * time.Millisecond}.Cmd()
	return m, cmd
}

// applyFilters recomputes the file filter and rebuilds the tree.
func (m *Model) applyFilters() {
	m.filter.recompute(m.files)
	m.nav.cachedTree = buildTree(m.files, m.filter.includedFiles)
	m.nav.rebuildTreeRows()

	// Ensure cursor points to an included file
	if !m.filter.isIncluded(m.nav.cursor.FileIdx) {
		// Find the first included file
		fileIdx := 0
		for i := range m.files {
			if m.filter.isIncluded(i) {
				fileIdx = i
				break
			}
		}
		m.nav.cursor = CursorTarget{Kind: CursorLine, FileIdx: fileIdx}
		m.nav.buildDiffLines(m.files)
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
