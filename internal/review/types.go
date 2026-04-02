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

// CheckCounts holds pre-aggregated check counts by outcome category.
// Populated from the API's checkRunCountsByState / statusContextCountsByState
// so we don't need to paginate through individual check nodes.
type CheckCounts struct {
	Failing int // FAILURE, TIMED_OUT, STARTUP_FAILURE, ERROR, ACTION_REQUIRED, CANCELLED
	Passing int // SUCCESS
	Skipped int // SKIPPED, NEUTRAL
	Pending int // IN_PROGRESS, QUEUED, PENDING, WAITING, EXPECTED
	Total   int
}

// Commit represents a single commit on a PR.
type Commit struct {
	SHA            string
	ShortSHA       string
	Message        string // first line only
	Author         string
	CommittedAt    time.Time
	Additions      int
	Deletions      int
	ChecksPass     *bool // nil = pending, true = pass, false = fail
	ChecksTotal    int
}

// Reviewer represents a reviewer and their review status on a PR.
type Reviewer struct {
	Login  string // User login or team slug
	IsTeam bool   // True if this is a team reviewer
	State  string // APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED, PENDING, or ""
}

// PullRequest represents a pull/merge request in a forge-agnostic way.
type PullRequest struct {
	Enriched  bool   // true after GetPR detail data has been merged in
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
	CommitList   []Commit // Individual commits (populated by GetPR)
	CommentCount int
	Body         string
	URL       string
	HeadSHA   string

	// CI status
	ChecksPass    *bool
	ChecksSummary string
	CheckRuns     []CheckRun
	ChecksTotal   int         // Total number of checks (from API totalCount); 0 means unknown
	CheckCounts   CheckCounts // Pre-aggregated counts by outcome (from API countsByState)

	// Assignees
	Assignees []string // User logins assigned to this PR

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
	Threads       []Thread       // review threads populated from forge
}

// MergeEnriched replaces all fields in pr with those from enriched,
// then restores any active review state (PendingReview, Threads)
// that was already set on the receiver.
func (pr *PullRequest) MergeEnriched(enriched *PullRequest) {
	savedReview := pr.PendingReview
	savedThreads := pr.Threads
	*pr = *enriched
	pr.Enriched = true
	if savedReview != nil {
		pr.PendingReview = savedReview
	}
	if savedThreads != nil {
		pr.Threads = savedThreads
	}
}

// ReviewEvent is the type of review action.
type ReviewEvent string

const (
	ReviewEventComment        ReviewEvent = "COMMENT"
	ReviewEventApprove        ReviewEvent = "APPROVE"
	ReviewEventRequestChanges ReviewEvent = "REQUEST_CHANGES"
)

// Thread represents a review thread anchored to a specific location in a PR diff.
// A thread groups one or more comments (the root comment plus any replies).
type Thread struct {
	NodeID     string    // GraphQL node ID (empty for optimistic threads)
	Path       string
	Line       int
	StartLine  int       // for multi-line (0 = single line)
	Side       string    // RIGHT (new) or LEFT (old)
	IsResolved bool
	IsOutdated bool
	Comments   []Comment // ordered chronologically; first is the root
}

// Comment represents a single review comment within a thread.
// Optimistic comments not yet confirmed by the server have negative IDs.
type Comment struct {
	ID        int    // Forge ID (negative = optimistic/temp, not yet confirmed)
	NodeID    string // GraphQL node ID (used for reply-to; empty for optimistic)
	Body      string
	Author    string
	CreatedAt string
	IsPending bool // true if part of a PENDING (draft) review
}

// IssueComment represents a top-level conversation comment on a PR
// (as opposed to an inline review comment anchored to a diff line).
type IssueComment struct {
	ID        int
	Author    string
	Body      string
	CreatedAt string
}

// PendingReview tracks the state of an in-progress review.
// Threads are stored on PullRequest.Threads (the single source of truth);
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
