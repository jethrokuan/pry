// Package reviewtest provides a configurable mock implementation of review.Service
// for use in UI tests. Each method is backed by a function field that can be set
// per-test. Unset methods return zero values.
package reviewtest

import (
	"context"

	"github.com/jethrokuan/pry/internal/diff"
	"github.com/jethrokuan/pry/internal/review"
)

// MockService implements review.Service with configurable function fields.
// Set the corresponding Fn field to control the return value for each method.
// Unset methods return zero values and nil errors.
type MockService struct {
	Owner string
	Repo  string

	CurrentUserFn          func(ctx context.Context) (string, error)
	UserTeamsFn            func(ctx context.Context) ([]string, error)
	ListPRsFn              func(ctx context.Context, filter review.PRFilter) ([]review.PullRequest, error)
	GetPRFn                func(ctx context.Context, number int) (*review.PullRequest, error)
	FetchDiffFilesFn       func(ctx context.Context, number int) ([]diff.DiffFile, error)
	FetchIssueCommentsFn     func(ctx context.Context, number int) ([]review.IssueComment, error)
	FetchCommentsAndReviewFn func(ctx context.Context, number int) ([]review.Thread, int, string, error)
	CreatePendingReviewFn func(ctx context.Context, prNumber int) (int, string, error)
	AddReviewCommentFn      func(ctx context.Context, prNumber int, reviewNodeID string, path string, line, startLine int, side, body string) (int, string, int, string, error)
	ReplyToReviewCommentFn  func(ctx context.Context, prNumber int, reviewNodeID string, commentNodeID, body string) (int, string, int, string, error)
	DeleteReviewCommentFn   func(ctx context.Context, prNumber, commentID int) error
	EditReviewCommentFn    func(ctx context.Context, prNumber, commentID int, body string) error
	SubmitReviewFn         func(ctx context.Context, pr *review.PullRequest, r *review.PendingReview) error
	FetchViewedFilesFn     func(ctx context.Context, prNodeID string) (map[string]bool, error)
	MarkFileAsViewedFn     func(ctx context.Context, prNodeID, path string) error
	UnmarkFileAsViewedFn   func(ctx context.Context, prNodeID, path string) error
	ListMentionableUsersFn    func(ctx context.Context) ([]review.MentionableUser, error)
	ClosePRFn                 func(ctx context.Context, prNumber int) error
	ReopenPRFn                func(ctx context.Context, prNumber int) error
	MergePRFn                 func(ctx context.Context, prNumber int) error
	MarkReadyForReviewFn      func(ctx context.Context, prNodeID string) error
	AssignPRFn                func(ctx context.Context, prNumber int, login string) error
	UnassignPRFn              func(ctx context.Context, prNumber int, login string) error
}

func (m *MockService) RepoOwner() string { return m.Owner }
func (m *MockService) RepoName() string  { return m.Repo }

func (m *MockService) CurrentUser(ctx context.Context) (string, error) {
	if m.CurrentUserFn != nil {
		return m.CurrentUserFn(ctx)
	}
	return "", nil
}

func (m *MockService) UserTeams(ctx context.Context) ([]string, error) {
	if m.UserTeamsFn != nil {
		return m.UserTeamsFn(ctx)
	}
	return nil, nil
}

func (m *MockService) ListPRs(ctx context.Context, filter review.PRFilter) ([]review.PullRequest, error) {
	if m.ListPRsFn != nil {
		return m.ListPRsFn(ctx, filter)
	}
	return nil, nil
}

func (m *MockService) GetPR(ctx context.Context, number int) (*review.PullRequest, error) {
	if m.GetPRFn != nil {
		return m.GetPRFn(ctx, number)
	}
	return nil, nil
}

func (m *MockService) FetchDiffFiles(ctx context.Context, number int) ([]diff.DiffFile, error) {
	if m.FetchDiffFilesFn != nil {
		return m.FetchDiffFilesFn(ctx, number)
	}
	return nil, nil
}

func (m *MockService) FetchIssueComments(ctx context.Context, number int) ([]review.IssueComment, error) {
	if m.FetchIssueCommentsFn != nil {
		return m.FetchIssueCommentsFn(ctx, number)
	}
	return nil, nil
}

func (m *MockService) FetchCommits(_ context.Context, _ int) ([]review.Commit, error) {
	return nil, nil
}

func (m *MockService) FetchCommitDiff(_ context.Context, _, _ string) ([]diff.DiffFile, error) {
	return nil, nil
}

func (m *MockService) FetchCommentsAndReview(ctx context.Context, number int) ([]review.Thread, int, string, error) {
	if m.FetchCommentsAndReviewFn != nil {
		return m.FetchCommentsAndReviewFn(ctx, number)
	}
	return nil, 0, "", nil
}

func (m *MockService) CreatePendingReview(ctx context.Context, prNumber int) (int, string, error) {
	if m.CreatePendingReviewFn != nil {
		return m.CreatePendingReviewFn(ctx, prNumber)
	}
	return 0, "", nil
}

func (m *MockService) AddReviewComment(ctx context.Context, prNumber int, reviewNodeID string, path string, line, startLine int, side, body string) (int, string, int, string, error) {
	if m.AddReviewCommentFn != nil {
		return m.AddReviewCommentFn(ctx, prNumber, reviewNodeID, path, line, startLine, side, body)
	}
	return 0, "", 0, "", nil
}

func (m *MockService) ReplyToReviewComment(ctx context.Context, prNumber int, reviewNodeID string, commentNodeID, body string) (int, string, int, string, error) {
	if m.ReplyToReviewCommentFn != nil {
		return m.ReplyToReviewCommentFn(ctx, prNumber, reviewNodeID, commentNodeID, body)
	}
	return 0, "", 0, "", nil
}

func (m *MockService) DeleteReviewComment(ctx context.Context, prNumber, commentID int) error {
	if m.DeleteReviewCommentFn != nil {
		return m.DeleteReviewCommentFn(ctx, prNumber, commentID)
	}
	return nil
}

func (m *MockService) EditReviewComment(ctx context.Context, prNumber, commentID int, body string) error {
	if m.EditReviewCommentFn != nil {
		return m.EditReviewCommentFn(ctx, prNumber, commentID, body)
	}
	return nil
}

func (m *MockService) SubmitReview(ctx context.Context, pr *review.PullRequest, r *review.PendingReview) error {
	if m.SubmitReviewFn != nil {
		return m.SubmitReviewFn(ctx, pr, r)
	}
	return nil
}

func (m *MockService) FetchViewedFiles(ctx context.Context, prNodeID string) (map[string]bool, error) {
	if m.FetchViewedFilesFn != nil {
		return m.FetchViewedFilesFn(ctx, prNodeID)
	}
	return nil, nil
}

func (m *MockService) MarkFileAsViewed(ctx context.Context, prNodeID, path string) error {
	if m.MarkFileAsViewedFn != nil {
		return m.MarkFileAsViewedFn(ctx, prNodeID, path)
	}
	return nil
}

func (m *MockService) UnmarkFileAsViewed(ctx context.Context, prNodeID, path string) error {
	if m.UnmarkFileAsViewedFn != nil {
		return m.UnmarkFileAsViewedFn(ctx, prNodeID, path)
	}
	return nil
}

func (m *MockService) ListMentionableUsers(ctx context.Context) ([]review.MentionableUser, error) {
	if m.ListMentionableUsersFn != nil {
		return m.ListMentionableUsersFn(ctx)
	}
	return nil, nil
}

func (m *MockService) ClosePR(ctx context.Context, prNumber int) error {
	if m.ClosePRFn != nil {
		return m.ClosePRFn(ctx, prNumber)
	}
	return nil
}

func (m *MockService) ReopenPR(ctx context.Context, prNumber int) error {
	if m.ReopenPRFn != nil {
		return m.ReopenPRFn(ctx, prNumber)
	}
	return nil
}

func (m *MockService) MergePR(ctx context.Context, prNumber int) error {
	if m.MergePRFn != nil {
		return m.MergePRFn(ctx, prNumber)
	}
	return nil
}

func (m *MockService) MarkReadyForReview(ctx context.Context, prNodeID string) error {
	if m.MarkReadyForReviewFn != nil {
		return m.MarkReadyForReviewFn(ctx, prNodeID)
	}
	return nil
}

func (m *MockService) AssignPR(ctx context.Context, prNumber int, login string) error {
	if m.AssignPRFn != nil {
		return m.AssignPRFn(ctx, prNumber, login)
	}
	return nil
}

func (m *MockService) UnassignPR(ctx context.Context, prNumber int, login string) error {
	if m.UnassignPRFn != nil {
		return m.UnassignPRFn(ctx, prNumber, login)
	}
	return nil
}
