package review

import (
	"context"

	"github.com/jkuan/pr-review/internal/diff"
)

// UserIdentity holds the authenticated user's login and team memberships.
type UserIdentity struct {
	Login string   // e.g. "@username"
	Teams []string // e.g. ["@org/team1", "@org/team2"]
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

	// FetchExistingComments gets all submitted review comments on a PR.
	FetchExistingComments(ctx context.Context, number int) ([]ExistingComment, error)
	// FetchPendingReview finds the user's existing pending/draft review.
	// Returns the review ID, node ID ("" if none) and any pre-existing comments.
	FetchPendingReview(ctx context.Context, number int) (int, string, []ExistingComment, error)

	// CreatePendingReview creates a PENDING (draft) review on the forge.
	// Returns the review ID and the forge-specific node ID.
	CreatePendingReview(ctx context.Context, prNumber int) (int, string, error)

	// AddReviewComment adds a single comment to an existing pending review.
	// Returns the forge comment ID.
	AddReviewComment(ctx context.Context, reviewNodeID string, comment InlineComment) (int, error)

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

	// ListMentionableUsers returns usernames that can be @mentioned in the repo.
	ListMentionableUsers(ctx context.Context) ([]string, error)

	// UploadImage uploads an image and returns a URL suitable for embedding in markdown.
	UploadImage(ctx context.Context, data []byte, filename string) (string, error)
}
