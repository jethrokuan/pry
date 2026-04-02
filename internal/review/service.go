package review

import (
	"context"

	"github.com/jethrokuan/pry/internal/diff"
)

// UserIdentity holds the authenticated user's login and team memberships.
type UserIdentity struct {
	Login string   // e.g. "@username"
	Teams []string // e.g. ["@org/team1", "@org/team2"]
}

// MentionableUser represents a user that can be @mentioned in the repo.
type MentionableUser struct {
	Login string // GitHub username (e.g. "octocat")
	Name  string // Display name (e.g. "The Octocat"), may be empty
}

// CacheInvalidator is implemented by services that support cache invalidation.
type CacheInvalidator interface {
	// InvalidateListPRs clears all cached PR list and detail results.
	InvalidateListPRs()
}

// Service defines what the application needs from a code review platform.
// Implementations adapt forge-specific APIs (GitHub, GitLab, etc.) to this interface.
type Service interface {
	// RepoOwner returns the repository owner/organization.
	RepoOwner() string
	// RepoName returns the repository name.
	RepoName() string
	// CurrentUser returns the authenticated user's login (e.g. "octocat").
	CurrentUser(ctx context.Context) (string, error)
	// UserTeams returns the team slugs ("org/team") the authenticated user
	// belongs to in the current repo's org. Results may be cached.
	UserTeams(ctx context.Context) ([]string, error)

	// ListPRs fetches pull requests matching the given filter.
	ListPRs(ctx context.Context, filter PRFilter) ([]PullRequest, error)
	// GetPR fetches a single pull request by number with full details.
	GetPR(ctx context.Context, number int) (*PullRequest, error)

	// FetchDiffFiles fetches and parses the changed files for a PR.
	FetchDiffFiles(ctx context.Context, number int) ([]diff.DiffFile, error)

	// FetchCommits fetches individual commits for a PR (lazy-loaded).
	FetchCommits(ctx context.Context, number int) ([]Commit, error)

	// FetchIssueComments fetches top-level conversation comments on a PR.
	FetchIssueComments(ctx context.Context, number int) ([]IssueComment, error)

	// FetchCommentsAndReview fetches all review threads on a PR and the user's
	// pending review in a single query. Comments have IsPending set correctly.
	// Returns (threads, pendingReviewID, pendingReviewNodeID, error).
	// If no pending review exists, reviewID is 0 and nodeID is "".
	FetchCommentsAndReview(ctx context.Context, number int) ([]Thread, int, string, error)

	// CreatePendingReview creates a PENDING (draft) review on the forge.
	// Returns the review ID and the forge-specific node ID.
	CreatePendingReview(ctx context.Context, prNumber int) (int, string, error)

	// AddReviewComment adds a single comment to the user's pending review,
	// creating the review if none exists. reviewNodeID is a cached hint —
	// if non-empty, the service tries it first for speed, falling back to
	// fetch-or-create if the hint is stale. Returns the forge comment ID
	// and the current review IDs (so callers can stay in sync).
	AddReviewComment(ctx context.Context, prNumber int, reviewNodeID string, path string, line, startLine int, side, body string) (int, string, int, string, error)

	// ReplyToReviewComment adds a reply to an existing review thread.
	// commentNodeID is the GraphQL node ID of any comment in the thread to reply to.
	// Returns the forge comment ID, the forge comment node ID, and the current review IDs.
	ReplyToReviewComment(ctx context.Context, prNumber int, reviewNodeID string, commentNodeID, body string) (int, string, int, string, error)

	// DeleteReviewComment deletes a pending review comment by its forge ID.
	DeleteReviewComment(ctx context.Context, prNumber, commentID int) error

	// EditReviewComment updates the body of a review comment by its forge ID.
	EditReviewComment(ctx context.Context, prNumber, commentID int, body string) error

	// SubmitReview submits a review with all accumulated comments.
	SubmitReview(ctx context.Context, pr *PullRequest, review *PendingReview) error

	// FetchViewedFiles returns the set of file paths already marked as viewed on a PR.
	FetchViewedFiles(ctx context.Context, prNodeID string) (map[string]bool, error)

	// MarkFileAsViewed marks a file as viewed on a PR.
	MarkFileAsViewed(ctx context.Context, prNodeID, path string) error
	// UnmarkFileAsViewed unmarks a file as viewed on a PR.
	UnmarkFileAsViewed(ctx context.Context, prNodeID, path string) error

	// ClosePR closes an open pull request.
	ClosePR(ctx context.Context, prNumber int) error
	// ReopenPR reopens a closed pull request.
	ReopenPR(ctx context.Context, prNumber int) error
	// MergePR merges a pull request using the default merge method.
	MergePR(ctx context.Context, prNumber int) error
	// MarkReadyForReview converts a draft PR to ready for review.
	MarkReadyForReview(ctx context.Context, prNodeID string) error
	// AssignPR adds the given user as an assignee on a PR.
	AssignPR(ctx context.Context, prNumber int, login string) error
	// UnassignPR removes the given user as an assignee from a PR.
	UnassignPR(ctx context.Context, prNumber int, login string) error

	// ListMentionableUsers returns users that can be @mentioned in the repo.
	ListMentionableUsers(ctx context.Context) ([]MentionableUser, error)

}
