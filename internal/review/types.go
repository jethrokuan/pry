package review

import (
	"time"
)

// PRFilter defines a named filter with a GitHub search qualifier.
type PRFilter struct {
	Name      string
	Qualifier string
}

// CheckRunStatus represents the execution status of a check run.
type CheckRunStatus string

const (
	CheckRunQueued     CheckRunStatus = "QUEUED"
	CheckRunInProgress CheckRunStatus = "IN_PROGRESS"
	CheckRunCompleted  CheckRunStatus = "COMPLETED"
)

// CheckRunConclusion represents the outcome of a completed check run.
type CheckRunConclusion string

const (
	ConclusionSuccess        CheckRunConclusion = "SUCCESS"
	ConclusionFailure        CheckRunConclusion = "FAILURE"
	ConclusionTimedOut       CheckRunConclusion = "TIMED_OUT"
	ConclusionCancelled      CheckRunConclusion = "CANCELLED"
	ConclusionSkipped        CheckRunConclusion = "SKIPPED"
	ConclusionNeutral        CheckRunConclusion = "NEUTRAL"
	ConclusionActionRequired CheckRunConclusion = "ACTION_REQUIRED"
	ConclusionStartupFailure CheckRunConclusion = "STARTUP_FAILURE"
	ConclusionStale          CheckRunConclusion = "STALE"
)

// CheckRun represents an individual CI check run or status context.
type CheckRun struct {
	Name        string
	Status      CheckRunStatus
	Conclusion  CheckRunConclusion
	StartedAt   time.Time
	CompletedAt time.Time
	DetailsURL  string
}

// Reviewer represents a reviewer and their review status on a PR.
type Reviewer struct {
	Login  string // User login or team slug
	IsTeam bool   // True if this is a team reviewer
	State  string // APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED, PENDING, or ""
}

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
	Files        int
	Commits      int
	CommentCount int
	Body         string
	URL       string
	HeadSHA   string

	// CI status
	ChecksPass    *bool
	ChecksSummary string
	CheckRuns     []CheckRun

	// Merge & review status
	MergeState     string     // BLOCKED, CLEAN, DIRTY, DRAFT, HAS_HOOKS, UNKNOWN, UNSTABLE
	Mergeable      string     // MERGEABLE, CONFLICTING, UNKNOWN
	ConflictFiles  []string   // File paths with merge conflicts (best-effort, may be empty)
	ReviewDecision string     // APPROVED, CHANGES_REQUESTED, REVIEW_REQUIRED
	Reviewers      []Reviewer // Individual reviewer statuses
	PendingTeams   []string   // Team slugs with outstanding review requests
	MyReviewState  string     // Authenticated user's latest review: APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED, or ""

	// Review state (populated when user enters review)
	PendingReview *PendingReview // nil until user starts reviewing
	Comments      []Comment      // populated from forge
}

// MergeEnriched replaces all fields in pr with those from enriched,
// then restores any active review state (PendingReview, Comments)
// that was already set on the receiver.
func (pr *PullRequest) MergeEnriched(enriched *PullRequest) {
	savedReview := pr.PendingReview
	savedComments := pr.Comments
	*pr = *enriched
	if savedReview != nil {
		pr.PendingReview = savedReview
	}
	if savedComments != nil {
		pr.Comments = savedComments
	}
}

// ReviewEvent is the type of review action.
type ReviewEvent string

const (
	ReviewEventComment        ReviewEvent = "COMMENT"
	ReviewEventApprove        ReviewEvent = "APPROVE"
	ReviewEventRequestChanges ReviewEvent = "REQUEST_CHANGES"
)

// Comment represents a review comment on a PR.
// Used for both existing (submitted/draft) comments from the forge and
// optimistic comments not yet confirmed by the server (identified by negative IDs).
type Comment struct {
	ID        int    // Forge ID (negative = optimistic/temp, not yet confirmed)
	Path      string
	Line      int
	StartLine int    // for multi-line (0 = single line)
	Side      string // RIGHT (new) or LEFT (old)
	Body      string
	Author    string
	CreatedAt string
	IsPending bool // true if part of a PENDING (draft) review
}

// PendingReview tracks the state of an in-progress review.
// Comments are stored on PullRequest.Comments (the single source of truth);
// PendingReview only tracks the review envelope (ID, event, body, viewed files).
type PendingReview struct {
	ReviewID     int    // Forge review ID (0 if not yet created)
	ReviewNodeID string // Forge-specific GraphQL ID for the review
	Body         string
	Event        ReviewEvent
	ViewedFiles  map[string]bool

	nextTempID int // Decrementing counter for optimistic temp IDs
}

// StartReview creates a new pending review on this PR and returns it.
func (pr *PullRequest) StartReview() *PendingReview {
	pr.PendingReview = NewPendingReview()
	return pr.PendingReview
}

// NewPendingReview creates a new empty pending review.
func NewPendingReview() *PendingReview {
	return &PendingReview{
		Event:       ReviewEventComment,
		ViewedFiles: make(map[string]bool),
	}
}

// NextTempID returns the next temporary (negative) ID for optimistic comments.
func (r *PendingReview) NextTempID() int {
	r.nextTempID--
	return r.nextTempID
}
