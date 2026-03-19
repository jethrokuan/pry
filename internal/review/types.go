package review

import (
	"fmt"
	"time"
)

// PRFilter defines a named filter with a GitHub search qualifier.
type PRFilter struct {
	Name      string
	Qualifier string
}

// SyncStatus tracks the sync state of a comment with the forge.
type SyncStatus int

const (
	SyncPending  SyncStatus = iota // Not yet sent to forge
	SyncInFlight                   // Currently being sent
	SyncComplete                   // Successfully synced, has ForgeID set
	SyncFailed                     // Sync failed
)

// PullRequest represents a pull/merge request in a forge-agnostic way.
type PullRequest struct {
	Number    int
	NodeID    string // Forge-specific ID for mutations
	Title     string
	Author    string
	Branch    string
	Base      string
	State     string
	Draft     bool
	Labels    []string
	CreatedAt time.Time
	UpdatedAt time.Time
	Additions int
	Deletions int
	Files     int
	Body      string
	URL       string
	HeadSHA   string

	// CI status
	ChecksPass    *bool
	ChecksSummary string

	// Review status
	ReviewDecision string   // APPROVED, CHANGES_REQUESTED, REVIEW_REQUIRED
	PendingTeams   []string // Team slugs with outstanding review requests
	MyReviewState  string   // Authenticated user's latest review: APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED, or ""
}


// ReviewEvent is the type of review action.
type ReviewEvent string

const (
	ReviewEventComment        ReviewEvent = "COMMENT"
	ReviewEventApprove        ReviewEvent = "APPROVE"
	ReviewEventRequestChanges ReviewEvent = "REQUEST_CHANGES"
)

// InlineComment represents a pending inline comment.
type InlineComment struct {
	Path      string
	Line      int
	StartLine int    // for multi-line (0 = single line)
	Side      string // RIGHT (new) or LEFT (old)
	Body      string

	// Sync tracking
	LocalID    int        // Monotonic local ID for async tracking
	ForgeID   int        // Forge comment ID once synced (0 if not yet)
	SyncStatus SyncStatus // Current sync state
	SyncError  error      // Last sync error, if any
}

// ExistingComment represents an existing review comment on a PR.
type ExistingComment struct {
	ID        int
	Path      string
	Line      int
	Side      string
	Body      string
	Author    string
	CreatedAt string
	IsPending bool // true if part of a PENDING (draft) review
}

// PendingReview accumulates comments before submission.
type PendingReview struct {
	PRNumber     int
	ReviewID     int    // Forge review ID (0 if not yet created)
	ReviewNodeID string // Forge-specific GraphQL ID for the review
	PRNodeID     string // Forge-specific ID for mutations
	CommitID     string // HEAD SHA of the PR
	Comments     []InlineComment
	Body         string
	Event        ReviewEvent
	ViewedFiles  map[string]bool

	// Sync tracking
	nextLocalID int
}

// NewPendingReview creates a new empty pending review.
func NewPendingReview(prNumber int, prNodeID, commitID string) *PendingReview {
	return &PendingReview{
		PRNumber:    prNumber,
		PRNodeID:    prNodeID,
		CommitID:    commitID,
		Comments:    make([]InlineComment, 0),
		Event:       ReviewEventComment,
		ViewedFiles: make(map[string]bool),
	}
}

// nextID returns the next local ID and increments the counter.
func (r *PendingReview) nextID() int {
	r.nextLocalID++
	return r.nextLocalID
}

// AddComment adds an inline comment to the pending review and returns its LocalID.
func (r *PendingReview) AddComment(path string, line int, body string) int {
	id := r.nextID()
	r.Comments = append(r.Comments, InlineComment{
		Path:       path,
		Line:       line,
		Side:       "RIGHT",
		Body:       body,
		LocalID:    id,
		SyncStatus: SyncPending,
	})
	return id
}

// AddMultiLineComment adds a multi-line inline comment and returns its LocalID.
func (r *PendingReview) AddMultiLineComment(path string, startLine, endLine int, body string) int {
	id := r.nextID()
	r.Comments = append(r.Comments, InlineComment{
		Path:       path,
		Line:       endLine,
		StartLine:  startLine,
		Side:       "RIGHT",
		Body:       body,
		LocalID:    id,
		SyncStatus: SyncPending,
	})
	return id
}

// AddSuggestion adds a code suggestion comment and returns its LocalID.
func (r *PendingReview) AddSuggestion(path string, line int, suggestion string) int {
	body := fmt.Sprintf("```suggestion\n%s\n```", suggestion)
	return r.AddComment(path, line, body)
}

// AddMultiLineSuggestion adds a multi-line code suggestion and returns its LocalID.
func (r *PendingReview) AddMultiLineSuggestion(path string, startLine, endLine int, suggestion string) int {
	body := fmt.Sprintf("```suggestion\n%s\n```", suggestion)
	return r.AddMultiLineComment(path, startLine, endLine, body)
}

// AddCommentDirect adds a fully constructed InlineComment and assigns it a LocalID.
func (r *PendingReview) AddCommentDirect(c InlineComment) int {
	id := r.nextID()
	c.LocalID = id
	r.Comments = append(r.Comments, c)
	return id
}

// RemoveComment removes a comment by index.
func (r *PendingReview) RemoveComment(index int) {
	if index >= 0 && index < len(r.Comments) {
		r.Comments = append(r.Comments[:index], r.Comments[index+1:]...)
	}
}

// RemoveCommentByLocalID removes a comment by its local ID. Returns the removed comment's ForgeID (0 if not synced).
func (r *PendingReview) RemoveCommentByLocalID(localID int) int {
	for i, c := range r.Comments {
		if c.LocalID == localID {
			forgeID := c.ForgeID
			r.Comments = append(r.Comments[:i], r.Comments[i+1:]...)
			return forgeID
		}
	}
	return 0
}

// FindByLocalID returns a pointer to the comment with the given LocalID, or nil.
func (r *PendingReview) FindByLocalID(localID int) *InlineComment {
	for i := range r.Comments {
		if r.Comments[i].LocalID == localID {
			return &r.Comments[i]
		}
	}
	return nil
}

// InFlightCount returns the number of comments currently being synced.
func (r *PendingReview) InFlightCount() int {
	count := 0
	for _, c := range r.Comments {
		if c.SyncStatus == SyncInFlight {
			count++
		}
	}
	return count
}

// UnsyncedComments returns all comments that haven't been synced yet.
func (r *PendingReview) UnsyncedComments() []InlineComment {
	var result []InlineComment
	for _, c := range r.Comments {
		if c.SyncStatus == SyncPending || c.SyncStatus == SyncFailed {
			result = append(result, c)
		}
	}
	return result
}

// SetSyncStatus updates the sync status of a comment by LocalID.
func (r *PendingReview) SetSyncStatus(localID int, status SyncStatus, forgeID int, err error) {
	if c := r.FindByLocalID(localID); c != nil {
		c.SyncStatus = status
		if forgeID > 0 {
			c.ForgeID = forgeID
		}
		c.SyncError = err
	}
}
