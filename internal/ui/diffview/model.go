package diffview

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jkuan/pr-review/internal/diff"
	"github.com/jkuan/pr-review/internal/review"
	"github.com/jkuan/pr-review/internal/ui/styles"
)

// --- Messages ---

type SubmitReviewMsg struct{}
type BackMsg struct{}

type filesLoadedMsg struct {
	files []diff.DiffFile
	err   error
}

type existingCommentsMsg struct {
	comments []review.ExistingComment
	err      error
}

type pendingReviewMsg struct {
	reviewID     int
	reviewNodeID string
	comments     []review.ExistingComment
	err          error
}

type reviewCreatedMsg struct {
	reviewID     int
	reviewNodeID string
	err          error
}

type commentSyncedMsg struct {
	localID  int
	forgeID int
	err      error
}

type commentDeletedMsg struct {
	localID int
	err     error
}

type commentEditedMsg struct {
	localID int
	body    string
	err     error
}

type viewedFilesMsg struct {
	viewed map[string]bool
	err    error
}

type markViewedMsg struct {
	path string
	err  error
}

type editorFinishedMsg struct {
	content string
	err     error
}

// --- Inline comment mode ---

type commentMode int

const (
	commentModeComment commentMode = iota
	commentModeSuggestion
)

// --- Focus ---

type Focus int

const (
	FocusFileTree Focus = iota
	FocusDiff
)

// --- Keys ---

type KeyMap struct {
	Up            key.Binding
	Down          key.Binding
	PageUp        key.Binding
	PageDown      key.Binding
	ToggleTree    key.Binding
	Comment       key.Binding
	DeleteComment key.Binding
	EditComment   key.Binding
	Reply         key.Binding
	SelectLine    key.Binding
	Submit        key.Binding
	ToggleComment key.Binding
	FoldComment   key.Binding
	MarkViewed    key.Binding
	OpenInBrowser key.Binding
	Editor        key.Binding
	Back          key.Binding
	Quit          key.Binding
	Enter         key.Binding
	Search        key.Binding
	FilterFile    key.Binding
	Help          key.Binding
	GotoPrefix    key.Binding
	GotoEnd       key.Binding
	NextMatch     key.Binding
	PrevMatch     key.Binding
}

var keys = KeyMap{
	Up:            key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:          key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	PageUp:        key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("ctrl+u", "page up")),
	PageDown:      key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "page down")),
	ToggleTree:    key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "toggle tree")),
	Comment:       key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "comment")),
	DeleteComment: key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete comment")),
	EditComment:   key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit comment")),
	Reply:         key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reply")),
	SelectLine:    key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "select")),
	Submit:        key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "submit review")),
	ToggleComment: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "toggle comments")),
	FoldComment:   key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("S-tab", "toggle all comments")),
	MarkViewed:    key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "mark viewed")),
	OpenInBrowser: key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "open in browser")),
	Editor:        key.NewBinding(key.WithKeys("ctrl+e"), key.WithHelp("ctrl+e", "open in editor")),
	Back:          key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	Quit:          key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	Enter:         key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select file")),
	Search:        key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
	FilterFile:    key.NewBinding(key.WithKeys("ctrl+p"), key.WithHelp("ctrl+p", "filter files")),
	Help:          key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	GotoPrefix:    key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "go to...")),
	GotoEnd:       key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "go to end")),
	NextMatch:     key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next match")),
	PrevMatch:     key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "prev match")),
}

// Inline comment key bindings (when textarea is active)
var inlineKeys = struct {
	Save       key.Binding
	Cancel     key.Binding
	ToggleMode key.Binding
	OpenEditor key.Binding
}{
	Save:       key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "save")),
	Cancel:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	ToggleMode: key.NewBinding(key.WithKeys("ctrl+t"), key.WithHelp("ctrl+t", "toggle mode")),
	OpenEditor: key.NewBinding(key.WithKeys("ctrl+e"), key.WithHelp("ctrl+e", "open $EDITOR")),
}

// --- Model ---

type Model struct {
	svc    review.Service
	pr     review.PullRequest
	review *review.PendingReview
	files  []diff.DiffFile

	nav      DiffNav
	comments CommentPanel
	search   SearchBar

	width    int
	height   int
	loading  bool
	spinner  spinner.Model
	showHelp bool

	errors errorStore // unified error tracking for all async operations

	confirmQuit bool // true when waiting for second quit key to confirm

	// Caches
	mdCache  map[mdCacheKey]string // rendered markdown cache
	treeDirty bool                 // true when file tree needs re-rendering
}

type diffLineInfo struct {
	fileIdx  int
	hunkIdx  int
	lineIdx  int
	newLine  int
	oldLine  int
	lineType diff.LineType
	content  string
}

// --- Constructor ---

func New(svc review.Service, pr review.PullRequest, rev *review.PendingReview) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		svc:               svc,
		pr:                pr,
		review:            rev,
		loading: true,
		spinner: s,
		errors:  newErrorStore(),
		mdCache: make(map[mdCacheKey]string),
		treeDirty:         true,
		nav: DiffNav{
			showTree:      true,
			focus:         FocusDiff,
			collapsedDirs: make(map[string]bool),
			activeCycler:  'h',
		},
		comments: CommentPanel{
			expanded: make(map[string]bool),
			cursor:   -1,
		},
	}
}

// --- Init ---

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.loadFiles(),
		m.loadComments(),
		m.loadPendingReview(),
		m.loadViewedFiles(),
		m.spinner.Tick,
	)
}

func (m Model) loadViewedFiles() tea.Cmd {
	return func() tea.Msg {
		viewed, err := m.svc.FetchViewedFiles(context.Background(), m.review.PRNodeID)
		return viewedFilesMsg{viewed: viewed, err: err}
	}
}

func (m Model) loadFiles() tea.Cmd {
	return func() tea.Msg {
		files, err := m.svc.FetchDiffFiles(context.Background(), m.pr.Number)
		return filesLoadedMsg{files: files, err: err}
	}
}

func (m Model) loadComments() tea.Cmd {
	return func() tea.Msg {
		comments, err := m.svc.FetchExistingComments(context.Background(), m.pr.Number)
		return existingCommentsMsg{comments: comments, err: err}
	}
}

func (m Model) loadPendingReview() tea.Cmd {
	return func() tea.Msg {
		reviewID, nodeID, comments, err := m.svc.FetchPendingReview(context.Background(), m.pr.Number)
		return pendingReviewMsg{reviewID: reviewID, reviewNodeID: nodeID, comments: comments, err: err}
	}
}

func (m Model) createPendingReviewCmd() tea.Cmd {
	return func() tea.Msg {
		id, nodeID, err := m.svc.CreatePendingReview(context.Background(), m.pr.Number)
		return reviewCreatedMsg{reviewID: id, reviewNodeID: nodeID, err: err}
	}
}

func (m Model) syncCommentCmd(c review.InlineComment) tea.Cmd {
	if m.review.ReviewNodeID == "" {
		return nil
	}
	// Capture values to avoid reading shared state from goroutine
	svc := m.svc
	reviewNodeID := m.review.ReviewNodeID
	localID := c.LocalID
	comment := review.InlineComment{
		Path:      c.Path,
		Line:      c.Line,
		StartLine: c.StartLine,
		Side:      c.Side,
		Body:      c.Body,
	}
	return func() tea.Msg {
		forgeID, err := svc.AddReviewComment(context.Background(), reviewNodeID, comment)
		return commentSyncedMsg{localID: localID, forgeID: forgeID, err: err}
	}
}

func (m Model) deleteCommentCmd(localID, forgeID int) tea.Cmd {
	if forgeID == 0 {
		return nil
	}
	svc := m.svc
	prNumber := m.pr.Number
	return func() tea.Msg {
		err := svc.DeleteReviewComment(context.Background(), prNumber, forgeID)
		return commentDeletedMsg{localID: localID, err: err}
	}
}

func (m Model) editCommentCmd(localID, forgeID int, body string) tea.Cmd {
	if forgeID == 0 {
		return nil
	}
	svc := m.svc
	prNumber := m.pr.Number
	return func() tea.Msg {
		err := svc.EditReviewComment(context.Background(), prNumber, forgeID, body)
		return commentEditedMsg{localID: localID, body: body, err: err}
	}
}

// --- Update ---

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.mdCache = make(map[mdCacheKey]string)
		m.treeDirty = true
		m.updateViewports()
		if m.comments.inlineActive {
			m.comments.inlineTextarea.SetWidth(m.inlineTextareaWidth() - 4)
		}
		if m.comments.popupActive {
			// Reopen to resize the popup viewport
			m.openCommentPopup()
		}

	case filesLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.errors.set(errCatLoad, 0, msg.err)
		} else {
			m.files = msg.files
			m.nav.cachedTree = buildTree(m.files)
			m.nav.rebuildTreeRows()
			m.nav.buildDiffLines(m.files)
			m.treeDirty = true
			m.updateViewports()
			m.updateDiffContent()
		}

	case existingCommentsMsg:
		if msg.err != nil {
			m.errors.set(errCatLoad, 0, fmt.Errorf("comments: %w", msg.err))
		} else {
			m.comments.existing = msg.comments
			m.rebuildCommentIndex()
			m.updateDiffContent()
		}

	case pendingReviewMsg:
		if msg.err != nil {
			m.errors.set(errCatReview, 0, msg.err)
		} else if msg.reviewID > 0 {
			m.review.ReviewID = msg.reviewID
			m.review.ReviewNodeID = msg.reviewNodeID
			// Restore comments from forge (crash recovery)
			for _, ec := range msg.comments {
				m.review.AddCommentDirect(review.InlineComment{
					Path:       ec.Path,
					Line:       ec.Line,
					Side:       ec.Side,
					Body:       ec.Body,
					ForgeID:   ec.ID,
					SyncStatus: review.SyncComplete,
				})
			}
			m.comments.forgeComments = msg.comments
			m.rebuildCommentIndex()
			m.updateDiffContent()
		} else {
			// No existing pending review — create one on the forge
			return m, m.createPendingReviewCmd()
		}

	case reviewCreatedMsg:
		if msg.err == nil {
			m.review.ReviewID = msg.reviewID
			m.review.ReviewNodeID = msg.reviewNodeID
			// Flush any comments that were added before the review was created
			var cmds []tea.Cmd
			for i := range m.review.Comments {
				if m.review.Comments[i].SyncStatus == review.SyncPending {
					m.review.Comments[i].SyncStatus = review.SyncInFlight
					cmds = append(cmds, m.syncCommentCmd(m.review.Comments[i]))
				}
			}
			if len(cmds) > 0 {
				return m, tea.Batch(cmds...)
			}
		} else {
			m.errors.set(errCatReview, 0, msg.err)
		}

	case commentSyncedMsg:
		if c := m.review.FindByLocalID(msg.localID); c != nil {
			if msg.err != nil {
				c.SyncStatus = review.SyncFailed
				c.SyncError = msg.err
				m.errors.set(errCatCommentSync, msg.localID, msg.err)
			} else {
				c.SyncStatus = review.SyncComplete
				c.ForgeID = msg.forgeID
				m.errors.clear(errCatCommentSync, msg.localID)
			}
			m.rebuildCommentIndex()
			m.updateDiffContent()
		}

	case commentDeletedMsg:
		if msg.err != nil {
			m.errors.set(errCatCommentSync, msg.localID, fmt.Errorf("delete: %w", msg.err))
		} else {
			m.errors.clear(errCatCommentSync, msg.localID)
		}
		m.rebuildCommentIndex()
		m.updateDiffContent()

	case commentEditedMsg:
		if msg.err != nil {
			m.errors.set(errCatCommentSync, msg.localID, fmt.Errorf("edit: %w", msg.err))
		} else {
			m.errors.clear(errCatCommentSync, msg.localID)
			// Update the local comment body
			if c := m.review.FindByLocalID(msg.localID); c != nil {
				c.Body = msg.body
			}
		}
		m.rebuildCommentIndex()
		m.updateDiffContent()

	case viewedFilesMsg:
		if msg.err != nil {
			m.errors.set(errCatViewed, 0, fmt.Errorf("viewed files: %w", msg.err))
		} else if msg.viewed != nil {
			for path := range msg.viewed {
				m.review.ViewedFiles[path] = true
			}
			m.treeDirty = true
			if len(m.files) > 0 {
				m.nav.treeViewport.SetContent(m.renderFileTree())
				m.treeDirty = false
			}
		}

	case markViewedMsg:
		if msg.err != nil {
			m.errors.set(errCatViewed, 0, fmt.Errorf("mark viewed: %w", msg.err))
		}
		// Tree is refreshed optimistically in markTreeItemViewed/markCurrentFileViewed,
		// but also refresh here to pick up any server-side state corrections
		m.treeDirty = true
		if len(m.files) > 0 {
			m.nav.treeViewport.SetContent(m.renderFileTree())
			m.treeDirty = false
		}

	case editorFinishedMsg:
		if msg.err == nil && msg.content != "" {
			m.comments.inlineTextarea.SetValue(msg.content)
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		if m.loading {
			return m, nil
		}
		if m.comments.inlineActive {
			return m.handleInlineKey(msg)
		}
		return m.handleKey(msg)
	}

	// Forward non-key messages to textarea when active
	if m.comments.inlineActive {
		var cmd tea.Cmd
		m.comments.inlineTextarea, cmd = m.comments.inlineTextarea.Update(msg)
		return m, cmd
	}

	return m, nil
}

// --- Viewport helpers ---

func (m *Model) syncViewportToCursor() {
	rendered := m.renderedLineForCursor(m.nav.diffCursor)
	if rendered < m.nav.diffViewport.YOffset {
		m.nav.diffViewport.SetYOffset(rendered)
	} else if rendered >= m.nav.diffViewport.YOffset+m.nav.diffViewport.Height {
		m.nav.diffViewport.SetYOffset(rendered - m.nav.diffViewport.Height + 1)
	}
	m.updateDiffContent()
}

// syncViewportToCursorWithComments ensures both the cursor line and its
// comment block (if any) are visible in the viewport.
func (m *Model) syncViewportToCursorWithComments() {
	rendered := m.renderedLineForCursor(m.nav.diffCursor)
	commentHeight := m.commentBlockHeight(m.nav.diffCursor)

	// Try to show cursor line + comment block. If the block is too tall,
	// at least keep the cursor line at the top of the viewport.
	endLine := rendered + commentHeight
	if endLine >= m.nav.diffViewport.YOffset+m.nav.diffViewport.Height {
		// Need to scroll down — put cursor at top if block fits, otherwise just show cursor
		if commentHeight < m.nav.diffViewport.Height {
			m.nav.diffViewport.SetYOffset(endLine - m.nav.diffViewport.Height + 1)
		} else {
			m.nav.diffViewport.SetYOffset(rendered)
		}
	}
	if rendered < m.nav.diffViewport.YOffset {
		m.nav.diffViewport.SetYOffset(rendered)
	}
	m.updateDiffContent()
}

// renderedLineForCursor computes the rendered line position (in viewport content)
// corresponding to the given diffCursor index, accounting for hunk headers,
// comment blocks, and inter-hunk blank lines.
func (m *Model) renderedLineForCursor(cursor int) int {
	if len(m.files) == 0 || m.nav.fileCursor >= len(m.files) {
		return cursor
	}
	file := m.files[m.nav.fileCursor]
	if file.IsBinary {
		return 0
	}

	rendered := 0
	lineIdx := 0

	for _, hunk := range file.Hunks {
		rendered++ // hunk header line

		for _, line := range hunk.Lines {
			if lineIdx == cursor {
				return rendered
			}
			rendered++ // the diff line itself

			// Count comment lines rendered below this diff line
			if line.NewNum > 0 {
				rendered += m.commentRenderedLines(file.Path, line.NewNum, "RIGHT")
			}
			if line.OldNum > 0 {
				rendered += m.commentRenderedLines(file.Path, line.OldNum, "LEFT")
			}

			lineIdx++
		}
		rendered++ // blank line between hunks
	}

	return rendered
}

// commentBlockHeight returns the total rendered line count for all comments
// attached to the diff line at the given cursor index.
func (m *Model) commentBlockHeight(cursor int) int {
	if cursor < 0 || cursor >= len(m.nav.diffLines) || len(m.files) == 0 || m.nav.fileCursor >= len(m.files) {
		return 0
	}
	dl := m.nav.diffLines[cursor]
	path := m.files[m.nav.fileCursor].Path
	h := 0
	if dl.newLine > 0 {
		h += m.commentRenderedLines(path, dl.newLine, "RIGHT")
	}
	if dl.oldLine > 0 {
		h += m.commentRenderedLines(path, dl.oldLine, "LEFT")
	}
	return h
}

// maxCommentBlockHeight returns the maximum rendered lines for a comment block.
// Comment blocks taller than this are capped and become scrollable.
func (m *Model) maxCommentBlockHeight() int {
	h := m.nav.diffViewport.Height / 3
	if h < 5 {
		h = 5
	}
	return h
}

// commentRenderedLines returns the number of rendered lines for comments
// on a specific file path, line number, and side.
func (m *Model) commentRenderedLines(path string, lineNum int, side string) int {
	existing := m.commentsForLine(path, lineNum, side)
	localPending := m.localPendingForLine(path, lineNum, side)
	totalCount := len(existing) + len(localPending)
	if totalCount == 0 {
		return 0
	}

	ck := commentKey(path, lineNum)
	if !m.comments.expanded[ck] {
		return 1 // folded summary line
	}

	// Expanded: each comment = header(1) + rendered body lines + separator(1)
	contentWidth := m.nav.diffViewport.Width - 8
	if contentWidth < 20 {
		contentWidth = 20
	}
	lines := 0
	for _, c := range existing {
		rendered := m.renderMarkdown(c.Body, contentWidth-2, "")
		lines += 1 + len(strings.Split(rendered, "\n")) + 1
	}
	for _, c := range localPending {
		rendered := m.renderMarkdown(c.Body, contentWidth-2, "")
		lines += 1 + len(strings.Split(rendered, "\n")) + 1
	}

	// Cap to max comment block height
	if maxH := m.maxCommentBlockHeight(); lines > maxH {
		return maxH
	}
	return lines
}

// hasUnsavedWork returns true if the review has any pending comments that would
// be lost on quit. This includes both locally-added comments and comments
// already synced to the forge as part of a draft review.
func (m *Model) hasUnsavedWork() bool {
	return len(m.review.Comments) > 0 || len(m.comments.forgeComments) > 0
}

const commentBoxHeight = 10 // textarea(5) + header(1) + border(2) + help(1) + padding(1)

func (m *Model) updateViewports() {
	treeWidth := 0
	if m.nav.showTree {
		treeWidth = min(50, m.width/3)
	}
	diffWidth := m.width - treeWidth - 1

	headerHeight := 3
	footerHeight := 2
	contentHeight := m.height - headerHeight - footerHeight
	if m.comments.inlineActive {
		contentHeight -= commentBoxHeight
	}
	if contentHeight < 1 {
		contentHeight = 1
	}

	m.nav.treeViewport = viewport.New(treeWidth, contentHeight)
	m.nav.diffViewport = viewport.New(diffWidth, contentHeight)

	if len(m.files) > 0 {
		m.nav.treeViewport.SetContent(m.renderFileTree())
		m.treeDirty = false
		m.updateDiffContent()
	}
}

// openBrowser opens the given URL in the user's default browser.
func openBrowser(url string) tea.Cmd {
	return func() tea.Msg {
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("open", url)
		case "windows":
			cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
		default:
			cmd = exec.Command("xdg-open", url)
		}
		_ = cmd.Start()
		return nil
	}
}

func (m *Model) updateDiffContent() {
	if len(m.files) == 0 || m.nav.fileCursor >= len(m.files) {
		m.nav.diffViewport.SetContent("No files to display")
		return
	}
	file := m.files[m.nav.fileCursor]
	m.nav.diffViewport.SetContent(m.renderDiffWithCursor(&file))
	if m.treeDirty {
		m.nav.treeViewport.SetContent(m.renderFileTree())
		m.treeDirty = false
	}
}

// autoFollowFile adjusts tree collapse state when fileCursor changes.
// Collapses the old file's folder (if different) and expands the new file's folder.
func (m *Model) autoFollowFile(oldFileIdx, newFileIdx int) {
	if m.nav.cachedTree == nil {
		return
	}

	oldDirs := findAncestorDirs(m.nav.cachedTree, oldFileIdx)
	newDirs := findAncestorDirs(m.nav.cachedTree, newFileIdx)

	// Build sets for quick lookup
	newDirSet := make(map[string]bool, len(newDirs))
	for _, d := range newDirs {
		newDirSet[d.dirPath] = true
	}

	// Collapse old dirs that are not ancestors of the new file
	for _, d := range oldDirs {
		if !newDirSet[d.dirPath] {
			m.nav.collapsedDirs[d.dirPath] = true
		}
	}

	// Expand new file's ancestor dirs
	for _, d := range newDirs {
		delete(m.nav.collapsedDirs, d.dirPath)
	}

	m.nav.rebuildTreeRows()
	m.nav.syncTreeCursorToFileCursor()
	m.nav.treeViewport.SetContent(m.renderFileTree())
}

// --- View ---

func (m Model) View() string {
	if m.width == 0 {
		return "Loading...\n"
	}

	var b strings.Builder

	// Header
	header := styles.Title.Render(fmt.Sprintf("PR #%d: %s", m.pr.Number, m.pr.Title))

	totalPending := len(m.review.Comments) + len(m.comments.forgeComments)
	pendingCount := ""
	if totalPending > 0 {
		pendingCount = lipgloss.NewStyle().Foreground(styles.Warning).
			Render(fmt.Sprintf("  [%d pending]", totalPending))
	}

	viewedCount := len(m.review.ViewedFiles)
	viewedLabel := ""
	if viewedCount > 0 {
		viewedLabel = lipgloss.NewStyle().Foreground(styles.Success).
			Render(fmt.Sprintf("  [%d/%d viewed]", viewedCount, len(m.files)))
	}

	syncLabel := ""
	inflight := m.review.InFlightCount()
	if inflight > 0 {
		syncLabel = lipgloss.NewStyle().Foreground(styles.Secondary).
			Render(fmt.Sprintf("  [syncing %d...]", inflight))
	}

	b.WriteString(header + pendingCount + viewedLabel + syncLabel + "\n")

	if m.loading {
		b.WriteString(m.spinner.View() + " Loading diff...\n")
		return b.String()
	}

	if err := m.errors.get(errCatLoad); err != nil {
		b.WriteString(lipgloss.NewStyle().Foreground(styles.Danger).
			Render(fmt.Sprintf("Error: %v", err)) + "\n")
		return b.String()
	}

	// Show sync errors (review, viewed, comment sync)
	b.WriteString(m.errors.renderSyncErrors())

	// File name bar
	if len(m.files) > 0 {
		fileName := m.files[m.nav.fileCursor].Path
		viewed := ""
		if m.review.ViewedFiles[fileName] {
			viewed = " ✓"
		}
		b.WriteString(styles.StatusBar.Render(
			fmt.Sprintf(" %s (%d/%d)%s ", fileName, m.nav.fileCursor+1, len(m.files), viewed)) + "\n")
	}

	// Main content
	if m.nav.showTree {
		separator := lipgloss.NewStyle().Foreground(styles.Muted).Render("│")
		split := lipgloss.JoinHorizontal(lipgloss.Top, m.nav.treeViewport.View(), separator, m.nav.diffViewport.View())
		b.WriteString(split + "\n")
	} else {
		b.WriteString(m.nav.diffViewport.View() + "\n")
	}

	// Inline comment box (below the diff)
	if m.comments.inlineActive {
		b.WriteString(m.renderCommentBox() + "\n")
	}

	// Input mode bars
	if m.search.gotoActive {
		prompt := lipgloss.NewStyle().Bold(true).Render("Go to line: ")
		b.WriteString(prompt + m.search.gotoInput + "█")
	} else if m.search.active {
		prompt := lipgloss.NewStyle().Bold(true).Render("/")
		b.WriteString(prompt + m.search.input + "█")
	} else if m.search.filterActive {
		prompt := lipgloss.NewStyle().Bold(true).Render("Filter files: ")
		b.WriteString(prompt + m.search.filterInput + "█\n")
		// Show filtered file list
		for i, idx := range m.search.filterFiles {
			if i >= 10 {
				b.WriteString(styles.HelpStyle.Render(fmt.Sprintf("  ... and %d more", len(m.search.filterFiles)-10)))
				break
			}
			cursor := "  "
			if i == m.search.filterCursor {
				cursor = "> "
			}
			b.WriteString(cursor + m.files[idx].Path + "\n")
		}
	} else if m.nav.pendingG {
		// Show g-prefix legend while waiting for second key
		keyStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.Cyan)
		b.WriteString(styles.HelpStyle.Render("g ") +
			keyStyle.Render("h") + styles.HelpStyle.Render(" hunk  ") +
			keyStyle.Render("f") + styles.HelpStyle.Render(" file  ") +
			keyStyle.Render("F") + styles.HelpStyle.Render(" unviewed  ") +
			keyStyle.Render("c") + styles.HelpStyle.Render(" comment  ") +
			keyStyle.Render("C") + styles.HelpStyle.Render(" commented file  ") +
			keyStyle.Render("0-9") + styles.HelpStyle.Render(" line"))
	} else if m.comments.cursor >= 0 && m.nav.focus == FocusDiff {
		// Comment selection mode footer
		refs := m.commentRefsAtCursor()
		var helpParts []string
		helpParts = append(helpParts, "j/k select comment")
		helpParts = append(helpParts, "enter view all")
		helpParts = append(helpParts, "r reply")
		if m.comments.cursor < len(refs) && refs[m.comments.cursor].isLocal {
			helpParts = append(helpParts, "e edit")
			helpParts = append(helpParts, "d delete")
		}
		helpParts = append(helpParts, "esc back")
		b.WriteString(styles.HelpStyle.Render(strings.Join(helpParts, "  ")))
	} else if m.confirmQuit {
		pendingCount := len(m.review.Comments) + len(m.comments.forgeComments)
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(styles.Warning).
			Render(fmt.Sprintf("You have %d pending comment(s). Press q again to quit.", pendingCount)))
	} else if !m.comments.inlineActive {
		// Footer
		var helpParts []string
		if m.nav.focus == FocusFileTree {
			helpParts = append(helpParts, "j/k navigate")
			helpParts = append(helpParts, "enter select")
			helpParts = append(helpParts, "tab fold")
			helpParts = append(helpParts, "S-tab fold all")
			helpParts = append(helpParts, "gf file")
			helpParts = append(helpParts, "/ filter")
			helpParts = append(helpParts, "m viewed")
			helpParts = append(helpParts, "e tree")
			helpParts = append(helpParts, "ctrl+s submit")
			helpParts = append(helpParts, "? help")
		} else {
			helpParts = append(helpParts, "j/k scroll")
			helpParts = append(helpParts, "g… go to")
			helpParts = append(helpParts, "G bottom")
			helpParts = append(helpParts, "/ search")
			helpParts = append(helpParts, "n/p next/prev")
			helpParts = append(helpParts, "c comment")
			helpParts = append(helpParts, "e tree")
			helpParts = append(helpParts, "m viewed")
			if m.nav.visualMode {
				helpParts = append(helpParts, "(SELECT)")
			}
			if m.search.query != "" {
				helpParts = append(helpParts, fmt.Sprintf("[/%s]", m.search.query))
			}
			helpParts = append(helpParts, "ctrl+s submit")
			helpParts = append(helpParts, "? help")
		}

		// Show active cycler indicator
		if m.nav.activeCycler != 0 {
			label := cyclerLabel(m.nav.activeCycler)
			helpParts = append(helpParts,
				lipgloss.NewStyle().Foreground(styles.Warning).Render(fmt.Sprintf("[n/N: %s]", label)))
		}

		b.WriteString(styles.HelpStyle.Render(strings.Join(helpParts, "  ")))
	}

	result := b.String()

	// Overlay popups if active
	if m.showHelp {
		result = m.overlayHelpPopup(result)
	}
	if m.comments.popupActive {
		result = m.overlayCommentPopup(result)
	}

	return result
}

// cyclerLabel returns a human-readable label for the active cycler.
func cyclerLabel(c rune) string {
	switch c {
	case 'f':
		return "file"
	case 'F':
		return "unviewed"
	case 'c':
		return "comment"
	case 'C':
		return "commented file"
	case 'h':
		return "hunk"
	case '/':
		return "search"
	default:
		return ""
	}
}

// overlayHelpPopup renders a centered help popup over the existing content.
func (m Model) overlayHelpPopup(base string) string {
	return m.overlayGeneric(base, m.renderHelpPopup())
}

// renderHelpPopup builds the help popup content.
func (m Model) renderHelpPopup() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.Primary)
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.Cyan)
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(styles.Current.DiffContext))
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.Warning).MarginTop(1)

	type binding struct {
		key  string
		desc string
	}

	sections := []struct {
		title    string
		bindings []binding
	}{
		{
			title: "Navigation",
			bindings: []binding{
				{"j / k", "Scroll down / up"},
				{"ctrl+d / ctrl+u", "Page down / up"},
				{"G", "Bottom of file"},
				{"n / p", "Next / prev in active cycler"},
			},
		},
		{
			title: "Go to (g prefix)",
			bindings: []binding{
				{"gh", "Next hunk (then n/p cycle hunks)"},
				{"gf", "Next file (then n/p cycle files)"},
				{"gF", "Next unviewed file"},
				{"gc", "Next comment (expand + select)"},
				{"gC", "Next file with comments"},
				{"g<number>", "Go to line number"},
			},
		},
		{
			title: "Search",
			bindings: []binding{
				{"/ (text)", "Search in file (then n/p cycle matches)"},
				{"ctrl+p", "Filter files"},
			},
		},
		{
			title: "Review",
			bindings: []binding{
				{"c", "Add comment"},
				{"enter", "View comments in popup"},
				{"dd", "Delete pending comment"},
				{"de", "Edit pending comment"},
				{"space", "Start visual selection"},
				{"m", "Toggle mark as viewed"},
				{"tab / S-tab", "Toggle / toggle all comments"},
				{"ctrl+s", "Submit review"},
			},
		},
		{
			title: "Comment selection (j/k on expanded comments)",
			bindings: []binding{
				{"j / k", "Next / prev comment"},
				{"enter", "View all comments in popup"},
				{"r", "Reply to comment"},
				{"e", "Edit pending comment"},
				{"d", "Delete pending comment"},
				{"esc", "Deselect comment"},
			},
		},
		{
			title: "Other",
			bindings: []binding{
				{"e", "Toggle file tree"},
				{"w", "Open in browser"},
				{"ctrl+e", "Open in $EDITOR"},
				{"esc", "Back"},
				{"q", "Quit"},
			},
		},
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render("  Keybindings") + "\n")

	for _, section := range sections {
		b.WriteString(sectionStyle.Render("  "+section.title) + "\n")
		for _, bind := range section.bindings {
			key := keyStyle.Render(fmt.Sprintf("  %-20s", bind.key))
			desc := descStyle.Render(bind.desc)
			b.WriteString(key + desc + "\n")
		}
	}

	b.WriteString("\n" + styles.HelpStyle.Render("  Press any key to close"))

	content := b.String()

	// Find the widest line for the box
	maxWidth := 0
	for _, line := range strings.Split(content, "\n") {
		if w := lipgloss.Width(line); w > maxWidth {
			maxWidth = w
		}
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.Primary).
		Background(lipgloss.Color(styles.Current.BgOverlay)).
		Padding(0, 1).
		Width(maxWidth + 2)

	return boxStyle.Render(content)
}
