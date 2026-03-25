package diffview

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jethrokuan/pry/internal/clipboard"
	"github.com/jethrokuan/pry/internal/review"
	"github.com/jethrokuan/pry/internal/ui/styles"
)

// --- Comment selection ---

// commentRef identifies a single comment in the rendered comment list for a diff line.
type commentRef struct {
	commentID int  // Comment.ID (may be negative for optimistic)
	editable  bool // true if this is the current user's pending comment
}

// commentRefsAtLine returns all expanded comments below the given diff line index, in render order.
func (m *Model) commentRefsAtLine(diffIdx int) []commentRef {
	if diffIdx < 0 || diffIdx >= len(m.nav.diffLines) || len(m.files) == 0 || m.nav.cursor.FileIdx >= len(m.files) {
		return nil
	}
	dl := m.nav.diffLines[diffIdx]
	path := m.files[m.nav.cursor.FileIdx].Path

	var refs []commentRef

	collectForSide := func(line int, side string) {
		ck := commentKey(path, line)
		if !m.comments.expanded[ck] {
			return
		}
		for _, c := range m.comments.CommentsForLine(path, line, side) {
			editable := c.IsPending && c.Author == m.currentUser
			refs = append(refs, commentRef{commentID: c.ID, editable: editable})
		}
	}

	if dl.newLine > 0 {
		collectForSide(dl.newLine, "RIGHT")
	}
	if dl.oldLine > 0 {
		collectForSide(dl.oldLine, "LEFT")
	}

	return refs
}

// commentRefsAtCursor returns all expanded comments below the current diff cursor.
func (m *Model) commentRefsAtCursor() []commentRef {
	return m.commentRefsAtLine(m.nav.cursor.LineIdx)
}

// editSelectedComment opens the inline editor for the selected editable comment.
func (m Model) editSelectedComment() (Model, tea.Cmd) {
	refs := m.commentRefsAtCursor()
	if !m.nav.cursor.IsComment() || m.nav.cursor.CommentIdx >= len(refs) {
		return m, nil
	}
	ref := refs[m.nav.cursor.CommentIdx]
	if !ref.editable {
		return m, nil
	}

	c := m.findCommentByID(ref.commentID)
	if c == nil {
		return m, nil
	}

	m.nav.cursor = m.nav.cursor.AsLine()
	m.editor.OpenForEdit(c.Path, c.Line, c.StartLine, c.Side, c.ID, c.Body, m.inlineTextareaWidth())
	m.updateViewports()

	return m, m.editor.BlinkCmd()
}

// deleteSelectedComment deletes the selected editable comment.
func (m Model) deleteSelectedComment() (Model, tea.Cmd) {
	refs := m.commentRefsAtCursor()
	if !m.nav.cursor.IsComment() || m.nav.cursor.CommentIdx >= len(refs) {
		return m, nil
	}
	ref := refs[m.nav.cursor.CommentIdx]
	if !ref.editable {
		return m, nil
	}

	commentID := ref.commentID

	// Optimistically remove
	m.removeCommentByID(commentID)
	m.nav.cursor = m.nav.cursor.AsLine()
	m.updateDiffContent()

	return m, m.deleteCommentCmd(commentID)
}

// --- Comment helpers ---

func commentKey(path string, line int) string {
	return fmt.Sprintf("%s:%d", path, line)
}

// openCommentPopup opens a scrollable popup showing all comments for the
// current diff line.
func (m *Model) openCommentPopup() {
	if len(m.files) == 0 || m.nav.cursor.LineIdx >= len(m.nav.diffLines) {
		return
	}

	popupW := m.width - 6
	if popupW > 120 {
		popupW = 120
	}
	// Height leaves room for border + title + footer
	popupH := m.height - 6
	if popupH < 5 {
		popupH = 5
	}
	contentW := popupW - 4 // border(2) + padding(2)
	vpH := popupH - 2     // title + footer

	content := m.buildCommentPopupContent(contentW)

	vp := viewport.New(viewport.WithWidth(contentW), viewport.WithHeight(vpH))
	vp.SetContent(content)

	m.comments.popupActive = true
	m.comments.popupViewport = vp
}

// buildCommentPopupContent formats all comments for the current diff line
// into a string suitable for display in the popup viewport.
func (m *Model) buildCommentPopupContent(width int) string {
	dl := m.nav.diffLines[m.nav.cursor.LineIdx]
	path := m.files[m.nav.cursor.FileIdx].Path

	authorStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.Cyan)
	draftStyle := lipgloss.NewStyle().Foreground(styles.Warning)
	bodyStyle := lipgloss.NewStyle().Width(width)
	sepStyle := lipgloss.NewStyle().Foreground(styles.Muted)
	separator := sepStyle.Render(strings.Repeat("─", width))

	var b strings.Builder

	writeComments := func(lineNum int, side string) {
		for _, c := range m.comments.CommentsForLine(path, lineNum, side) {
			label := "💬"
			if c.IsPending {
				label = "📝"
			}
			b.WriteString(label + " " + authorStyle.Render("@"+c.Author))
			if c.IsPending {
				b.WriteString(" " + draftStyle.Render("(draft)"))
			}
			b.WriteString("\n\n")
			rendered := m.renderMarkdown(c.Body, width, styles.BgOverlay)
			b.WriteString(bodyStyle.Render(rendered) + "\n\n")
			b.WriteString(separator + "\n\n")
		}
	}

	if dl.newLine > 0 {
		writeComments(dl.newLine, "RIGHT")
	}
	if dl.oldLine > 0 {
		writeComments(dl.oldLine, "LEFT")
	}

	return strings.TrimRight(b.String(), "\n")
}

// --- Comment mutation methods ---
// All comment data mutations go through these methods, which automatically
// rebuild the comment index via CommentPanel.RebuildIndex().

// setComments replaces the full comment list and rebuilds the index.
func (m *Model) setComments(comments []review.Comment) {
	m.comments.comments = comments
	m.pr.Comments = comments
	m.comments.RebuildIndex()
}

// mergePendingComments merges pending review comments into the comment list.
// If a comment with the same ID already exists (e.g. from FetchComments which
// doesn't set IsPending), it is replaced with the pending version so that
// the IsPending flag and other pending-specific fields are preserved.
func (m *Model) mergePendingComments(pending []review.Comment) {
	pendingByID := make(map[int]review.Comment, len(pending))
	for _, c := range pending {
		pendingByID[c.ID] = c
	}
	// Replace existing entries that have a pending counterpart.
	for i, c := range m.comments.comments {
		if pc, ok := pendingByID[c.ID]; ok {
			m.comments.comments[i] = pc
			delete(pendingByID, c.ID)
		}
	}
	// Append any remaining pending comments not yet in the list.
	for _, c := range pending {
		if _, ok := pendingByID[c.ID]; ok {
			m.comments.comments = append(m.comments.comments, c)
		}
	}
	m.pr.Comments = m.comments.comments
	m.comments.RebuildIndex()
}

// addOptimisticComment appends a comment with a temp ID and rebuilds the index.
// Returns the temp ID assigned.
func (m *Model) addOptimisticComment(c review.Comment) int {
	tempID := m.pendingReview.NextTempID()
	c.ID = tempID
	m.comments.comments = append(m.comments.comments, c)
	m.pr.Comments = m.comments.comments
	m.comments.RebuildIndex()
	return tempID
}

// replaceComment swaps an optimistic comment (by temp ID) with a real one from the server.
func (m *Model) replaceComment(tempID int, real review.Comment) {
	for i, c := range m.comments.comments {
		if c.ID == tempID {
			m.comments.comments[i] = real
			m.pr.Comments = m.comments.comments
			m.comments.RebuildIndex()
			return
		}
	}
}

// removeCommentByID removes a comment by ID and rebuilds the index.
func (m *Model) removeCommentByID(id int) {
	for i, c := range m.comments.comments {
		if c.ID == id {
			m.comments.comments = append(m.comments.comments[:i], m.comments.comments[i+1:]...)
			m.pr.Comments = m.comments.comments
			m.comments.RebuildIndex()
			return
		}
	}
}

// updateCommentBody updates the body of a comment by ID and rebuilds the index.
func (m *Model) updateCommentBody(id int, body string) {
	for i, c := range m.comments.comments {
		if c.ID == id {
			m.comments.comments[i].Body = body
			m.pr.Comments = m.comments.comments
			m.comments.RebuildIndex()
			return
		}
	}
}

// findCommentByID returns a pointer to the comment with the given ID, or nil.
func (m *Model) findCommentByID(id int) *review.Comment {
	for i := range m.comments.comments {
		if m.comments.comments[i].ID == id {
			return &m.comments.comments[i]
		}
	}
	return nil
}

func lineAndSide(dl diffLineInfo) (int, string) {
	if dl.newLine > 0 {
		return dl.newLine, "RIGHT"
	}
	return dl.oldLine, "LEFT"
}

// --- Comment folding ---

func (m Model) toggleCommentAtCursor() Model {
	m.nav.cursor = m.nav.cursor.AsLine()
	if len(m.files) == 0 || m.nav.cursor.FileIdx >= len(m.files) {
		return m
	}
	path := m.files[m.nav.cursor.FileIdx].Path

	if m.nav.focus == FocusDiff && m.nav.cursor.LineIdx < len(m.nav.diffLines) {
		dl := m.nav.diffLines[m.nav.cursor.LineIdx]
		line := dl.newLine
		if line == 0 {
			line = dl.oldLine
		}
		ck := commentKey(path, line)
		m.comments.expanded[ck] = !m.comments.expanded[ck]
	} else {
		// Cycle all comments for current file: if any expanded → collapse all, else expand all
		anyExpanded := false
		keys := m.commentKeysForFile(path)
		for _, ck := range keys {
			if m.comments.expanded[ck] {
				anyExpanded = true
				break
			}
		}
		for _, ck := range keys {
			m.comments.expanded[ck] = !anyExpanded
		}
	}

	m.updateDiffContent()
	return m
}

// commentKeysForFile returns all comment keys for the given file path.
func (m *Model) commentKeysForFile(path string) []string {
	seen := make(map[string]bool)
	var keys []string
	add := func(ck string) {
		if !seen[ck] {
			seen[ck] = true
			keys = append(keys, ck)
		}
	}
	for _, c := range m.comments.comments {
		if c.Path == path {
			add(commentKey(path, c.Line))
		}
	}
	return keys
}

// --- Mark as viewed ---

func (m Model) markCurrentFileViewed() (Model, tea.Cmd) {
	if len(m.files) == 0 || m.nav.cursor.FileIdx >= len(m.files) {
		return m, nil
	}
	file := m.files[m.nav.cursor.FileIdx]
	path := file.Path
	prNodeID := m.pr.NodeID

	if m.pendingReview.ViewedFiles[path] {
		// Unmark
		delete(m.pendingReview.ViewedFiles, path)
		m.nav.treeViewport.SetContent(m.renderFileTree())
		m.updateDiffContent()
		return m, func() tea.Msg {
			err := m.svc.UnmarkFileAsViewed(context.Background(), prNodeID, path)
			return markViewedMsg{path: "", err: err}
		}
	}

	// Optimistically mark as viewed
	m.pendingReview.ViewedFiles[path] = true

	// Navigate to next unviewed file
	m.navigateToNextUnviewed()

	m.nav.treeViewport.SetContent(m.renderFileTree())
	m.updateDiffContent()
	return m, func() tea.Msg {
		err := m.svc.MarkFileAsViewed(context.Background(), prNodeID, path)
		return markViewedMsg{path: path, err: err}
	}
}

// navigateToNextUnviewed moves to the next unviewed file, wrapping around.
func (m *Model) navigateToNextUnviewed() {
	n := len(m.files)
	if n == 0 {
		return
	}
	// Search forward from current position
	for i := 1; i < n; i++ {
		idx := (m.nav.cursor.FileIdx + i) % n
		if !m.filter.isIncluded(idx) {
			continue
		}
		if !m.pendingReview.ViewedFiles[m.files[idx].Path] {
			oldIdx := m.nav.cursor.FileIdx
			m.nav.cursor = CursorTarget{Kind: CursorLine, FileIdx: idx}
			m.nav.buildDiffLines(m.files)
			m.autoFollowFile(oldIdx, idx)
			return
		}
	}
	// All files viewed — stay on current file
}

// --- Batch mark viewed (tree item) ---

// markTreeItemViewed handles 'm' on a tree row: single file or batch folder toggle.
func (m Model) markTreeItemViewed() (Model, tea.Cmd) {
	if m.nav.treeCursor < 0 || m.nav.treeCursor >= len(m.nav.treeRows) {
		return m, nil
	}
	row := m.nav.treeRows[m.nav.treeCursor]

	if row.node.fileIdx >= 0 {
		// Single file — sync fileCursor to tree selection and delegate
		m.nav.cursor.FileIdx = row.node.fileIdx
		return m.markCurrentFileViewed()
	}

	// Folder: collect all descendant file indices
	indices := collectFileIndices(row.node)
	if len(indices) == 0 {
		return m, nil
	}

	// Check if all descendants are already viewed
	allViewed := true
	for _, idx := range indices {
		if idx < len(m.files) && !m.pendingReview.ViewedFiles[m.files[idx].Path] {
			allViewed = false
			break
		}
	}

	prNodeID := m.pr.NodeID
	var cmds []tea.Cmd

	if allViewed {
		// Unmark all
		for _, idx := range indices {
			if idx < len(m.files) {
				path := m.files[idx].Path
				delete(m.pendingReview.ViewedFiles, path)
				p := path // capture for closure
				cmds = append(cmds, func() tea.Msg {
					err := m.svc.UnmarkFileAsViewed(context.Background(), prNodeID, p)
					return markViewedMsg{path: "", err: err}
				})
			}
		}
	} else {
		// Mark all unviewed
		for _, idx := range indices {
			if idx < len(m.files) {
				path := m.files[idx].Path
				if !m.pendingReview.ViewedFiles[path] {
					m.pendingReview.ViewedFiles[path] = true
					p := path // capture for closure
					cmds = append(cmds, func() tea.Msg {
						err := m.svc.MarkFileAsViewed(context.Background(), prNodeID, p)
						return markViewedMsg{path: p, err: err}
					})
				}
			}
		}
	}

	// Optimistic UI update
	m.nav.treeViewport.SetContent(m.renderFileTree())

	if len(cmds) > 0 {
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

// --- Inline comment editor ---

func (m *Model) openInlineComment(path string, line, startLine int, side string, mode commentMode, suggestion string) tea.Cmd {
	m.editor.Open(path, line, startLine, side, mode, suggestion, m.inlineTextareaWidth())
	m.updateViewports()
	return m.editor.BlinkCmd()
}

func (m *Model) closeInlineComment() {
	m.editor.Close()
	m.updateViewports()
	m.updateDiffContent()
}

func (m Model) inlineTextareaWidth() int {
	treeWidth := 0
	if m.nav.showTree {
		treeWidth = min(50, m.width/3) + 1
	}
	return m.width - treeWidth
}

// handleEditorSave processes a save message from the InlineEditor.
func (m Model) handleEditorSave(msg inlineEditorSaveMsg) (Model, tea.Cmd) {
	// Editing an existing comment
	if msg.editCommentID != 0 {
		c := m.findCommentByID(msg.editCommentID)
		if c != nil {
			oldBody := c.Body
			commentID := c.ID
			// Optimistically update body
			m.updateCommentBody(commentID, msg.body)
			m.closeInlineComment()
			return m, m.editCommentCmd(commentID, msg.body, oldBody)
		}
		m.closeInlineComment()
		return m, nil
	}

	side := msg.side
	if side == "" {
		side = "RIGHT"
	}

	// Create optimistic comment
	optimistic := review.Comment{
		Path:      msg.path,
		Line:      msg.line,
		StartLine: msg.startLine,
		Side:      side,
		Body:      msg.body,
		Author:    m.currentUser,
		IsPending: true,
	}
	tempID := m.addOptimisticComment(optimistic)
	m.inflight[tempID] = true

	m.closeInlineComment()

	// The service ensures a pending review exists internally — no buffering needed.
	return m, m.addReviewCommentCmd(tempID, msg.path, msg.line, msg.startLine, side, msg.body)
}

func (m Model) openExternalEditorForComment() tea.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}

	content := m.editor.ta.Value()

	tmpFile, err := os.CreateTemp("", "pry-comment-*.md")
	if err != nil {
		return nil
	}
	tmpFile.WriteString(content)
	tmpFile.Close()
	tmpPath := tmpFile.Name()

	return tea.ExecProcess(exec.Command(editor, tmpPath), func(err error) tea.Msg {
		if err != nil {
			return editorFinishedMsg{err: err}
		}
		data, err := os.ReadFile(tmpPath)
		os.Remove(tmpPath)
		if err != nil {
			return editorFinishedMsg{err: err}
		}
		return editorFinishedMsg{content: string(data)}
	})
}

func (m *Model) startComment() tea.Cmd {
	if len(m.nav.diffLines) == 0 || m.nav.cursor.LineIdx >= len(m.nav.diffLines) {
		return nil
	}
	dl := m.nav.diffLines[m.nav.cursor.LineIdx]
	path := m.files[m.nav.cursor.FileIdx].Path
	line, side := lineAndSide(dl)

	if m.nav.visualMode {
		s, e := min(m.nav.visualStart, m.nav.visualEnd), max(m.nav.visualStart, m.nav.visualEnd)
		startLine, _ := lineAndSide(m.nav.diffLines[s])
		endLine, endSide := lineAndSide(m.nav.diffLines[e])
		m.nav.visualMode = false
		return m.openInlineComment(path, endLine, startLine, endSide, commentModeComment, "")
	}
	return m.openInlineComment(path, line, 0, side, commentModeComment, "")
}

// --- Image paste support ---

func checkClipboardImageCmd() tea.Msg {
	data, err := clipboard.ReadImage()
	return clipboardImageMsg{data: data, err: err}
}

func (m Model) uploadImageCmd(data []byte) tea.Cmd {
	svc := m.svc
	return func() tea.Msg {
		url, err := svc.UploadImage(context.Background(), data, "image.png")
		return imageUploadedMsg{url: url, err: err}
	}
}

func (m Model) openInEditor() tea.Cmd {
	if len(m.files) == 0 {
		return nil
	}
	file := m.files[m.nav.cursor.FileIdx]
	line := 1
	if m.nav.cursor.LineIdx < len(m.nav.diffLines) {
		if dl := m.nav.diffLines[m.nav.cursor.LineIdx]; dl.newLine > 0 {
			line = dl.newLine
		}
	}
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}
	return tea.ExecProcess(exec.Command(editor, fmt.Sprintf("+%d", line), file.Path), nil)
}
