package testutil

import (
	"context"

	"github.com/jethrokuan/pry/internal/diff"
	"github.com/jethrokuan/pry/internal/review"
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
	FetchCommentsFn       func(ctx context.Context, number int) ([]review.Comment, error)
	FetchPendingReviewFn  func(ctx context.Context, number int) (int, string, []review.Comment, error)
	CreatePendingReviewFn func(ctx context.Context, prNumber int) (int, string, error)
	AddReviewCommentFn    func(ctx context.Context, prNumber int, reviewNodeID string, path string, line, startLine int, side, body string) (int, int, string, error)
	DeleteReviewCommentFn   func(ctx context.Context, prNumber, commentID int) error
	EditReviewCommentFn     func(ctx context.Context, prNumber, commentID int, body string) error
	SubmitReviewFn          func(ctx context.Context, pr *review.PullRequest, review *review.PendingReview) error
	FetchViewedFilesFn      func(ctx context.Context, prNodeID string) (map[string]bool, error)
	MarkFileAsViewedFn      func(ctx context.Context, prNodeID, path string) error
	UnmarkFileAsViewedFn    func(ctx context.Context, prNodeID, path string) error
	ListMentionableUsersFn    func(ctx context.Context) ([]string, error)
	UploadImageFn             func(ctx context.Context, data []byte, filename string) (string, error)
	ClosePRFn                 func(ctx context.Context, prNumber int) error
	ReopenPRFn                func(ctx context.Context, prNumber int) error
	MergePRFn                 func(ctx context.Context, prNumber int) error
	MarkReadyForReviewFn      func(ctx context.Context, prNodeID string) error
	AssignPRFn                func(ctx context.Context, prNumber int, login string) error
	UnassignPRFn              func(ctx context.Context, prNumber int, login string) error
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

func (m *MockService) FetchComments(ctx context.Context, number int) ([]review.Comment, error) {
	if m.FetchCommentsFn != nil {
		return m.FetchCommentsFn(ctx, number)
	}
	return nil, nil
}

func (m *MockService) FetchPendingReview(ctx context.Context, number int) (int, string, []review.Comment, error) {
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

func (m *MockService) AddReviewComment(ctx context.Context, prNumber int, reviewNodeID string, path string, line, startLine int, side, body string) (int, int, string, error) {
	if m.AddReviewCommentFn != nil {
		return m.AddReviewCommentFn(ctx, prNumber, reviewNodeID, path, line, startLine, side, body)
	}
	return 1, 1, "R_node1", nil
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

func (m *MockService) SubmitReview(ctx context.Context, pr *review.PullRequest, rev *review.PendingReview) error {
	if m.SubmitReviewFn != nil {
		return m.SubmitReviewFn(ctx, pr, rev)
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

func (m *MockService) ListMentionableUsers(ctx context.Context) ([]string, error) {
	if m.ListMentionableUsersFn != nil {
		return m.ListMentionableUsersFn(ctx)
	}
	return nil, nil
}

func (m *MockService) UploadImage(ctx context.Context, data []byte, filename string) (string, error) {
	if m.UploadImageFn != nil {
		return m.UploadImageFn(ctx, data, filename)
	}
	return "", nil
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
