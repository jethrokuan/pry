package app

import (
	"context"

	tea "charm.land/bubbletea/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jethrokuan/pry/internal/config"
	"github.com/jethrokuan/pry/internal/diff"
	"github.com/jethrokuan/pry/internal/review"
	"github.com/jethrokuan/pry/internal/ui/diffview"
	"github.com/jethrokuan/pry/internal/ui/prdetail"
	"github.com/jethrokuan/pry/internal/ui/prlist"
	"github.com/jethrokuan/pry/internal/ui/submit"
)

// stubService is a minimal review.Service for testing app routing.
type stubService struct{}

func (s *stubService) RepoOwner() string                           { return "owner" }
func (s *stubService) RepoName() string                            { return "repo" }
func (s *stubService) CurrentUser(context.Context) (string, error) { return "testuser", nil }
func (s *stubService) UserTeams(context.Context) ([]string, error) { return nil, nil }
func (s *stubService) ListPRs(_ context.Context, _ review.PRFilter) ([]review.PullRequest, error) {
	return nil, nil
}
func (s *stubService) GetPR(_ context.Context, _ int) (*review.PullRequest, error) {
	return nil, nil
}
func (s *stubService) FetchDiffFiles(_ context.Context, _ int) ([]diff.DiffFile, error) {
	return nil, nil
}
func (s *stubService) FetchExistingComments(_ context.Context, _ int) ([]review.ExistingComment, error) {
	return nil, nil
}
func (s *stubService) FetchPendingReview(_ context.Context, _ int) (int, string, []review.ExistingComment, error) {
	return 0, "", nil, nil
}
func (s *stubService) CreatePendingReview(_ context.Context, _ int) (int, string, error) {
	return 0, "", nil
}
func (s *stubService) AddReviewComment(_ context.Context, _ string, _ review.InlineComment) (int, error) {
	return 0, nil
}
func (s *stubService) DeleteReviewComment(_ context.Context, _, _ int) error { return nil }
func (s *stubService) EditReviewComment(_ context.Context, _, _ int, _ string) error {
	return nil
}
func (s *stubService) SubmitReview(_ context.Context, _ *review.PullRequest, _ *review.PendingReview) error {
	return nil
}
func (s *stubService) FetchViewedFiles(_ context.Context, _ string) (map[string]bool, error) {
	return nil, nil
}
func (s *stubService) MarkFileAsViewed(_ context.Context, _, _ string) error   { return nil }
func (s *stubService) UnmarkFileAsViewed(_ context.Context, _, _ string) error { return nil }
func (s *stubService) ListMentionableUsers(_ context.Context) ([]string, error) {
	return nil, nil
}
func (s *stubService) UploadImage(_ context.Context, _ []byte, _ string) (string, error) {
	return "", nil
}

// defaultFilters provides a minimal filter set so prlist.Init() doesn't panic.
var defaultFilters = []review.PRFilter{{Name: "Default", Qualifier: "is:open"}}

func newTestModel() Model {
	return New(&stubService{}, config.Config{}, defaultFilters, nil)
}

func testPR() *review.PullRequest {
	return &review.PullRequest{
		Number:  42,
		NodeID:  "PR_node42",
		Title:   "Test PR",
		HeadSHA: "abc123",
	}
}

// update is a helper that calls Update and type-asserts the result back to Model.
func update(m Model, msg tea.Msg) Model {
	result, _ := m.Update(msg)
	return result.(Model)
}

var _ = Describe("App message routing", func() {
	var m Model

	BeforeEach(func() {
		m = newTestModel()
	})

	Describe("initial state", func() {
		It("starts on the PR list screen", func() {
			Expect(m.screen).To(Equal(ScreenPRList))
		})

		It("has no selected PR", func() {
			Expect(m.selectedPR).To(BeNil())
		})

		It("has no pending review", func() {
			Expect(m.selectedPR).To(BeNil())
		})
	})

	Describe("PRSelectedMsg", func() {
		BeforeEach(func() {
			m = update(m, prlist.PRSelectedMsg{PR: testPR()})
		})

		It("transitions to the diff view screen", func() {
			Expect(m.screen).To(Equal(ScreenDiffView))
		})

		It("stores the selected PR", func() {
			Expect(m.selectedPR).NotTo(BeNil())
			Expect(m.selectedPR.Number).To(Equal(42))
		})

		It("creates a pending review for the selected PR", func() {
			Expect(m.selectedPR.PendingReview).NotTo(BeNil())
			Expect(m.selectedPR.PendingReview.Event).To(Equal(review.ReviewEventComment))
		})
	})

	Describe("DiffView transitions", func() {
		BeforeEach(func() {
			m = update(m, prlist.PRSelectedMsg{PR: testPR()})
			Expect(m.screen).To(Equal(ScreenDiffView))
		})

		Describe("SubmitReviewMsg", func() {
			It("transitions to the submit screen", func() {
				m = update(m, diffview.SubmitReviewMsg{})
				Expect(m.screen).To(Equal(ScreenSubmit))
			})
		})

		Describe("BackMsg from diff view", func() {
			It("transitions back to the PR list", func() {
				m = update(m, diffview.BackMsg{})
				Expect(m.screen).To(Equal(ScreenPRList))
			})

			It("clears the selected PR and pending review", func() {
				m = update(m, diffview.BackMsg{})
				Expect(m.selectedPR).To(BeNil())
			})
		})

		Describe("prBodyLoadedMsg", func() {
			It("updates the selected PR in-place", func() {
				fullPR := testPR()
				fullPR.Title = "Updated Title"
				fullPR.NodeID = "PR_full_node"
				fullPR.HeadSHA = "def456"
				m = update(m, prBodyLoadedMsg{pr: fullPR})

				Expect(m.selectedPR.Title).To(Equal("Updated Title"))
				Expect(m.selectedPR.NodeID).To(Equal("PR_full_node"))
				Expect(m.selectedPR.HeadSHA).To(Equal("def456"))
			})
		})
	})

	Describe("Submit screen transitions", func() {
		BeforeEach(func() {
			m = update(m, prlist.PRSelectedMsg{PR: testPR()})
			m = update(m, diffview.SubmitReviewMsg{})
			Expect(m.screen).To(Equal(ScreenSubmit))
		})

		Describe("SubmittedMsg", func() {
			It("transitions back to the PR list", func() {
				m = update(m, submit.SubmittedMsg{})
				Expect(m.screen).To(Equal(ScreenPRList))
			})

			It("clears the selected PR and pending review", func() {
				m = update(m, submit.SubmittedMsg{})
				Expect(m.selectedPR).To(BeNil())
			})
		})

		Describe("CancelledMsg", func() {
			It("transitions back to the diff view", func() {
				m = update(m, submit.CancelledMsg{})
				Expect(m.screen).To(Equal(ScreenDiffView))
			})
		})
	})

	Describe("PRDetail transitions", func() {
		BeforeEach(func() {
			pr := testPR()
			m.selectedPR = pr
			m.prDetail = prdetail.New(pr)
			m.screen = ScreenPRDetail
		})

		Describe("StartReviewMsg", func() {
			It("transitions to the diff view", func() {
				m = update(m, prdetail.StartReviewMsg{PR: testPR()})
				Expect(m.screen).To(Equal(ScreenDiffView))
			})

			It("creates a pending review", func() {
				m = update(m, prdetail.StartReviewMsg{PR: testPR()})
				Expect(m.selectedPR.PendingReview).NotTo(BeNil())
				Expect(m.selectedPR.PendingReview.Event).To(Equal(review.ReviewEventComment))
			})
		})

		Describe("BackMsg from PR detail", func() {
			It("transitions to the PR list", func() {
				m = update(m, prdetail.BackMsg{})
				Expect(m.screen).To(Equal(ScreenPRList))
			})
		})

		Describe("prBodyLoadedMsg", func() {
			It("updates the selected PR when successful", func() {
				fullPR := testPR()
				fullPR.Body = "Full body text"
				m = update(m, prBodyLoadedMsg{pr: fullPR})

				Expect(m.selectedPR.Body).To(Equal("Full body text"))
			})
		})
	})

	Describe("userIdentityMsg", func() {
		It("stores the user identity", func() {
			identity := &review.UserIdentity{
				Login: "testuser",
				Teams: []string{"org/team1"},
			}
			m = update(m, userIdentityMsg{identity: identity})

			Expect(m.ctx.UserIdentity).NotTo(BeNil())
			Expect(m.ctx.UserIdentity.Login).To(Equal("testuser"))
			Expect(m.ctx.UserIdentity.Teams).To(ConsistOf("org/team1"))
		})

		It("ignores errors", func() {
			m = update(m, userIdentityMsg{err: context.DeadlineExceeded})
			Expect(m.ctx.UserIdentity).To(BeNil())
		})

		It("forwards identity to diffview when on diff screen", func() {
			m = update(m, prlist.PRSelectedMsg{PR: testPR()})
			Expect(m.screen).To(Equal(ScreenDiffView))

			identity := &review.UserIdentity{
				Login: "testuser",
				Teams: []string{"org/team1"},
			}
			m = update(m, userIdentityMsg{identity: identity})

			Expect(m.ctx.UserIdentity).NotTo(BeNil())
			Expect(m.ctx.UserIdentity.Login).To(Equal("testuser"))
		})
	})

	Describe("NewWithPR", func() {
		It("starts on the diff view screen", func() {
			m := NewWithPR(&stubService{}, config.Config{}, 99, defaultFilters, nil)

			Expect(m.screen).To(Equal(ScreenDiffView))
			Expect(m.initialPR).To(Equal(99))
			Expect(m.selectedPR.PendingReview).NotTo(BeNil())
			Expect(m.selectedPR.PendingReview.Event).To(Equal(review.ReviewEventComment))
		})
	})

	Describe("WindowSizeMsg", func() {
		It("stores width and height", func() {
			m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})

			Expect(m.width).To(Equal(120))
			Expect(m.height).To(Equal(40))
		})
	})
})
