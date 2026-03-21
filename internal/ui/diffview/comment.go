package diffview

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jkuan/pr-review/internal/clipboard"
	"github.com/jkuan/pr-review/internal/review"
	"github.com/jkuan/pr-review/internal/ui/styles"
)

// --- Comment selection ---

// commentRef identifies a single comment in the rendered comment list for a diff line.
type commentRef struct {
	isLocal bool // true for local pending (m.pr.PendingReview.Comments)
	localID int  // InlineComment.LocalID (only meaningful when isLocal)
}

// commentRefsAtLine returns all expanded comments below the given diff line index, in render order.
func (m *Model) commentRefsAtLine(diffIdx int) []commentRef {
	if diffIdx < 0 || diffIdx >= len(m.nav.diffLines) || len(m.files) == 0 || m.nav.fileCursor >= len(m.files) {
		return nil
	}
	dl := m.nav.diffLines[diffIdx]
	path := m.files[m.nav.fileCursor].Path

	var refs []commentRef

	collectForSide := func(line int, side string) {
		ck := commentKey(path, line)
		if !m.comments.expanded[ck] {
			return
		}
		for range m.commentsForLine(path, line, side) {
			refs = append(refs, commentRef{})
		}
		for _, c := range m.localPendingForLine(path, line, side) {
			refs = append(refs, commentRef{isLocal: true, localID: c.LocalID})
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
	return m.commentRefsAtLine(m.nav.diffCursor)
}

// editSelectedComment opens the inline editor for the selected local pending comment.
func (m Model) editSelectedComment() (Model, tea.Cmd) {
	refs := m.commentRefsAtCursor()
	if m.comments.cursor < 0 || m.comments.cursor >= len(refs) {
		return m, nil
	}
	ref := refs[m.comments.cursor]
	if !ref.isLocal {
		return m, nil
	}

	c := m.pr.PendingReview.FindByLocalID(ref.localID)
	if c == nil {
		return m, nil
	}

	m.comments.cursor = -1
	m.comments.inlineActive = true
	m.comments.inlinePath = c.Path
	m.comments.inlineLine = c.Line
	m.comments.inlineStartLine = c.StartLine
	m.comments.inlineSide = c.Side
	m.comments.inlineMode = commentModeComment
	m.comments.inlineEditLocalID = c.LocalID

	ta := textarea.New()
	ta.Placeholder = "Edit your comment..."
	ta.Focus()
	ta.CharLimit = 0
	ta.SetWidth(m.inlineTextareaWidth() - 4)
	ta.SetHeight(5)
	ta.SetValue(c.Body)

	m.comments.inlineTextarea = ta
	m.updateViewports()

	return m, textarea.Blink
}

// deleteSelectedComment deletes the selected local pending comment.
func (m Model) deleteSelectedComment() (Model, tea.Cmd) {
	refs := m.commentRefsAtCursor()
	if m.comments.cursor < 0 || m.comments.cursor >= len(refs) {
		return m, nil
	}
	ref := refs[m.comments.cursor]
	if !ref.isLocal {
		return m, nil
	}

	c := m.pr.PendingReview.FindByLocalID(ref.localID)
	if c == nil {
		return m, nil
	}

	localID := c.LocalID
	forgeID := c.ForgeID

	m.removeLocalComment(localID)
	m.comments.cursor = -1
	m.updateDiffContent()

	return m, m.deleteCommentCmd(localID, forgeID)
}

// --- Comment helpers ---

func commentKey(path string, line int) string {
	return fmt.Sprintf("%s:%d", path, line)
}

// openCommentPopup opens a scrollable popup showing all comments for the
// current diff line.
func (m *Model) openCommentPopup() {
	if len(m.files) == 0 || m.nav.diffCursor >= len(m.nav.diffLines) {
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
	dl := m.nav.diffLines[m.nav.diffCursor]
	path := m.files[m.nav.fileCursor].Path

	authorStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.Cyan)
	pendingStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.Warning)
	draftStyle := lipgloss.NewStyle().Foreground(styles.Warning)
	bodyStyle := lipgloss.NewStyle().Width(width)
	sepStyle := lipgloss.NewStyle().Foreground(styles.Muted)
	separator := sepStyle.Render(strings.Repeat("─", width))

	var b strings.Builder

	writeExisting := func(lineNum int, side string) {
		for _, c := range m.commentsForLine(path, lineNum, side) {
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
	writeLocal := func(lineNum int, side string) {
		for _, c := range m.localPendingForLine(path, lineNum, side) {
			syncLabel := ""
			switch c.SyncStatus {
			case review.SyncInFlight:
				syncLabel = " ..."
			case review.SyncComplete:
				syncLabel = " ✓"
			case review.SyncFailed:
				syncLabel = " ✗"
			}
			b.WriteString("📝 " + pendingStyle.Render("(pending)") + syncLabel + "\n\n")
			rendered := m.renderMarkdown(c.Body, width, styles.BgOverlay)
			b.WriteString(bodyStyle.Render(rendered) + "\n\n")
			b.WriteString(separator + "\n\n")
		}
	}

	writeSide := func(lineNum int, side string) {
		writeExisting(lineNum, side)
		writeLocal(lineNum, side)
	}

	if dl.newLine > 0 {
		writeSide(dl.newLine, "RIGHT")
	}
	if dl.oldLine > 0 {
		writeSide(dl.oldLine, "LEFT")
	}

	return strings.TrimRight(b.String(), "\n")
}

func commentIndexKey(path string, line int, side string) string {
	return fmt.Sprintf("%s:%d:%s", path, line, side)
}

// --- Comment mutation methods ---
// All comment data mutations go through these methods, which automatically
// rebuild the comment index. This eliminates the need to manually call
// rebuildCommentIndex() at each mutation site.

// setExistingComments replaces the existing comments and rebuilds the index.
func (m *Model) setExistingComments(comments []review.ExistingComment) {
	m.comments.existing = comments
	m.rebuildCommentIndex()
}

// setForgeComments replaces the forge comments and rebuilds the index.
func (m *Model) setForgeComments(comments []review.ExistingComment) {
	m.comments.forgeComments = comments
	m.rebuildCommentIndex()
}

// addLocalComment adds a new local pending comment and rebuilds the index.
// Returns the assigned LocalID.
func (m *Model) addLocalComment(c review.InlineComment) int {
	id := m.pr.PendingReview.AddCommentDirect(c)
	m.rebuildCommentIndex()
	return id
}

// removeLocalComment removes a local pending comment by LocalID and rebuilds
// the index. Returns the removed comment's ForgeID.
func (m *Model) removeLocalComment(localID int) int {
	forgeID := m.pr.PendingReview.RemoveCommentByLocalID(localID)
	m.rebuildCommentIndex()
	return forgeID
}

// updateLocalComment finds a comment by LocalID and applies the given mutation,
// then rebuilds the index.
func (m *Model) updateLocalComment(localID int, fn func(*review.InlineComment)) {
	if c := m.pr.PendingReview.FindByLocalID(localID); c != nil {
		fn(c)
		m.rebuildCommentIndex()
	}
}

// restoreForgeComments batch-adds comments from the forge (crash recovery)
// and sets forgeComments, rebuilding the index once at the end.
func (m *Model) restoreForgeComments(pendingComments []review.ExistingComment, forgeComments []review.ExistingComment) {
	for _, ec := range pendingComments {
		m.pr.PendingReview.AddCommentDirect(review.InlineComment{
			Path:       ec.Path,
			Line:       ec.Line,
			Side:       ec.Side,
			Body:       ec.Body,
			ForgeID:    ec.ID,
			SyncStatus: review.SyncComplete,
		})
	}
	m.comments.forgeComments = forgeComments
	m.rebuildCommentIndex()
}

// rebuildCommentIndex rebuilds the existingIndex and localPendingIndex maps
// from the current comment data. It is called automatically by mutation methods
// and should not normally be called directly.
func (m *Model) rebuildCommentIndex() {
	m.comments.existingIndex = make(map[string][]review.ExistingComment)
	m.comments.fileCommentIndex = make(map[string]bool)
	for _, c := range m.comments.existing {
		k := commentIndexKey(c.Path, c.Line, c.Side)
		m.comments.existingIndex[k] = append(m.comments.existingIndex[k], c)
		m.comments.fileCommentIndex[c.Path] = true
	}
	for _, c := range m.comments.forgeComments {
		k := commentIndexKey(c.Path, c.Line, c.Side)
		m.comments.existingIndex[k] = append(m.comments.existingIndex[k], c)
		m.comments.fileCommentIndex[c.Path] = true
	}

	m.comments.localPendingIndex = make(map[string][]review.InlineComment)
	for _, c := range m.pr.PendingReview.Comments {
		k := commentIndexKey(c.Path, c.Line, c.Side)
		m.comments.localPendingIndex[k] = append(m.comments.localPendingIndex[k], c)
		m.comments.fileCommentIndex[c.Path] = true
	}
}

func (m *Model) commentsForLine(path string, line int, side string) []review.ExistingComment {
	exact := m.comments.existingIndex[commentIndexKey(path, line, side)]
	// Also include comments with empty side (they match any side query)
	if side != "" {
		emptySide := m.comments.existingIndex[commentIndexKey(path, line, "")]
		if len(emptySide) > 0 {
			result := make([]review.ExistingComment, 0, len(exact)+len(emptySide))
			result = append(result, exact...)
			result = append(result, emptySide...)
			return result
		}
	}
	return exact
}

func (m *Model) localPendingForLine(path string, line int, side string) []review.InlineComment {
	return m.comments.localPendingIndex[commentIndexKey(path, line, side)]
}

func lineAndSide(dl diffLineInfo) (int, string) {
	if dl.newLine > 0 {
		return dl.newLine, "RIGHT"
	}
	return dl.oldLine, "LEFT"
}

// --- Comment folding ---

func (m Model) toggleCommentAtCursor() Model {
	m.comments.cursor = -1
	if len(m.files) == 0 || m.nav.fileCursor >= len(m.files) {
		return m
	}
	path := m.files[m.nav.fileCursor].Path

	if m.nav.focus == FocusDiff && m.nav.diffCursor < len(m.nav.diffLines) {
		dl := m.nav.diffLines[m.nav.diffCursor]
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

func (m Model) toggleAllComments() Model {
	m.comments.cursor = -1
	// Global cycle: if any comment is expanded → collapse all, else expand all
	anyExpanded := false
	for _, v := range m.comments.expanded {
		if v {
			anyExpanded = true
			break
		}
	}

	// Collect all comment keys across all files
	allKeys := make(map[string]bool)
	for _, c := range m.comments.existing {
		allKeys[commentKey(c.Path, c.Line)] = true
	}
	for _, c := range m.comments.forgeComments {
		allKeys[commentKey(c.Path, c.Line)] = true
	}
	for _, c := range m.pr.PendingReview.Comments {
		allKeys[commentKey(c.Path, c.Line)] = true
	}

	for ck := range allKeys {
		m.comments.expanded[ck] = !anyExpanded
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
	for _, c := range m.comments.existing {
		if c.Path == path {
			add(commentKey(path, c.Line))
		}
	}
	for _, c := range m.comments.forgeComments {
		if c.Path == path {
			add(commentKey(path, c.Line))
		}
	}
	for _, c := range m.pr.PendingReview.Comments {
		if c.Path == path {
			add(commentKey(c.Path, c.Line))
		}
	}
	return keys
}

// --- Mark as viewed ---

func (m Model) markCurrentFileViewed() (Model, tea.Cmd) {
	if len(m.files) == 0 || m.nav.fileCursor >= len(m.files) {
		return m, nil
	}
	file := m.files[m.nav.fileCursor]
	path := file.Path
	prNodeID := m.pr.NodeID

	if m.pr.PendingReview.ViewedFiles[path] {
		// Unmark
		delete(m.pr.PendingReview.ViewedFiles, path)
		m.nav.treeViewport.SetContent(m.renderFileTree())
		m.updateDiffContent()
		return m, func() tea.Msg {
			err := m.ctx.Svc.UnmarkFileAsViewed(context.Background(), prNodeID, path)
			return markViewedMsg{path: "", err: err}
		}
	}

	// Optimistically mark as viewed
	m.pr.PendingReview.ViewedFiles[path] = true

	// Navigate to next unviewed file
	m.navigateToNextUnviewed()

	m.nav.treeViewport.SetContent(m.renderFileTree())
	m.updateDiffContent()
	return m, func() tea.Msg {
		err := m.ctx.Svc.MarkFileAsViewed(context.Background(), prNodeID, path)
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
		idx := (m.nav.fileCursor + i) % n
		if !m.filter.isIncluded(idx) {
			continue
		}
		if !m.pr.PendingReview.ViewedFiles[m.files[idx].Path] {
			oldIdx := m.nav.fileCursor
			m.nav.fileCursor = idx
			m.nav.buildDiffLines(m.files)
			m.nav.diffCursor = 0
			m.autoFollowFile(oldIdx, m.nav.fileCursor)
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
		m.nav.fileCursor = row.node.fileIdx
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
		if idx < len(m.files) && !m.pr.PendingReview.ViewedFiles[m.files[idx].Path] {
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
				delete(m.pr.PendingReview.ViewedFiles, path)
				p := path // capture for closure
				cmds = append(cmds, func() tea.Msg {
					err := m.ctx.Svc.UnmarkFileAsViewed(context.Background(), prNodeID, p)
					return markViewedMsg{path: "", err: err}
				})
			}
		}
	} else {
		// Mark all unviewed
		for _, idx := range indices {
			if idx < len(m.files) {
				path := m.files[idx].Path
				if !m.pr.PendingReview.ViewedFiles[path] {
					m.pr.PendingReview.ViewedFiles[path] = true
					p := path // capture for closure
					cmds = append(cmds, func() tea.Msg {
						err := m.ctx.Svc.MarkFileAsViewed(context.Background(), prNodeID, p)
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
	m.comments.inlineActive = true
	m.comments.inlinePath = path
	m.comments.inlineLine = line
	m.comments.inlineStartLine = startLine
	m.comments.inlineSide = side
	m.comments.inlineMode = mode
	m.comments.inlineSuggestion = suggestion

	ta := textarea.New()
	ta.Placeholder = "Write your comment..."
	ta.Focus()
	ta.CharLimit = 0
	ta.SetWidth(m.inlineTextareaWidth() - 4)
	ta.SetHeight(5)

	if mode == commentModeSuggestion && suggestion != "" {
		ta.SetValue(suggestion)
	}

	m.comments.inlineTextarea = ta
	m.updateViewports()

	return textarea.Blink
}

func (m *Model) closeInlineComment() {
	m.comments.inlineActive = false
	m.comments.inlineEditLocalID = 0
	m.comments.confirmDiscard = false
	m.comments.mentionActive = false
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

func (m Model) saveInlineComment() (Model, tea.Cmd) {
	text := strings.TrimSpace(m.comments.inlineTextarea.Value())
	if text == "" {
		m.closeInlineComment()
		return m, nil
	}

	body := text
	if m.comments.inlineMode == commentModeSuggestion {
		body = fmt.Sprintf("```suggestion\n%s\n```", text)
	}

	// Editing an existing comment
	if m.comments.inlineEditLocalID != 0 {
		c := m.pr.PendingReview.FindByLocalID(m.comments.inlineEditLocalID)
		if c != nil {
			localID := c.LocalID
			forgeID := c.ForgeID
			m.updateLocalComment(localID, func(c *review.InlineComment) {
				c.Body = body
			})
			m.closeInlineComment()
			return m, m.editCommentCmd(localID, forgeID, body)
		}
		m.closeInlineComment()
		return m, nil
	}

	side := m.comments.inlineSide
	if side == "" {
		side = "RIGHT"
	}

	syncStatus := review.SyncPending
	if m.pr.PendingReview.ReviewNodeID != "" {
		syncStatus = review.SyncInFlight
	}

	newComment := review.InlineComment{
		Path:       m.comments.inlinePath,
		Line:       m.comments.inlineLine,
		StartLine:  m.comments.inlineStartLine,
		Side:       side,
		Body:       body,
		SyncStatus: syncStatus,
	}
	m.addLocalComment(newComment)

	// Get the comment back with its assigned LocalID
	added := m.pr.PendingReview.Comments[len(m.pr.PendingReview.Comments)-1]

	m.closeInlineComment()

	// If no pending review exists on the forge yet, create one now.
	// The reviewCreatedMsg handler will flush all pending comments.
	if m.pr.PendingReview.ReviewNodeID == "" {
		return m, m.createPendingReviewCmd()
	}
	return m, m.syncCommentCmd(added)
}

func (m Model) openExternalEditorForComment() tea.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}

	content := m.comments.inlineTextarea.Value()

	tmpFile, err := os.CreateTemp("", "pr-review-comment-*.md")
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
	if len(m.nav.diffLines) == 0 || m.nav.diffCursor >= len(m.nav.diffLines) {
		return nil
	}
	dl := m.nav.diffLines[m.nav.diffCursor]
	path := m.files[m.nav.fileCursor].Path
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

// --- Delete / Edit comment at cursor ---

// pendingCommentAtCursor returns the local pending comment at the current diff cursor, if any.
func (m *Model) pendingCommentAtCursor() *review.InlineComment {
	if len(m.nav.diffLines) == 0 || m.nav.diffCursor >= len(m.nav.diffLines) {
		return nil
	}
	dl := m.nav.diffLines[m.nav.diffCursor]
	path := m.files[m.nav.fileCursor].Path
	line, side := lineAndSide(dl)

	for i := range m.pr.PendingReview.Comments {
		c := &m.pr.PendingReview.Comments[i]
		if c.Path == path && c.Line == line && c.Side == side {
			return c
		}
	}
	return nil
}

func (m Model) deleteCommentAtCursor() (Model, tea.Cmd) {
	c := m.pendingCommentAtCursor()
	if c == nil {
		return m, nil
	}
	localID := c.LocalID
	forgeID := c.ForgeID

	m.removeLocalComment(localID)
	m.updateDiffContent()

	return m, m.deleteCommentCmd(localID, forgeID)
}

func (m Model) editCommentAtCursor() (Model, tea.Cmd) {
	c := m.pendingCommentAtCursor()
	if c == nil {
		return m, nil
	}

	// Open inline editor pre-filled with existing comment body
	m.comments.inlineActive = true
	m.comments.inlinePath = c.Path
	m.comments.inlineLine = c.Line
	m.comments.inlineStartLine = c.StartLine
	m.comments.inlineSide = c.Side
	m.comments.inlineMode = commentModeComment
	m.comments.inlineEditLocalID = c.LocalID

	ta := textarea.New()
	ta.Placeholder = "Edit your comment..."
	ta.Focus()
	ta.CharLimit = 0
	ta.SetWidth(m.inlineTextareaWidth() - 4)
	ta.SetHeight(5)
	ta.SetValue(c.Body)

	m.comments.inlineTextarea = ta
	m.updateViewports()

	return m, textarea.Blink
}

// --- Image paste support ---

func checkClipboardImageCmd() tea.Msg {
	data, err := clipboard.ReadImage()
	return clipboardImageMsg{data: data, err: err}
}

func (m Model) uploadImageCmd(data []byte) tea.Cmd {
	svc := m.ctx.Svc
	return func() tea.Msg {
		url, err := svc.UploadImage(context.Background(), data, "image.png")
		return imageUploadedMsg{url: url, err: err}
	}
}

func (m Model) openInEditor() tea.Cmd {
	if len(m.files) == 0 {
		return nil
	}
	file := m.files[m.nav.fileCursor]
	line := 1
	if m.nav.diffCursor < len(m.nav.diffLines) {
		if dl := m.nav.diffLines[m.nav.diffCursor]; dl.newLine > 0 {
			line = dl.newLine
		}
	}
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}
	return tea.ExecProcess(exec.Command(editor, fmt.Sprintf("+%d", line), file.Path), nil)
}
