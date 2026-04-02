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

	"github.com/jethrokuan/pry/internal/ai"
	"github.com/jethrokuan/pry/internal/clipboard"
	"github.com/jethrokuan/pry/internal/diff"
	"github.com/jethrokuan/pry/internal/git"
	"github.com/jethrokuan/pry/internal/jj"
	"github.com/jethrokuan/pry/internal/review"
	"github.com/jethrokuan/pry/internal/ui/components/flash"
	"github.com/jethrokuan/pry/internal/ui/components/helppopup"
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

type commentsAndReviewMsg struct {
	threads      []review.Thread
	reviewID     int
	reviewNodeID string
	err          error
}

// commentAddedMsg is sent when a background AddReviewComment call completes.
type commentAddedMsg struct {
	tempID       int            // The optimistic temp ID to replace
	comment      review.Comment // The real comment from the server
	reviewID     int            // Fresh review ID from the server
	reviewNodeID string         // Fresh review node ID from the server
	err          error
}

type commentDeletedMsg struct {
	commentID int
	err       error
}

type commentEditedMsg struct {
	commentID int
	body      string
	oldBody   string // original body for rollback on failure
	err       error
}

type viewedFilesMsg struct {
	viewed map[string]bool
	err    error
}

type refreshDoneMsg struct {
	files        []diff.DiffFile
	threads      []review.Thread
	reviewID     int
	reviewNodeID string
	viewed       map[string]bool
	err          error
}

type checkoutMsg struct {
	branch string
	err    error
}

type markViewedMsg struct {
	path string
	err  error
}

type issueCommentsMsg struct {
	comments []review.IssueComment
	err      error
}

type editorFinishedMsg struct {
	content string
	err     error
}

type commitsLoadedMsg struct {
	commits []review.Commit
	err     error
}

type commitDiffLoadedMsg struct {
	files []diff.DiffFile
	err   error
}


// MentionableUsersMsg carries mentionable users from the app layer.
type MentionableUsersMsg struct {
	Users []review.MentionableUser
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
	NextThread  key.Binding
	PrevThread  key.Binding
	NextComment key.Binding
	PrevComment key.Binding
	// DeleteComment and EditComment are only active in comment-select mode
	DeleteComment key.Binding
	EditComment   key.Binding
	ViewComment   key.Binding
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
	Suggest       key.Binding
	Checkout      key.Binding
	Refresh       key.Binding
	CommitPicker  key.Binding
	AIAsk         key.Binding
}

var keys = KeyMap{
	Up:            key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:          key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	PageUp:        key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("ctrl+u", "page up")),
	PageDown:      key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "page down")),
	ToggleTree:    key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "toggle tree")),
	NextFile:      key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "next file")),
	PrevFile:      key.NewBinding(key.WithKeys("F"), key.WithHelp("F", "prev file")),
	NextHunk:      key.NewBinding(key.WithKeys("h"), key.WithHelp("h", "next hunk")),
	PrevHunk:      key.NewBinding(key.WithKeys("H"), key.WithHelp("H", "prev hunk")),
	NextThread:    key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "next thread")),
	PrevThread:    key.NewBinding(key.WithKeys("T"), key.WithHelp("T", "prev thread")),
	NextComment:   key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "next comment")),
	PrevComment:   key.NewBinding(key.WithKeys("C"), key.WithHelp("C", "prev comment")),
	DeleteComment: key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete comment")),
	EditComment:   key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit comment")),
	ViewComment:   key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "view comment")),
	SelectLine:    key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "select")),
	Submit:        key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "submit review")),
	ToggleComment: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "toggle fold")),
	FoldComment:   key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("S-tab", "toggle all folds")),
	MarkViewed:    key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "mark viewed")),
	OpenInBrowser: key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "open in browser")),
	Editor:        key.NewBinding(key.WithKeys("ctrl+e"), key.WithHelp("ctrl+e", "open in editor")),
	Back:          key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	Quit:          key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
	Enter:         key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "comment/select")),
	Search:        key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
	NextSearch:    key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next match")),
	PrevSearch:    key.NewBinding(key.WithKeys("N"), key.WithHelp("N", "prev match")),
	FilterFile:    key.NewBinding(key.WithKeys("ctrl+p"), key.WithHelp("ctrl+p", "filter files")),
	Help:          key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Info:          key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "PR info")),
	NarrowPrefix:  key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "filter prefix")),
	JumpBack:      key.NewBinding(key.WithKeys("ctrl+o"), key.WithHelp("ctrl+o", "jump back")),
	JumpForward:   key.NewBinding(key.WithKeys("ctrl+i"), key.WithHelp("ctrl+i", "jump forward")),
	CopyLink:      key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy link")),
	Suggest:       key.NewBinding(key.WithKeys("shift+enter"), key.WithHelp("S-enter", "suggest")),
	Checkout:      key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "checkout PR")),
	Refresh:       key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
	CommitPicker:  key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "select commit")),
	AIAsk:         key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "AI assistant")),
}

// Inline comment key bindings (when textarea is active)
var inlineKeys = struct {
	Save       key.Binding
	Cancel     key.Binding
	OpenEditor key.Binding
}{
	Save:       key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "save")),
	Cancel:     key.NewBinding(key.WithKeys("esc", "ctrl+c"), key.WithHelp("esc", "cancel")),
	OpenEditor: key.NewBinding(key.WithKeys("ctrl+e"), key.WithHelp("ctrl+e", "open $EDITOR")),
}

// --- Model ---

type Model struct {
	svc           review.Service
	pr            *review.PullRequest
	pendingReview *review.PendingReview
	files         []diff.DiffFile
	currentUser   string // authenticated user login

	nav      DiffNav
	comments CommentPanel
	search   SearchBar
	filter   FileFilter
	editor   InlineEditor

	// Optimistic comment tracking: maps temp (negative) IDs to in-flight state
	inflight map[int]bool

	width    int
	height   int

	offsets RenderOffsets // populated during renderDiffWithCursor

	loading    bool
	refreshing bool // true while background refresh is in progress
	spinner    spinner.Model
	showHelp   bool

	loadErr error // fatal load error — blocks rendering

	confirmQuit        bool // true when waiting for second quit key to confirm
	confirmDelete      bool // true when waiting for y/n to confirm comment deletion
	narrowPrefixActive bool // true when waiting for second key after 'T'

	useJJ bool // true when repo is managed by Jujutsu

	// PR info popup
	prInfoActive   bool
	prInfoViewport viewport.Model
	issueComments  []review.IssueComment // top-level conversation comments
	prInfoBlocks   []int                 // line offsets of each block (description, comments) for n/N nav

	// Commit-level diff viewing
	commits            []review.Commit // all PR commits (loaded lazily)
	commitsLoaded      bool            // true after first fetch
	commitStart        int             // start index into commits (-1 = no selection)
	commitEnd          int             // end index into commits (-1 = no selection, same as start = single commit)
	commitPickerActive bool            // overlay visible
	commitPickerCursor int             // cursor position in picker
	commitPickerAnchor int             // range anchor index (-1 = no anchor)

	// AI review assistant panel
	aiPanel     AIPanel
	aiEnabled   bool // true when AI is available and configured
	aiCheckedOut bool // true after PR branch has been checked out for AI

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

// WithMentionableUsers seeds the @-mention autocomplete with pre-loaded users.
func WithMentionableUsers(users []review.MentionableUser) Option {
	return func(m *Model) {
		if len(users) > 0 {
			m.editor.SetMentionUsers(users)
		}
	}
}

// WithCurrentUser sets the authenticated user's login for comment ownership.
func WithCurrentUser(login string) Option {
	return func(m *Model) {
		m.currentUser = login
	}
}

// WithJujutsu enables Jujutsu-based checkout instead of gh pr checkout.
func WithJujutsu() Option {
	return func(m *Model) {
		m.useJJ = true
	}
}

// WithOwnerFilterDisabled explicitly disables the owner filter regardless of identity.
func WithOwnerFilterDisabled() Option {
	return func(m *Model) {
		m.filter.ownerEnabled = false
	}
}

// WithAI configures the AI review assistant agent.
func WithAI(agent *ai.Agent) Option {
	return func(m *Model) {
		m.aiPanel.SetAgent(agent)
		m.aiEnabled = true
	}
}

func New(svc review.Service, pr *review.PullRequest, opts ...Option) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	m := Model{
		svc:               svc,
		pr:                pr,
		pendingReview:     pr.PendingReview,
		inflight:          make(map[int]bool),
		loading: true,
		spinner: s,
		mdCache: make(map[mdCacheKey]string),
		treeDirty:         true,
		nav: DiffNav{
			showTree:       true,
			focus:          FocusDiff,
			collapsedDirs:  make(map[string]bool),
			collapsedHunks: make(map[string]bool),
			activeCycler:   CyclerHunk,
		},
		comments: CommentPanel{
			expanded: make(map[string]bool),
		},
		commitStart:        -1,
		commitEnd:          -1,
		commitPickerAnchor: -1,
		aiPanel:            initAIPanel(),
	}

	for _, opt := range opts {
		opt(&m)
	}

	return m
}

// PendingReview returns the current pending review state.
// The app layer reads this when transitioning to the submit screen.
func (m Model) PendingReview() *review.PendingReview {
	return m.pendingReview
}

// --- Init ---

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.loadFiles(),
		m.loadCommentsAndReview(),
		m.loadViewedFiles(),
		m.spinner.Tick,
	)
}


func (m Model) loadViewedFiles() tea.Cmd {
	if m.pr.NodeID == "" {
		return nil
	}
	return func() tea.Msg {
		viewed, err := m.svc.FetchViewedFiles(context.Background(), m.pr.NodeID)
		return viewedFilesMsg{viewed: viewed, err: err}
	}
}

func (m Model) loadFiles() tea.Cmd {
	return func() tea.Msg {
		files, err := m.svc.FetchDiffFiles(context.Background(), m.pr.Number)
		return filesLoadedMsg{files: files, err: err}
	}
}

func (m Model) loadCommentsAndReview() tea.Cmd {
	return func() tea.Msg {
		threads, reviewID, nodeID, err := m.svc.FetchCommentsAndReview(context.Background(), m.pr.Number)
		return commentsAndReviewMsg{threads: threads, reviewID: reviewID, reviewNodeID: nodeID, err: err}
	}
}

// refreshCmd fetches diff files, comments+review, and viewed files,
// returning a single refreshDoneMsg when all complete.
func (m Model) refreshCmd() tea.Cmd {
	svc := m.svc
	prNumber := m.pr.Number
	prNodeID := m.pr.NodeID
	return func() tea.Msg {
		ctx := context.Background()
		var result refreshDoneMsg

		// Fetch files
		files, err := svc.FetchDiffFiles(ctx, prNumber)
		if err != nil {
			result.err = fmt.Errorf("diff: %w", err)
			return result
		}
		result.files = files

		// Fetch threads and pending review atomically
		threads, reviewID, nodeID, err := svc.FetchCommentsAndReview(ctx, prNumber)
		if err != nil {
			result.err = fmt.Errorf("comments: %w", err)
			return result
		}
		result.threads = threads
		result.reviewID = reviewID
		result.reviewNodeID = nodeID

		// Fetch viewed files
		if prNodeID != "" {
			viewed, err := svc.FetchViewedFiles(ctx, prNodeID)
			if err != nil {
				result.err = fmt.Errorf("viewed: %w", err)
				return result
			}
			result.viewed = viewed
		}

		return result
	}
}

// addReviewCommentCmd adds a comment to the pending review. The cached
// reviewNodeID is passed as a fast-path hint; the service falls back to
// fetch-or-create if the hint is empty or stale.
func (m Model) addReviewCommentCmd(tempID int, path string, line, startLine int, side, body string) tea.Cmd {
	svc := m.svc
	currentUser := m.currentUser
	prNumber := m.pr.Number
	cachedNodeID := m.pendingReview.ReviewNodeID

	return func() tea.Msg {
		forgeID, forgeNodeID, reviewID, reviewNodeID, err := svc.AddReviewComment(context.Background(), prNumber, cachedNodeID, path, line, startLine, side, body)
		if err != nil {
			return commentAddedMsg{tempID: tempID, reviewID: reviewID, reviewNodeID: reviewNodeID, err: err}
		}

		return commentAddedMsg{
			tempID:       tempID,
			reviewID:     reviewID,
			reviewNodeID: reviewNodeID,
			comment: review.Comment{
				ID:        forgeID,
				NodeID:    forgeNodeID,
				Body:      body,
				Author:    currentUser,
				IsPending: true,
			},
		}
	}
}

// replyToReviewCommentCmd adds a reply to an existing thread via the service.
func (m Model) replyToReviewCommentCmd(tempID int, commentNodeID, body string) tea.Cmd {
	svc := m.svc
	currentUser := m.currentUser
	prNumber := m.pr.Number
	cachedNodeID := m.pendingReview.ReviewNodeID

	return func() tea.Msg {
		forgeID, nodeID, _, reviewNodeID, err := svc.ReplyToReviewComment(context.Background(), prNumber, cachedNodeID, commentNodeID, body)
		if err != nil {
			return commentAddedMsg{tempID: tempID, reviewNodeID: reviewNodeID, err: err}
		}

		return commentAddedMsg{
			tempID:       tempID,
			reviewNodeID: reviewNodeID,
			comment: review.Comment{
				ID:        forgeID,
				NodeID:    nodeID,
				Body:      body,
				Author:    currentUser,
				IsPending: true,
			},
		}
	}
}

func (m Model) deleteCommentCmd(commentID int) tea.Cmd {
	if commentID <= 0 {
		return nil // temp/optimistic comment, nothing to delete on server
	}
	svc := m.svc
	prNumber := m.pr.Number
	return func() tea.Msg {
		err := svc.DeleteReviewComment(context.Background(), prNumber, commentID)
		return commentDeletedMsg{commentID: commentID, err: err}
	}
}

func (m Model) editCommentCmd(commentID int, body, oldBody string) tea.Cmd {
	if commentID <= 0 {
		return nil
	}
	svc := m.svc
	prNumber := m.pr.Number
	return func() tea.Msg {
		err := svc.EditReviewComment(context.Background(), prNumber, commentID, body)
		return commentEditedMsg{commentID: commentID, body: body, oldBody: oldBody, err: err}
	}
}

func (m Model) fetchIssueCommentsCmd() tea.Cmd {
	svc := m.svc
	prNumber := m.pr.Number
	return func() tea.Msg {
		comments, err := svc.FetchIssueComments(context.Background(), prNumber)
		return issueCommentsMsg{comments: comments, err: err}
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
		if m.editor.IsActive() {
			m.editor.SetWidth(m.inlineTextareaWidth())
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
			m.loadErr = msg.err
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

	case commentsAndReviewMsg:
		if msg.err != nil {
			m.loadErr = fmt.Errorf("comments: %w", msg.err)
		} else {
			m.setThreads(msg.threads)
			if msg.reviewID > 0 {
				m.pendingReview.ReviewID = msg.reviewID
				m.pendingReview.ReviewNodeID = msg.reviewNodeID
			}
			m.updateDiffContent()
		}

	case commentAddedMsg:
		delete(m.inflight, msg.tempID)
		// Keep local review state in sync with the server
		if m.pendingReview != nil && msg.reviewNodeID != "" {
			m.pendingReview.ReviewID = msg.reviewID
			m.pendingReview.ReviewNodeID = msg.reviewNodeID
		}
		if msg.err != nil {
			// Rollback: remove the optimistic comment
			m.removeCommentByID(msg.tempID)
			m.updateDiffContent()
			return m, flash.ShowMsg{ID: "diffview", Text: "Comment failed: " + msg.err.Error(), Style: flash.StyleDanger, Expires: 3 * time.Second}.Cmd()
		}
		// Replace temp comment with real one from server
		m.replaceComment(msg.tempID, msg.comment)
		m.updateDiffContent()

	case commentDeletedMsg:
		if msg.err != nil {
			// Re-fetch comments to restore the deleted one
			return m, tea.Batch(m.loadCommentsAndReview(), flash.ShowMsg{ID: "diffview", Text: "Delete failed: " + msg.err.Error(), Style: flash.StyleDanger, Expires: 3 * time.Second}.Cmd())
		}
		m.updateDiffContent()

	case commentEditedMsg:
		if msg.err != nil {
			// Rollback: restore old body
			m.updateCommentBody(msg.commentID, msg.oldBody)
			m.updateDiffContent()
			return m, flash.ShowMsg{ID: "diffview", Text: "Edit failed: " + msg.err.Error(), Style: flash.StyleDanger, Expires: 3 * time.Second}.Cmd()
		}
		m.updateDiffContent()

	case viewedFilesMsg:
		if msg.err != nil {
			return m, flash.ShowMsg{ID: "viewed-err", Text: fmt.Sprintf("Viewed error: %v", msg.err), Style: flash.StyleDanger, Expires: 3 * time.Second}.Cmd()
		} else if msg.viewed != nil {
			for path := range msg.viewed {
				m.pendingReview.ViewedFiles[path] = true
			}
			m.treeDirty = true
			if len(m.files) > 0 {
				m.nav.treeViewport.SetContent(m.renderFileTree())
				m.treeDirty = false
			}
		}

	case refreshDoneMsg:
		m.refreshing = false
		if msg.err != nil {
			return m, flash.ShowMsg{ID: "refresh", Text: "Refresh failed: " + msg.err.Error(), Style: flash.StyleDanger, Expires: 3 * time.Second}.Cmd()
		}
		// Update diff files
		m.files = msg.files
		m.filter.recompute(m.files)
		m.nav.cachedTree = buildTree(m.files, m.filter.includedFiles)
		m.nav.rebuildTreeRows()
		m.nav.buildDiffLines(m.files)
		m.treeDirty = true
		// Update threads and pending review state atomically
		m.setThreads(msg.threads)
		if msg.reviewID > 0 {
			m.pendingReview.ReviewID = msg.reviewID
			m.pendingReview.ReviewNodeID = msg.reviewNodeID
		}
		// Update viewed files
		if msg.viewed != nil {
			for path := range msg.viewed {
				m.pendingReview.ViewedFiles[path] = true
			}
		}
		m.updateViewports()
		m.updateDiffContent()
		return m, flash.ShowMsg{ID: "refresh", Text: "Refreshed", Style: flash.StyleSuccess, Expires: 2 * time.Second}.Cmd()

	case commitsLoadedMsg:
		if msg.err != nil {
			m.commitPickerActive = false
			return m, flash.ShowMsg{ID: "diffview", Text: "Failed to load commits: " + msg.err.Error(), Style: flash.StyleDanger, Expires: 3 * time.Second}.Cmd()
		}
		m.commits = msg.commits
		m.commitsLoaded = true

	case commitDiffLoadedMsg:
		m.loading = false
		if msg.err != nil {
			// Revert selection on failure
			m.commitStart = -1
			m.commitEnd = -1
			return m, flash.ShowMsg{ID: "diffview", Text: "Failed to load commit diff: " + msg.err.Error(), Style: flash.StyleDanger, Expires: 3 * time.Second}.Cmd()
		}
		m.files = msg.files
		m.filter.recompute(m.files)
		m.nav.cachedTree = buildTree(m.files, m.filter.includedFiles)
		m.nav.rebuildTreeRows()
		m.nav.cursor = CursorTarget{Kind: CursorLine, FileIdx: 0}
		m.nav.buildDiffLines(m.files)
		m.treeDirty = true
		m.updateViewports()
		m.updateDiffContent()
		label := "Full PR diff"
		if m.isCommitView() {
			label = m.commitViewLabel()
		}
		return m, flash.ShowMsg{ID: "diffview", Text: label, Style: flash.StyleSuccess, Expires: 2 * time.Second}.Cmd()

	case checkoutMsg:
		if msg.err != nil {
			return m, flash.ShowMsg{ID: "checkout", Text: fmt.Sprintf("Checkout failed: %v", msg.err), Style: flash.StyleDanger, Expires: 5 * time.Second}.Cmd()
		}
		return m, flash.ShowMsg{ID: "checkout", Text: fmt.Sprintf("Checked out branch %s", msg.branch), Style: flash.StyleSuccess, Expires: 3 * time.Second}.Cmd()

	case issueCommentsMsg:
		if msg.err == nil {
			m.issueComments = msg.comments
			if m.prInfoActive {
				m.openPRInfoPopup()
			}
		}

	case markViewedMsg:
		if msg.err != nil {
			return m, flash.ShowMsg{ID: "viewed-err", Text: fmt.Sprintf("Mark viewed error: %v", msg.err), Style: flash.StyleDanger, Expires: 3 * time.Second}.Cmd()
		}
		// Tree is refreshed optimistically in markTreeItemViewed/markCurrentFileViewed,
		// but also refresh here to pick up any server-side state corrections
		m.treeDirty = true
		if len(m.files) > 0 {
			m.nav.treeViewport.SetContent(m.renderFileTree())
			m.treeDirty = false
		}

	case UserIdentityMsg:
		if msg.Identity != nil {
			m.currentUser = msg.Identity.Login
		}
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
			pendingReview := m.pendingReview
			threads := m.pr.Threads
			*m.pr = *msg.PR
			m.pendingReview = pendingReview
			m.pr.Threads = threads
			if m.prInfoActive {
				m.openPRInfoPopup()
			}
			// Viewed files may not have been loaded yet if PRNodeID was
			// empty at Init time (e.g. CLI launch with just a PR number).
			// Now that app.go has backfilled the node ID, try again.
			if len(m.pendingReview.ViewedFiles) == 0 && m.pr.NodeID != "" {
				return m, m.loadViewedFiles()
			}
		}

	case MentionableUsersMsg:
		if len(msg.Users) > 0 {
			m.editor.SetMentionUsers(msg.Users)
		}

	case copyResultMsg:
		if msg.err != nil {
			return m, flash.ShowMsg{ID: "diffview", Text: "Copy failed: " + msg.err.Error(), Style: flash.StyleDanger, Expires: 3 * time.Second}.Cmd()
		}
		return m, flash.ShowMsg{ID: "diffview", Text: fmt.Sprintf("Copied %s", msg.url), Expires: 1500 * time.Millisecond}.Cmd()

	case editorFinishedMsg:
		if msg.err == nil && msg.content != "" {
			m.editor.SetValue(msg.content)
		}

	// AI panel streaming messages
	case aiFirstChunkMsg, aiNextChunkMsg, aiStreamDoneMsg, aiStreamErrorMsg:
		cmd := m.aiPanel.HandleStreamMsg(msg)
		return m, cmd

	case aiDraftResultMsg:
		m.aiPanel.HandleDraftResult(msg)
		if msg.err == nil && msg.result != nil {
			return m, m.applyDraftResult(msg.result)
		}

	case aiActionMsg:
		// Emit inlineEditorSaveMsg for each action — reuses the existing comment flow
		var cmds []tea.Cmd
		var posted, edited int
		for _, action := range msg.actions {
			side := action.Side
			if side == "" {
				side = "RIGHT"
			}

			if action.Action == "edit" {
				// Find the user's pending comment at this location to get its ID
				var editID int
				for _, c := range m.comments.CommentsForLine(action.Path, action.Line, side) {
					if c.IsPending && c.Author == m.currentUser {
						editID = c.ID
						break
					}
				}
				if editID == 0 {
					continue // no editable comment at this location
				}
				saveMsg := inlineEditorSaveMsg{
					body:          action.Body,
					path:          action.Path,
					line:          action.Line,
					side:          side,
					editCommentID: editID,
				}
				newM, cmd := m.handleEditorSave(saveMsg)
				m = newM
				cmds = append(cmds, cmd)
				edited++
				continue
			}

			saveMsg := inlineEditorSaveMsg{
				body: action.Body,
				path: action.Path,
				line: action.Line,
				side: side,
				mode: commentModeComment,
			}
			newM, cmd := m.handleEditorSave(saveMsg)
			m = newM
			cmds = append(cmds, cmd)
			posted++
		}
		if posted+edited > 0 {
			var parts []string
			if posted > 0 {
				parts = append(parts, fmt.Sprintf("posted %d comment(s)", posted))
			}
			if edited > 0 {
				parts = append(parts, fmt.Sprintf("edited %d comment(s)", edited))
			}
			cmds = append(cmds, flash.ShowMsg{
				ID:      "ai-action",
				Text:    "AI " + strings.Join(parts, ", "),
				Style:   flash.StyleSuccess,
				Expires: 2 * time.Second,
			}.Cmd())
		}
		return m, tea.Batch(cmds...)

	case aiCheckoutDoneMsg:
		if msg.err != nil {
			m.aiCheckedOut = false // allow retry
			m.aiPanel.errorText = fmt.Sprintf("Checkout failed: %v", msg.err)
			m.aiPanel.state = aiPanelActive
			m.aiPanel.input.Focus()
			m.aiPanel.rebuildViewportContent()
			return m, flash.ShowMsg{ID: "ai-checkout", Text: "Checkout failed: " + msg.err.Error(), Style: flash.StyleDanger, Expires: 3 * time.Second}.Cmd()
		}
		// Checkout succeeded — now submit the queued query
		cmd := m.aiPanel.Submit(msg.task)
		return m, tea.Batch(
			flash.ShowMsg{ID: "ai-checkout", Text: "Checked out " + m.pr.Branch, Style: flash.StyleSuccess, Expires: 2 * time.Second}.Cmd(),
			cmd,
		)

	case spinner.TickMsg:
		var cmds []tea.Cmd
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
		if m.aiPanel.IsOpen() || m.aiPanel.IsWorking() {
			aiCmd := m.aiPanel.HandleStreamMsg(msg)
			cmds = append(cmds, aiCmd)
		}
		return m, tea.Batch(cmds...)

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
		if m.editor.IsActive() {
			return m.handleInlineEditorKey(msg)
		}
		return m.handleKey(msg)
	}

	// Forward non-key messages to AI panel textarea when input is active
	if m.aiPanel.IsInputActive() {
		var cmd tea.Cmd
		m.aiPanel.input, cmd = m.aiPanel.input.Update(msg)
		return m, cmd
	}

	// Forward non-key messages to textarea when active
	if m.editor.IsActive() {
		var cmd tea.Cmd
		m.editor.ta, cmd = m.editor.ta.Update(msg)
		m.updateDiffContent()
		return m, cmd
	}

	return m, nil
}

// --- Viewport helpers ---

// RenderOffsets records the rendered Y positions of elements during renderDiffWithCursor.
// This eliminates the need for analytical position calculations that drift out of sync
// with actual rendering.
type RenderOffsets struct {
	// DiffLineY maps diffLines index -> rendered Y line (0-based) in viewport content.
	DiffLineY []int
	// CommentBlockHeight maps diffLines index -> total rendered height of
	// comment threads + editor below that diff line.
	CommentBlockHeight []int
	// SelectedCommentY is the absolute Y of the selected comment box (0 if none).
	SelectedCommentY int
	// SelectedCommentHeight is the height of the selected comment box (0 if none).
	SelectedCommentHeight int
}

// syncViewport renders the diff content to populate offsets, then scrolls the
// viewport to ensure the current target is visible. The target is determined
// by cursor state: if a comment is selected, scroll to that comment box;
// otherwise scroll to the cursor's diff line.
func (m *Model) syncViewport() {
	m.updateDiffContent()

	var y, height int
	if m.nav.cursor.IsComment() && m.offsets.SelectedCommentHeight > 0 {
		y = m.offsets.SelectedCommentY
		height = m.offsets.SelectedCommentHeight
	} else if m.nav.cursor.LineIdx < len(m.offsets.DiffLineY) {
		y = m.offsets.DiffLineY[m.nav.cursor.LineIdx]
		height = 1 + m.offsets.CommentBlockHeight[m.nav.cursor.LineIdx]
	} else {
		return
	}

	vpHeight := m.nav.diffViewport.Height()
	vpTop := m.nav.diffViewport.YOffset()
	elemBottom := y + height

	// Already fully visible in the top half — no scroll needed
	if y >= vpTop && elemBottom <= vpTop+vpHeight/2 {
		return
	}

	// Already fully visible anywhere — no scroll needed for single lines
	if height <= 1 && y >= vpTop && elemBottom <= vpTop+vpHeight {
		return
	}

	// Position element near top of viewport
	margin := vpHeight / 6
	if margin < 1 {
		margin = 1
	}
	offset := y - margin
	if offset < 0 {
		offset = 0
	}
	m.nav.diffViewport.SetYOffset(offset)
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



// hasUnsavedWork returns true if there are any pending (draft) comments
// from the current user that haven't been submitted yet.
func (m *Model) hasUnsavedWork() bool {
	for _, t := range m.comments.threads {
		for _, c := range t.Comments {
			if c.IsPending && c.Author == m.currentUser {
				return true
			}
		}
	}
	return false
}

// pendingCommentCount returns the number of pending (draft) comments from the current user.
func (m *Model) pendingCommentCount() int {
	n := 0
	for _, t := range m.comments.threads {
		for _, c := range t.Comments {
			if c.IsPending && c.Author == m.currentUser {
				n++
			}
		}
	}
	return n
}

const editorHeaderLines = 2 // title bar + file name bar before viewport

// treeWidth returns the width of the file tree panel (excluding separator), or 0 if hidden.
func (m Model) treeWidth() int {
	if !m.nav.showTree {
		return 0
	}
	return min(50, m.width/3)
}

// treePanelWidth returns the total width consumed by the tree panel including the separator, or 0 if hidden.
func (m Model) treePanelWidth() int {
	w := m.treeWidth()
	if w > 0 {
		return w + 1
	}
	return 0
}

func (m *Model) updateViewports() {
	treeWidth := m.treeWidth()
	diffWidth := m.width - treeWidth - 1

	headerHeight := 3
	footerHeight := 2
	contentHeight := m.height - headerHeight - footerHeight
	if contentHeight < 1 {
		contentHeight = 1
	}

	m.nav.treeViewport = viewport.New(viewport.WithWidth(treeWidth), viewport.WithHeight(contentHeight))
	m.nav.diffViewport = viewport.New(viewport.WithWidth(diffWidth), viewport.WithHeight(contentHeight))

	if m.aiPanel.IsOpen() {
		// Floating panel takes most of the screen
		panelW := m.width - 6
		if panelW > 120 {
			panelW = 120
		}
		panelH := m.height - 6
		if panelH < 10 {
			panelH = 10
		}
		m.aiPanel.Resize(panelW, panelH)
	}

	if len(m.files) > 0 {
		m.nav.treeViewport.SetContent(m.renderFileTree())
		m.treeDirty = false
		m.updateDiffContent()
	}
}

// copyForgeLink builds a GitHub permalink for the current file+line (or selection)
// and copies it to the system clipboard.
func (m *Model) copyForgeLink() tea.Cmd {
	if len(m.files) == 0 || m.nav.cursor.FileIdx >= len(m.files) {
		return flash.ShowMsg{ID: "diffview", Text: "No file to copy link for", Expires: 1500 * time.Millisecond}.Cmd()
	}

	file := m.files[m.nav.cursor.FileIdx]
	sha := m.pr.HeadSHA
	owner := m.svc.RepoOwner()
	repo := m.svc.RepoName()

	// Build base blob URL
	url := fmt.Sprintf("https://github.com/%s/%s/blob/%s/%s", owner, repo, sha, file.Path)

	// Add line anchor
	if m.nav.cursor.LineIdx < len(m.nav.diffLines) {
		if m.nav.visualMode {
			// Selection active — compute line range
			startIdx := min(m.nav.visualStart.LineIdx, m.nav.visualEnd.LineIdx)
			endIdx := max(m.nav.visualStart.LineIdx, m.nav.visualEnd.LineIdx)
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
			line := m.resolveLineNumber(m.nav.cursor.LineIdx)
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

// checkoutPR checks out the PR branch locally, using jj or gh depending on
// whether the repo is Jujutsu-managed.
func (m Model) checkoutPR() tea.Cmd {
	prNumber := m.pr.Number
	branch := m.pr.Branch
	useJJ := m.useJJ
	return func() tea.Msg {
		var err error
		if useJJ {
			err = jj.Checkout(branch)
		} else {
			err = git.CheckoutPR(prNumber)
		}
		return checkoutMsg{branch: branch, err: err}
	}
}

func (m *Model) updateDiffContent() {
	if len(m.files) == 0 || m.nav.cursor.FileIdx >= len(m.files) {
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
	file := m.files[m.nav.cursor.FileIdx]
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

	pendingCount := ""
	if n := m.pendingCommentCount(); n > 0 {
		pendingCount = lipgloss.NewStyle().Foreground(styles.Warning).
			Render(fmt.Sprintf("  [%d pending]", n))
	}

	viewedCount := len(m.pendingReview.ViewedFiles)
	viewedLabel := ""
	if viewedCount > 0 {
		viewedLabel = lipgloss.NewStyle().Foreground(styles.Success).
			Render(fmt.Sprintf("  [%d/%d viewed]", viewedCount, len(m.files)))
	}

	syncLabel := ""
	if n := len(m.inflight); n > 0 {
		syncLabel = lipgloss.NewStyle().Foreground(styles.Secondary).
			Render(fmt.Sprintf("  [syncing %d...]", n))
	}

	commitLabel := ""
	if m.isCommitView() {
		commitLabel = lipgloss.NewStyle().Foreground(styles.Info).
			Render(fmt.Sprintf("  [%s]", m.commitViewLabel()))
	}

	b.WriteString(header + pendingCount + viewedLabel + syncLabel + commitLabel + "\n")

	if m.loading {
		b.WriteString(m.spinner.View() + " Loading diff...\n")
		return b.String()
	}

	if m.loadErr != nil {
		b.WriteString(lipgloss.NewStyle().Foreground(styles.Danger).
			Render(fmt.Sprintf("Error: %v", m.loadErr)) + "\n")
		return b.String()
	}

	// File name bar
	if len(m.files) > 0 {
		fileName := m.files[m.nav.cursor.FileIdx].Path
		viewed := ""
		if m.pendingReview.ViewedFiles[fileName] {
			viewed = " ✓"
		}
		b.WriteString(styles.StatusBar.Render(
			fmt.Sprintf(" %s (%d/%d)%s ", fileName, m.nav.cursor.FileIdx+1, len(m.files), viewed)) + "\n")
	}

	// Main content
	if m.nav.showTree {
		separator := lipgloss.NewStyle().Foreground(styles.Muted).Render("│")
		split := lipgloss.JoinHorizontal(lipgloss.Top, m.nav.treeViewport.View(), separator, m.nav.diffViewport.View())
		b.WriteString(split + "\n")
	} else {
		b.WriteString(m.nav.diffViewport.View() + "\n")
	}

	// Input mode bars
	if m.search.IsActive() {
		b.WriteString(m.search.View())
	} else if m.filter.regexActive {
		prompt := lipgloss.NewStyle().Bold(true).Render("Narrow (regex): ")
		b.WriteString(prompt + m.filter.regexInput + "█")
	} else if m.nav.cursor.IsComment() && m.nav.focus == FocusDiff {
		// Comment selection mode footer
		refs := m.commentRefsAtCursor()
		var helpParts []string
		helpParts = append(helpParts, "j/k select comment")
		helpParts = append(helpParts, "enter reply")
		helpParts = append(helpParts, keys.ViewComment.Help().Key+" expand")
		if flatIdx := m.flatCommentIndex(); flatIdx < len(refs) && refs[flatIdx].editable {
			helpParts = append(helpParts, "e edit")
			helpParts = append(helpParts, "d delete")
		}
		helpParts = append(helpParts, "esc back")
		if m.confirmDelete {
			b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(styles.Warning).
				Render("Delete comment? (y/n)"))
		} else {
			b.WriteString(styles.HelpStyle.Render(strings.Join(helpParts, "  ")))
		}
	} else if m.confirmQuit {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(styles.Warning).
			Render(fmt.Sprintf("You have %d pending comment(s). Press ctrl+c again to quit.", m.pendingCommentCount())))
	} else if !m.editor.IsActive() {
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
			helpParts = append(helpParts, "x filter")
			helpParts = append(helpParts, "m viewed")
			helpParts = append(helpParts, "b tree")
			helpParts = append(helpParts, "i info")
			if m.aiEnabled {
				helpParts = append(helpParts, "a AI")
			}
			helpParts = append(helpParts, "ctrl+s submit")
			helpParts = append(helpParts, "? help")
		} else {
			helpParts = append(helpParts, "j/k scroll")
			helpParts = append(helpParts, "f/F file")
			helpParts = append(helpParts, "h/H hunk")
			helpParts = append(helpParts, "t/T thread")
			helpParts = append(helpParts, "c/C comment")
			helpParts = append(helpParts, "/ search")
			helpParts = append(helpParts, "x filter")
			helpParts = append(helpParts, "l commits")
			helpParts = append(helpParts, "enter comment")
			helpParts = append(helpParts, "b tree")
			helpParts = append(helpParts, "m viewed")
			if m.nav.visualMode {
				helpParts = append(helpParts, "(SELECT)")
			}
			if m.search.Query() != "" {
				helpParts = append(helpParts, fmt.Sprintf("[/%s n/N]", m.search.Query()))
			}
			helpParts = append(helpParts, "i info")
			if m.aiEnabled {
				helpParts = append(helpParts, "a AI")
			}
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


		// Show AI background indicator when working but hidden
		if m.aiEnabled && m.aiPanel.IsWorking() && !m.aiPanel.IsOpen() {
			status := m.aiPanel.StatusText()
			if status == "" {
				status = "working..."
			}
			aiLabel := fmt.Sprintf("[AI: %s]", status)
			helpParts = append(helpParts,
				lipgloss.NewStyle().Foreground(styles.Cyan).Render(aiLabel))
		}

		b.WriteString(styles.HelpStyle.Render(strings.Join(helpParts, "  ")))
	}

	result := b.String()

	// Overlay autocomplete dropdown for inline editor
	if m.editor.IsActive() {
		result = m.overlayAutocompleteDropdown(result)
	}

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
	if m.commitPickerActive {
		result = m.overlayGeneric(result, m.renderCommitPicker())
	}
	if m.aiPanel.IsOpen() {
		dropdown := m.renderAIContextDropdown()
		result = m.overlayAIPanel(result, dropdown)
	}

	return result
}

// currentPosition dynamically computes the position counter for the active navigation type.
func (m Model) currentPosition() (label string, index int, total int) {
	switch m.nav.activeCycler {
	case CyclerFile:
		return "File", m.nav.cursor.FileIdx + 1, len(m.files)
	case CyclerThread:
		if len(m.files) == 0 || m.nav.cursor.FileIdx >= len(m.files) {
			return "Thread", 0, 0
		}
		path := m.files[m.nav.cursor.FileIdx].Path
		t, p := 0, 0
		for i, dl := range m.nav.diffLines {
			if m.comments.LineHasComments(path, dl) {
				t++
				if i == m.nav.cursor.LineIdx {
					p = t
				}
			}
		}
		return "Thread", p, t
	case CyclerComment:
		positions := m.buildCommentPositions()
		total := len(positions)
		curLine := 0
		if m.nav.cursor.LineIdx < len(m.nav.diffLines) {
			dl := m.nav.diffLines[m.nav.cursor.LineIdx]
			if dl.newLine > 0 {
				curLine = dl.newLine
			} else {
				curLine = dl.oldLine
			}
		}
		cur := 0
		for i, cp := range positions {
			if cp.fileIdx == m.nav.cursor.FileIdx && cp.line == curLine && cp.threadIdx == m.nav.cursor.ThreadIdx && cp.commentIdx == m.nav.cursor.CommentIdx {
				cur = i + 1
				break
			}
		}
		return "Comment", cur, total
	case CyclerSearch:
		if m.search.Query() == "" {
			return "Match", 0, 0
		}
		query := strings.ToLower(m.search.Query())
		t, p := 0, 0
		for i, dl := range m.nav.diffLines {
			if strings.Contains(strings.ToLower(dl.content), query) {
				t++
				if i == m.nav.cursor.LineIdx {
					p = t
				}
			}
		}
		return "Match", p, t
	default:
		// Default: show hunk position
		if len(m.files) == 0 || m.nav.cursor.FileIdx >= len(m.files) {
			return "Hunk", 0, 0
		}
		file := m.files[m.nav.cursor.FileIdx]
		hunkIdx := 0
		if len(m.nav.diffLines) > 0 && m.nav.cursor.LineIdx < len(m.nav.diffLines) {
			hunkIdx = m.nav.diffLines[m.nav.cursor.LineIdx].hunkIdx + 1
		}
		return "Hunk", hunkIdx, len(file.Hunks)
	}
}

// overlayHelpPopup renders a centered help popup over the existing content.
func (m Model) overlayHelpPopup(base string) string {
	popup := helppopup.Render(m.helpSections(), m.width)
	return m.overlayGeneric(base, popup)
}

// helpSections returns the keybinding sections for the diffview help popup.
func (m Model) helpSections() []helppopup.Section {
	sections := []helppopup.Section{
		helppopup.Bind("Navigation",
			keys.Down, keys.Up, keys.PageDown, keys.PageUp,
			keys.NextFile, keys.PrevFile, keys.NextHunk, keys.PrevHunk,
			keys.NextThread, keys.PrevThread, keys.NextComment, keys.PrevComment,
			keys.JumpBack, keys.JumpForward,
		),
		helppopup.Bind("Search",
			keys.Search, keys.NextSearch, keys.PrevSearch, keys.FilterFile,
		),
		{Title: "Filter", Entries: []helppopup.Entry{
			{Key: "xf", Desc: "narrow by regex path"},
			{Key: "xo", Desc: "toggle CODEOWNERS filter"},
			{Key: "xx", Desc: "clear all filters"},
		}},
		helppopup.Bind("Review",
			keys.Enter, keys.SelectLine, keys.MarkViewed,
			keys.ToggleComment, keys.FoldComment, keys.Submit,
		),
		{Title: "Comment Selection", Entries: []helppopup.Entry{
			{Key: "enter", Desc: "reply to thread"},
			helppopup.FromBinding(keys.ViewComment),
			helppopup.FromBinding(keys.EditComment),
			helppopup.FromBinding(keys.DeleteComment),
		}},
	}
	if m.aiEnabled {
		sections = append(sections, helppopup.Bind("AI Assistant",
			keys.AIAsk,
		))
	}
	sections = append(sections, helppopup.Bind("Other",
		keys.ToggleTree, keys.Info, keys.CommitPicker, keys.Refresh, keys.OpenInBrowser, keys.CopyLink,
		keys.Checkout, keys.Editor, keys.Help, keys.Back, keys.Quit,
	))
	return sections
}
