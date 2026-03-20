package testutil

import (
	"context"

	"github.com/jkuan/pr-review/internal/diff"
	"github.com/jkuan/pr-review/internal/review"
)

// MockService is a configurable mock implementation of review.Service.
// Each method delegates to a corresponding function field. If the field is nil,
// the method returns a sensible zero value.
type MockService struct {
	RepoOwnerFn             func() string
	RepoNameFn              func() string
	CurrentUserFn           func(ctx context.Context) (string, error)
	UserTeamsFn             func(ctx context.Context) ([]string, error)
	ListPRsFn               func(ctx context.Context, filter review.PRFilter) ([]review.PullRequest, error)
	GetPRFn                 func(ctx context.Context, number int) (*review.PullRequest, error)
	FetchDiffFilesFn        func(ctx context.Context, number int) ([]diff.DiffFile, error)
	FetchExistingCommentsFn func(ctx context.Context, number int) ([]review.ExistingComment, error)
	FetchPendingReviewFn    func(ctx context.Context, number int) (int, string, []review.ExistingComment, error)
	CreatePendingReviewFn   func(ctx context.Context, prNumber int) (int, string, error)
	AddReviewCommentFn      func(ctx context.Context, reviewNodeID string, comment review.InlineComment) (int, error)
	DeleteReviewCommentFn   func(ctx context.Context, prNumber, commentID int) error
	EditReviewCommentFn     func(ctx context.Context, prNumber, commentID int, body string) error
	SubmitReviewFn          func(ctx context.Context, review *review.PendingReview) error
	FetchViewedFilesFn      func(ctx context.Context, prNodeID string) (map[string]bool, error)
	MarkFileAsViewedFn      func(ctx context.Context, prNodeID, path string) error
	UnmarkFileAsViewedFn    func(ctx context.Context, prNodeID, path string) error
}

// Compile-time check that MockService implements review.Service.
var _ review.Service = (*MockService)(nil)

func (m *MockService) RepoOwner() string {
	if m.RepoOwnerFn != nil {
		return m.RepoOwnerFn()
	}
	return "test-owner"
}

func (m *MockService) RepoName() string {
	if m.RepoNameFn != nil {
		return m.RepoNameFn()
	}
	return "test-repo"
}

func (m *MockService) CurrentUser(ctx context.Context) (string, error) {
	if m.CurrentUserFn != nil {
		return m.CurrentUserFn(ctx)
	}
	return "test-user", nil
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
	return &review.PullRequest{Number: number}, nil
}

func (m *MockService) FetchDiffFiles(ctx context.Context, number int) ([]diff.DiffFile, error) {
	if m.FetchDiffFilesFn != nil {
		return m.FetchDiffFilesFn(ctx, number)
	}
	return nil, nil
}

func (m *MockService) FetchExistingComments(ctx context.Context, number int) ([]review.ExistingComment, error) {
	if m.FetchExistingCommentsFn != nil {
		return m.FetchExistingCommentsFn(ctx, number)
	}
	return nil, nil
}

func (m *MockService) FetchPendingReview(ctx context.Context, number int) (int, string, []review.ExistingComment, error) {
	if m.FetchPendingReviewFn != nil {
		return m.FetchPendingReviewFn(ctx, number)
	}
	return 0, "", nil, nil
}

func (m *MockService) CreatePendingReview(ctx context.Context, prNumber int) (int, string, error) {
	if m.CreatePendingReviewFn != nil {
		return m.CreatePendingReviewFn(ctx, prNumber)
	}
	return 1, "R_node1", nil
}

func (m *MockService) AddReviewComment(ctx context.Context, reviewNodeID string, comment review.InlineComment) (int, error) {
	if m.AddReviewCommentFn != nil {
		return m.AddReviewCommentFn(ctx, reviewNodeID, comment)
	}
	return 1, nil
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

func (m *MockService) SubmitReview(ctx context.Context, rev *review.PendingReview) error {
	if m.SubmitReviewFn != nil {
		return m.SubmitReviewFn(ctx, rev)
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
