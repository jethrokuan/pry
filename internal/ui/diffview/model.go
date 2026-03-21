package diffview

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jethrokuan/pry/internal/appctx"
	"github.com/jethrokuan/pry/internal/clipboard"
	"github.com/jethrokuan/pry/internal/ui/components/helppopup"
	"github.com/jethrokuan/pry/internal/diff"
	"github.com/jethrokuan/pry/internal/review"
	"github.com/jethrokuan/pry/internal/ui/styles"
)

// --- Messages ---

type SubmitReviewMsg struct{}
type BackMsg struct{}

// PRBodyLoadedMsg carries the full PR data after async fetch.
type PRBodyLoadedMsg struct {
	PR  *review.PullRequest
	Err error
}

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

type clipboardImageMsg struct {
	data []byte
	err  error
}

type imageUploadedMsg struct {
	url string
	err error
}

type flashExpiredMsg struct{}

type mentionableUsersMsg struct {
	users []string
	err   error
}

// UserIdentityMsg carries the resolved user identity from the app layer.
type UserIdentityMsg struct {
	Identity *review.UserIdentity
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
	Up          key.Binding
	Down        key.Binding
	PageUp      key.Binding
	PageDown    key.Binding
	ToggleTree  key.Binding
	NextFile    key.Binding
	PrevFile    key.Binding
	NextHunk    key.Binding
	PrevHunk    key.Binding
	NextComment key.Binding
	PrevComment key.Binding
	// DeleteComment and EditComment are only active in comment-select mode
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
	NextSearch    key.Binding
	PrevSearch    key.Binding
	FilterFile    key.Binding
	Help          key.Binding
	Info          key.Binding
	NarrowPrefix  key.Binding
	JumpBack      key.Binding
	JumpForward   key.Binding
	CopyLink      key.Binding
}

var keys = KeyMap{
	Up:            key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:          key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	PageUp:        key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("ctrl+u", "page up")),
	PageDown:      key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "page down")),
	ToggleTree:    key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "toggle tree")),
	NextFile:      key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "next file")),
	PrevFile:      key.NewBinding(key.WithKeys("F"), key.WithHelp("F", "prev file")),
	NextHunk:      key.NewBinding(key.WithKeys("h"), key.WithHelp("h", "next hunk")),
	PrevHunk:      key.NewBinding(key.WithKeys("H"), key.WithHelp("H", "prev hunk")),
	NextComment:   key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "next comment")),
	PrevComment:   key.NewBinding(key.WithKeys("C"), key.WithHelp("C", "prev comment")),
	DeleteComment: key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete comment")),
	EditComment:   key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit comment")),
	Reply:         key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reply")),
	SelectLine:    key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "select")),
	Submit:        key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "submit review")),
	ToggleComment: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "toggle fold")),
	FoldComment:   key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("S-tab", "toggle all folds")),
	MarkViewed:    key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "mark viewed")),
	OpenInBrowser: key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "open in browser")),
	Editor:        key.NewBinding(key.WithKeys("ctrl+e"), key.WithHelp("ctrl+e", "open in editor")),
	Back:          key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	Quit:          key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
	Enter:         key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "action")),
	Search:        key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
	NextSearch:    key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next match")),
	PrevSearch:    key.NewBinding(key.WithKeys("N"), key.WithHelp("N", "prev match")),
	FilterFile:    key.NewBinding(key.WithKeys("ctrl+p"), key.WithHelp("ctrl+p", "filter files")),
	Help:          key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Info:          key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "PR info")),
	NarrowPrefix:  key.NewBinding(key.WithKeys("T"), key.WithHelp("T", "filter prefix")),
	JumpBack:      key.NewBinding(key.WithKeys("ctrl+o"), key.WithHelp("ctrl+o", "jump back")),
	JumpForward:   key.NewBinding(key.WithKeys("ctrl+i"), key.WithHelp("ctrl+i", "jump forward")),
	CopyLink:      key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy link")),
}

// Inline comment key bindings (when textarea is active)
var inlineKeys = struct {
	Save       key.Binding
	Cancel     key.Binding
	ToggleMode key.Binding
	OpenEditor key.Binding
	PasteImage key.Binding
}{
	Save:       key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "save")),
	Cancel:     key.NewBinding(key.WithKeys("esc", "ctrl+c"), key.WithHelp("esc", "cancel")),
	ToggleMode: key.NewBinding(key.WithKeys("ctrl+t"), key.WithHelp("ctrl+t", "toggle mode")),
	OpenEditor: key.NewBinding(key.WithKeys("ctrl+e"), key.WithHelp("ctrl+e", "open $EDITOR")),
	PasteImage: key.NewBinding(key.WithKeys("ctrl+v"), key.WithHelp("ctrl+v", "paste image")),
}

// --- Model ---

type Model struct {
	ctx   *appctx.Context
	pr    *review.PullRequest
	files []diff.DiffFile

	nav      DiffNav
	comments CommentPanel
	search   SearchBar
	filter   FileFilter

	width    int
	height   int
	loading  bool
	spinner  spinner.Model
	showHelp bool

	errors errorStore // unified error tracking for all async operations

	confirmQuit        bool // true when waiting for second quit key to confirm
	narrowPrefixActive bool // true when waiting for second key after 'T'

	// PR info popup
	prInfoActive   bool
	prInfoViewport viewport.Model

	// Flash message (auto-dismissing status text)
	flashMsg string

	// Caches
	mdCache  map[mdCacheKey]string // rendered markdown cache
	treeDirty bool                 // true when file tree needs re-rendering
}

type diffLineInfo struct {
	fileIdx   int
	hunkIdx   int
	lineIdx   int
	newLine   int
	oldLine   int
	lineType  diff.LineType
	content   string
	collapsed bool // true for collapsed hunk placeholder lines
}

// --- Constructor ---

// Option configures the Model at construction time.
type Option func(*Model)

// WithUserIdentity sets the owner filter identities from a resolved user identity.
// When provided, the owner filter defaults to on (loading CODEOWNERS lazily).
func WithUserIdentity(id *review.UserIdentity) Option {
	return func(m *Model) {
		if id == nil {
			return
		}
		m.filter.ownerIdentities = make([]string, 0, 1+len(id.Teams))
		m.filter.ownerIdentities = append(m.filter.ownerIdentities, "@"+id.Login)
		for _, t := range id.Teams {
			m.filter.ownerIdentities = append(m.filter.ownerIdentities, "@"+t)
		}
		m.filter.ownerEnabled = true
		m.filter.codeowners = loadCodeowners()
		if m.filter.codeowners == nil {
			// No CODEOWNERS file — disable owner filter silently
			m.filter.ownerEnabled = false
		}
	}
}

// WithOwnerFilterDisabled explicitly disables the owner filter regardless of identity.
func WithOwnerFilterDisabled() Option {
	return func(m *Model) {
		m.filter.ownerEnabled = false
	}
}

func New(ctx *appctx.Context, pr *review.PullRequest, opts ...Option) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	m := Model{
		ctx:               ctx,
		pr:                pr,
		loading: true,
		spinner: s,
		errors:  newErrorStore(),
		mdCache: make(map[mdCacheKey]string),
		treeDirty:         true,
		nav: DiffNav{
			showTree:       true,
			focus:          FocusDiff,
			collapsedDirs:  make(map[string]bool),
			collapsedHunks: make(map[string]bool),
			activeCycler:   'h',
		},
		comments: CommentPanel{
			expanded: make(map[string]bool),
			cursor:   -1,
		},
	}

	for _, opt := range opts {
		opt(&m)
	}

	// Apply user identity from context if available and not already set by options
	if len(m.filter.ownerIdentities) == 0 && ctx.UserIdentity != nil {
		WithUserIdentity(ctx.UserIdentity)(&m)
	}

	return m
}

// --- Init ---

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.loadFiles(),
		m.loadComments(),
		m.loadPendingReview(),
		m.loadViewedFiles(),
		m.loadMentionableUsers(),
		m.spinner.Tick,
	)
}

func (m Model) loadMentionableUsers() tea.Cmd {
	return func() tea.Msg {
		users, err := m.ctx.Svc.ListMentionableUsers(context.Background())
		return mentionableUsersMsg{users: users, err: err}
	}
}

func (m Model) loadViewedFiles() tea.Cmd {
	if m.pr.NodeID == "" {
		return nil
	}
	return func() tea.Msg {
		viewed, err := m.ctx.Svc.FetchViewedFiles(context.Background(), m.pr.NodeID)
		return viewedFilesMsg{viewed: viewed, err: err}
	}
}

func (m Model) loadFiles() tea.Cmd {
	return func() tea.Msg {
		files, err := m.ctx.Svc.FetchDiffFiles(context.Background(), m.pr.Number)
		return filesLoadedMsg{files: files, err: err}
	}
}

func (m Model) loadComments() tea.Cmd {
	return func() tea.Msg {
		comments, err := m.ctx.Svc.FetchExistingComments(context.Background(), m.pr.Number)
		return existingCommentsMsg{comments: comments, err: err}
	}
}

func (m Model) loadPendingReview() tea.Cmd {
	return func() tea.Msg {
		reviewID, nodeID, comments, err := m.ctx.Svc.FetchPendingReview(context.Background(), m.pr.Number)
		return pendingReviewMsg{reviewID: reviewID, reviewNodeID: nodeID, comments: comments, err: err}
	}
}

func (m Model) createPendingReviewCmd() tea.Cmd {
	return func() tea.Msg {
		id, nodeID, err := m.ctx.Svc.CreatePendingReview(context.Background(), m.pr.Number)
		return reviewCreatedMsg{reviewID: id, reviewNodeID: nodeID, err: err}
	}
}

func (m Model) syncCommentCmd(c review.InlineComment) tea.Cmd {
	if m.pr.PendingReview.ReviewNodeID == "" {
		return nil
	}
	// Capture values to avoid reading shared state from goroutine
	svc := m.ctx.Svc
	reviewNodeID := m.pr.PendingReview.ReviewNodeID
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
	svc := m.ctx.Svc
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
	svc := m.ctx.Svc
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
		if m.prInfoActive {
			m.openPRInfoPopup()
		}

	case filesLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.errors.set(errCatLoad, 0, msg.err)
		} else {
			m.files = msg.files
			m.filter.recompute(m.files)
			m.nav.cachedTree = buildTree(m.files, m.filter.includedFiles)
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
			m.setExistingComments(msg.comments)
			m.updateDiffContent()
		}

	case pendingReviewMsg:
		if msg.err != nil {
			m.errors.set(errCatReview, 0, msg.err)
		} else if msg.reviewID > 0 {
			m.pr.PendingReview.ReviewID = msg.reviewID
			m.pr.PendingReview.ReviewNodeID = msg.reviewNodeID
			// Restore comments from forge (crash recovery)
			m.restoreForgeComments(msg.comments, msg.comments)
			m.updateDiffContent()
		}
		// No existing pending review — that's fine, we'll create one lazily
		// when the user actually takes a review action (e.g., adds a comment).

	case reviewCreatedMsg:
		if msg.err == nil {
			m.pr.PendingReview.ReviewID = msg.reviewID
			m.pr.PendingReview.ReviewNodeID = msg.reviewNodeID
			// Flush any comments that were added before the review was created
			var cmds []tea.Cmd
			for i := range m.pr.PendingReview.Comments {
				if m.pr.PendingReview.Comments[i].SyncStatus == review.SyncPending {
					m.pr.PendingReview.Comments[i].SyncStatus = review.SyncInFlight
					cmds = append(cmds, m.syncCommentCmd(m.pr.PendingReview.Comments[i]))
				}
			}
			if len(cmds) > 0 {
				return m, tea.Batch(cmds...)
			}
		} else {
			m.errors.set(errCatReview, 0, msg.err)
		}

	case commentSyncedMsg:
		if msg.err != nil {
			m.updateLocalComment(msg.localID, func(c *review.InlineComment) {
				c.SyncStatus = review.SyncFailed
				c.SyncError = msg.err
			})
			m.errors.set(errCatCommentSync, msg.localID, msg.err)
		} else {
			m.updateLocalComment(msg.localID, func(c *review.InlineComment) {
				c.SyncStatus = review.SyncComplete
				c.ForgeID = msg.forgeID
			})
			m.errors.clear(errCatCommentSync, msg.localID)
		}
		m.updateDiffContent()

	case commentDeletedMsg:
		if msg.err != nil {
			m.errors.set(errCatCommentSync, msg.localID, fmt.Errorf("delete: %w", msg.err))
		} else {
			m.errors.clear(errCatCommentSync, msg.localID)
		}
		m.updateDiffContent()

	case commentEditedMsg:
		if msg.err != nil {
			m.errors.set(errCatCommentSync, msg.localID, fmt.Errorf("edit: %w", msg.err))
		} else {
			m.errors.clear(errCatCommentSync, msg.localID)
			// Update the local comment body
			m.updateLocalComment(msg.localID, func(c *review.InlineComment) {
				c.Body = msg.body
			})
		}
		m.updateDiffContent()

	case viewedFilesMsg:
		if msg.err != nil {
			m.errors.set(errCatViewed, 0, fmt.Errorf("viewed files: %w", msg.err))
		} else if msg.viewed != nil {
			for path := range msg.viewed {
				m.pr.PendingReview.ViewedFiles[path] = true
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

	case UserIdentityMsg:
		if msg.Identity != nil && len(m.filter.ownerIdentities) == 0 {
			m.filter.ownerIdentities = make([]string, 0, 1+len(msg.Identity.Teams))
			m.filter.ownerIdentities = append(m.filter.ownerIdentities, "@"+msg.Identity.Login)
			for _, t := range msg.Identity.Teams {
				m.filter.ownerIdentities = append(m.filter.ownerIdentities, "@"+t)
			}
			m.filter.ownerEnabled = true
			if m.filter.codeowners == nil {
				m.filter.codeowners = loadCodeowners()
			}
			if m.filter.codeowners == nil {
				m.filter.ownerEnabled = false
			}
			if m.filter.ownerEnabled && len(m.files) > 0 {
				m.applyFilters()
			}
		}

	case PRBodyLoadedMsg:
		if msg.Err == nil && msg.PR != nil {
			// Preserve review state when updating PR metadata
			pendingReview := m.pr.PendingReview
			existingComments := m.pr.ExistingComments
			*m.pr = *msg.PR
			m.pr.PendingReview = pendingReview
			m.pr.ExistingComments = existingComments
			if m.prInfoActive {
				m.openPRInfoPopup()
			}
			// Viewed files may not have been loaded yet if PRNodeID was
			// empty at Init time (e.g. CLI launch with just a PR number).
			// Now that app.go has backfilled the node ID, try again.
			if len(m.pr.PendingReview.ViewedFiles) == 0 && m.pr.NodeID != "" {
				return m, m.loadViewedFiles()
			}
		}

	case mentionableUsersMsg:
		if msg.err == nil {
			m.comments.mentionAll = msg.users
		}

	case clipboardImageMsg:
		if msg.err != nil {
			return m, m.setFlash("Clipboard error: " + msg.err.Error())
		} else if msg.data == nil {
			return m, m.setFlash("No image in clipboard")
		} else {
			flashCmd := m.setFlash("Uploading image...")
			return m, tea.Batch(flashCmd, m.uploadImageCmd(msg.data))
		}

	case imageUploadedMsg:
		if msg.err != nil {
			return m, m.setFlash("Upload failed: " + msg.err.Error())
		} else if m.comments.inlineActive {
			// Insert markdown image link at current cursor position
			mdLink := fmt.Sprintf("![image](%s)", msg.url)
			m.comments.inlineTextarea.InsertString(mdLink)
			return m, m.setFlash("Image uploaded")
		}

	case copyResultMsg:
		if msg.err != nil {
			return m, m.setFlash("Copy failed: " + msg.err.Error())
		}
		return m, m.setFlash(fmt.Sprintf("Copied %s", msg.url))

	case editorFinishedMsg:
		if msg.err == nil && msg.content != "" {
			m.comments.inlineTextarea.SetValue(msg.content)
		}

	case flashExpiredMsg:
		m.flashMsg = ""

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.MouseWheelMsg:
		// Forward mouse wheel events to active popup viewports for scroll support
		if m.prInfoActive {
			var cmd tea.Cmd
			m.prInfoViewport, cmd = m.prInfoViewport.Update(msg)
			return m, cmd
		}
		if m.comments.popupActive {
			var cmd tea.Cmd
			m.comments.popupViewport, cmd = m.comments.popupViewport.Update(msg)
			return m, cmd
		}
		return m, nil

	case tea.KeyPressMsg:
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
	if rendered < m.nav.diffViewport.YOffset() {
		m.nav.diffViewport.SetYOffset(rendered)
	} else if rendered >= m.nav.diffViewport.YOffset()+m.nav.diffViewport.Height() {
		m.nav.diffViewport.SetYOffset(rendered - m.nav.diffViewport.Height() + 1)
	}
	m.updateDiffContent()
}

// syncViewportToCursorWithComments centers the cursor line and its comment
// block in the viewport. If the combined block is taller than the viewport,
// it places the cursor line at the top instead.
func (m *Model) syncViewportToCursorWithComments() {
	rendered := m.renderedLineForCursor(m.nav.diffCursor)
	commentHeight := m.commentBlockHeight(m.nav.diffCursor)

	// Total height of the diff line (1) + its comment block.
	totalHeight := 1 + commentHeight
	vpHeight := m.nav.diffViewport.Height()

	if totalHeight >= vpHeight {
		// Block is taller than viewport — keep cursor line at top.
		m.nav.diffViewport.SetYOffset(rendered)
	} else {
		// Center the block (diff line + comments) within the viewport.
		topMargin := (vpHeight - totalHeight) / 2
		offset := rendered - topMargin
		if offset < 0 {
			offset = 0
		}
		m.nav.diffViewport.SetYOffset(offset)
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
	h := m.nav.diffViewport.Height() / 3
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
	contentWidth := m.nav.diffViewport.Width() - 8
	if contentWidth < 20 {
		contentWidth = 20
	}
	lines := 0
	for _, c := range existing {
		rendered := m.renderMarkdown(c.Body, contentWidth-2)
		lines += 1 + len(strings.Split(rendered, "\n")) + 1
	}
	for _, c := range localPending {
		rendered := m.renderMarkdown(c.Body, contentWidth-2)
		lines += 1 + len(strings.Split(rendered, "\n")) + 1
	}

	// Cap to max comment block height
	if maxH := m.maxCommentBlockHeight(); lines > maxH {
		return maxH
	}
	return lines
}

// setFlash sets a flash message and returns a command that clears it after a delay.
func (m *Model) setFlash(msg string) tea.Cmd {
	m.flashMsg = msg
	return tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg {
		return flashExpiredMsg{}
	})
}

// hasUnsavedWork returns true if the review has any pending comments that would
// be lost on quit. This includes both locally-added comments and comments
// already synced to the forge as part of a draft review.
func (m *Model) hasUnsavedWork() bool {
	return len(m.pr.PendingReview.Comments) > 0 || len(m.comments.forgeComments) > 0
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

	m.nav.treeViewport = viewport.New(viewport.WithWidth(treeWidth), viewport.WithHeight(contentHeight))
	m.nav.diffViewport = viewport.New(viewport.WithWidth(diffWidth), viewport.WithHeight(contentHeight))

	if len(m.files) > 0 {
		m.nav.treeViewport.SetContent(m.renderFileTree())
		m.treeDirty = false
		m.updateDiffContent()
	}
}

// copyForgeLink builds a GitHub permalink for the current file+line (or selection)
// and copies it to the system clipboard.
func (m *Model) copyForgeLink() tea.Cmd {
	if len(m.files) == 0 || m.nav.fileCursor >= len(m.files) {
		return m.setFlash("No file to copy link for")
	}

	file := m.files[m.nav.fileCursor]
	sha := m.pr.HeadSHA
	owner := m.ctx.Svc.RepoOwner()
	repo := m.ctx.Svc.RepoName()

	// Build base blob URL
	url := fmt.Sprintf("https://github.com/%s/%s/blob/%s/%s", owner, repo, sha, file.Path)

	// Add line anchor
	if m.nav.diffCursor < len(m.nav.diffLines) {
		if m.nav.visualMode {
			// Selection active — compute line range
			startIdx := min(m.nav.visualStart, m.nav.visualEnd)
			endIdx := max(m.nav.visualStart, m.nav.visualEnd)
			startLine := m.resolveLineNumber(startIdx)
			endLine := m.resolveLineNumber(endIdx)
			if startLine > 0 && endLine > 0 {
				if startLine == endLine {
					url += fmt.Sprintf("#L%d", startLine)
				} else {
					url += fmt.Sprintf("#L%d-L%d", startLine, endLine)
				}
			}
		} else {
			// Single line
			line := m.resolveLineNumber(m.nav.diffCursor)
			if line > 0 {
				url += fmt.Sprintf("#L%d", line)
			}
		}
	}

	return m.copyToClipboard(url)
}

// resolveLineNumber returns the new-side line number for a diff line index,
// falling back to old-side if the line is a deletion.
func (m *Model) resolveLineNumber(idx int) int {
	if idx >= len(m.nav.diffLines) {
		return 0
	}
	dl := m.nav.diffLines[idx]
	if dl.newLine > 0 {
		return dl.newLine
	}
	return dl.oldLine
}

// copyToClipboard copies text to the system clipboard and shows a flash message.
func (m *Model) copyToClipboard(text string) tea.Cmd {
	return func() tea.Msg {
		if err := clipboard.WriteText(text); err != nil {
			return copyResultMsg{err: err}
		}
		return copyResultMsg{url: text}
	}
}

type copyResultMsg struct {
	url string
	err error
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
		if err := cmd.Start(); err != nil {
			slog.Warn("failed to open URL in browser", "url", url, "error", err)
		}
		return nil
	}
}

func (m *Model) updateDiffContent() {
	if len(m.files) == 0 || m.nav.fileCursor >= len(m.files) {
		m.nav.diffViewport.SetContent("No files to display")
		if m.treeDirty {
			m.nav.treeViewport.SetContent(m.renderFileTree())
			m.treeDirty = false
		}
		return
	}
	if m.filter.isActive() && m.filter.filteredCount == 0 {
		m.nav.diffViewport.SetContent("No matching files.")
		if m.treeDirty {
			m.nav.treeViewport.SetContent(m.renderFileTree())
			m.treeDirty = false
		}
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

	totalPending := len(m.pr.PendingReview.Comments) + len(m.comments.forgeComments)
	pendingCount := ""
	if totalPending > 0 {
		pendingCount = lipgloss.NewStyle().Foreground(styles.Warning).
			Render(fmt.Sprintf("  [%d pending]", totalPending))
	}

	viewedCount := len(m.pr.PendingReview.ViewedFiles)
	viewedLabel := ""
	if viewedCount > 0 {
		viewedLabel = lipgloss.NewStyle().Foreground(styles.Success).
			Render(fmt.Sprintf("  [%d/%d viewed]", viewedCount, len(m.files)))
	}

	syncLabel := ""
	inflight := m.pr.PendingReview.InFlightCount()
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
		if m.pr.PendingReview.ViewedFiles[fileName] {
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
	} else if m.filter.regexActive {
		prompt := lipgloss.NewStyle().Bold(true).Render("Narrow (regex): ")
		b.WriteString(prompt + m.filter.regexInput + "█")
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
		pendingCount := len(m.pr.PendingReview.Comments) + len(m.comments.forgeComments)
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(styles.Warning).
			Render(fmt.Sprintf("You have %d pending comment(s). Press ctrl+c again to quit.", pendingCount)))
	} else if !m.comments.inlineActive {
		// Footer
		var helpParts []string
		if m.nav.focus == FocusFileTree {
			helpParts = append(helpParts, "j/k navigate")
			helpParts = append(helpParts, "enter select")
			helpParts = append(helpParts, "tab fold")
			helpParts = append(helpParts, "S-tab fold all")
			helpParts = append(helpParts, "f/F file")
			helpParts = append(helpParts, "/ filter")
			helpParts = append(helpParts, "^f narrow")
			if len(m.filter.ownerIdentities) > 0 {
				helpParts = append(helpParts, "^o owner")
			}
			if m.filter.isActive() {
				helpParts = append(helpParts, "^x clear")
			}
			helpParts = append(helpParts, "T filter")
			helpParts = append(helpParts, "m viewed")
			helpParts = append(helpParts, "t tree")
			helpParts = append(helpParts, "i info")
			helpParts = append(helpParts, "ctrl+s submit")
			helpParts = append(helpParts, "? help")
		} else {
			helpParts = append(helpParts, "j/k scroll")
			helpParts = append(helpParts, "f/F file")
			helpParts = append(helpParts, "h/H hunk")
			helpParts = append(helpParts, "c/C comment")
			helpParts = append(helpParts, "/ search")
			helpParts = append(helpParts, "T filter")
			helpParts = append(helpParts, "enter comment")
			helpParts = append(helpParts, "t tree")
			helpParts = append(helpParts, "m viewed")
			if m.nav.visualMode {
				helpParts = append(helpParts, "(SELECT)")
			}
			if m.search.query != "" {
				helpParts = append(helpParts, fmt.Sprintf("[/%s n/N]", m.search.query))
			}
			helpParts = append(helpParts, "i info")
			helpParts = append(helpParts, "ctrl+s submit")
			helpParts = append(helpParts, "? help")
		}

		// Always show position counter for the active object type
		{
			label, idx, total := m.currentPosition()
			if total > 0 {
				cyclerText := fmt.Sprintf("[%s %d/%d]", label, idx, total)
				helpParts = append(helpParts,
					lipgloss.NewStyle().Foreground(styles.Warning).Render(cyclerText))
			}
		}

		// Show flash message if active
		if m.flashMsg != "" {
			helpParts = append(helpParts,
				lipgloss.NewStyle().Foreground(styles.Cyan).Italic(true).Render(m.flashMsg))
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
	if m.prInfoActive {
		result = m.overlayPRInfoPopup(result)
	}

	return result
}

// currentPosition dynamically computes the position counter for the active navigation type.
func (m Model) currentPosition() (label string, index int, total int) {
	switch m.nav.activeCycler {
	case 'f':
		return "File", m.nav.fileCursor + 1, len(m.files)
	case 'F':
		t, p := 0, 0
		for i, f := range m.files {
			if !m.pr.PendingReview.ViewedFiles[f.Path] {
				t++
				if i == m.nav.fileCursor {
					p = t
				}
			}
		}
		return "Unviewed", p, t
	case 'c':
		if len(m.files) == 0 || m.nav.fileCursor >= len(m.files) {
			return "Comment", 0, 0
		}
		path := m.files[m.nav.fileCursor].Path
		t, p := 0, 0
		for i, dl := range m.nav.diffLines {
			if m.lineHasComments(path, dl) {
				t++
				if i == m.nav.diffCursor {
					p = t
				}
			}
		}
		return "Comment", p, t
	case 'C':
		t, p := 0, 0
		for i, f := range m.files {
			if m.fileHasComments(f.Path) {
				t++
				if i == m.nav.fileCursor {
					p = t
				}
			}
		}
		return "Commented", p, t
	case '/':
		if m.search.query == "" {
			return "Match", 0, 0
		}
		query := strings.ToLower(m.search.query)
		t, p := 0, 0
		for i, dl := range m.nav.diffLines {
			if strings.Contains(strings.ToLower(dl.content), query) {
				t++
				if i == m.nav.diffCursor {
					p = t
				}
			}
		}
		return "Match", p, t
	default:
		// Default: show hunk position
		if len(m.files) == 0 || m.nav.fileCursor >= len(m.files) {
			return "Hunk", 0, 0
		}
		file := m.files[m.nav.fileCursor]
		hunkIdx := 0
		if len(m.nav.diffLines) > 0 && m.nav.diffCursor < len(m.nav.diffLines) {
			hunkIdx = m.nav.diffLines[m.nav.diffCursor].hunkIdx + 1
		}
		return "Hunk", hunkIdx, len(file.Hunks)
	}
}

// overlayHelpPopup renders a centered help popup over the existing content.
func (m Model) overlayHelpPopup(base string) string {
	popup := helppopup.Render(helpSections(), m.width)
	return m.overlayGeneric(base, popup)
}

// helpSections returns the keybinding sections for the diffview help popup.
func helpSections() []helppopup.Section {
	return []helppopup.Section{
		helppopup.Bind("Navigation",
			keys.Down, keys.Up, keys.PageDown, keys.PageUp,
			keys.NextFile, keys.PrevFile, keys.NextHunk, keys.PrevHunk,
			keys.NextComment, keys.PrevComment, keys.JumpBack, keys.JumpForward,
		),
		helppopup.Bind("Search",
			keys.Search, keys.NextSearch, keys.PrevSearch, keys.FilterFile,
		),
		{Title: "Filter", Entries: []helppopup.Entry{
			{Key: "Tf", Desc: "narrow by regex path"},
			{Key: "To", Desc: "toggle CODEOWNERS filter"},
			{Key: "Tx", Desc: "clear all filters"},
		}},
		helppopup.Bind("Review",
			keys.Enter, keys.SelectLine, keys.MarkViewed,
			keys.ToggleComment, keys.FoldComment, keys.Submit,
		),
		helppopup.Bind("Comment Selection",
			keys.Reply, keys.EditComment, keys.DeleteComment,
		),
		helppopup.Bind("Other",
			keys.ToggleTree, keys.Info, keys.OpenInBrowser, keys.CopyLink,
			keys.Editor, keys.Help, keys.Back, keys.Quit,
		),
	}
}
