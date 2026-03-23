package submit

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/jethrokuan/pry/internal/review"
	"github.com/jethrokuan/pry/internal/review/reviewtest"
)

func newTestModel(svc *reviewtest.MockService) Model {
	pr := &review.PullRequest{
		Number:        42,
		Title:         "Test PR",
		PendingReview: review.NewPendingReview(),
	}
	return New(svc, pr)
}

func sized(m Model) Model {
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return m
}

func pressKey(m Model, k rune) (Model, tea.Cmd) {
	return m.Update(tea.KeyPressMsg{Code: k})
}

func pressEsc(m Model) (Model, tea.Cmd) {
	return m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
}

func pressCtrlS(m Model) (Model, tea.Cmd) {
	return m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
}

var _ = ginkgo.Describe("Submit Model", func() {

	ginkgo.Describe("New", func() {
		ginkgo.It("initializes with default comment action", func() {
			svc := &reviewtest.MockService{}
			m := newTestModel(svc)
			gomega.Expect(m.pendingReview.Event).To(gomega.Equal(review.ReviewEventComment))
			gomega.Expect(m.submitting).To(gomega.BeFalse())
			gomega.Expect(m.submitted).To(gomega.BeFalse())
		})
	})

	ginkgo.Describe("WindowSizeMsg", func() {
		ginkgo.It("sets dimensions", func() {
			svc := &reviewtest.MockService{}
			m := newTestModel(svc)
			m = sized(m)
			gomega.Expect(m.width).To(gomega.Equal(120))
			gomega.Expect(m.height).To(gomega.Equal(40))
		})
	})

	ginkgo.Describe("action selection", func() {
		ginkgo.It("selects Comment with key 1", func() {
			svc := &reviewtest.MockService{}
			m := sized(newTestModel(svc))
			m, _ = pressKey(m, '2')
			gomega.Expect(m.pendingReview.Event).To(gomega.Equal(review.ReviewEventApprove))
			m, _ = pressKey(m, '1')
			gomega.Expect(m.pendingReview.Event).To(gomega.Equal(review.ReviewEventComment))
		})

		ginkgo.It("selects Approve with key 2", func() {
			svc := &reviewtest.MockService{}
			m := sized(newTestModel(svc))
			m, _ = pressKey(m, '2')
			gomega.Expect(m.pendingReview.Event).To(gomega.Equal(review.ReviewEventApprove))
		})

		ginkgo.It("selects Request Changes with key 3", func() {
			svc := &reviewtest.MockService{}
			m := sized(newTestModel(svc))
			m, _ = pressKey(m, '3')
			gomega.Expect(m.pendingReview.Event).To(gomega.Equal(review.ReviewEventRequestChanges))
		})
	})

	ginkgo.Describe("cancel", func() {
		ginkgo.It("sends CancelledMsg on esc", func() {
			svc := &reviewtest.MockService{}
			m := sized(newTestModel(svc))
			var cmd tea.Cmd
			m, cmd = pressEsc(m)
			gomega.Expect(cmd).ToNot(gomega.BeNil())
			msg := cmd()
			gomega.Expect(msg).To(gomega.BeAssignableToTypeOf(CancelledMsg{}))
		})
	})

	ginkgo.Describe("body editing", func() {
		ginkgo.It("enters body focus with b key", func() {
			svc := &reviewtest.MockService{}
			m := sized(newTestModel(svc))
			m, _ = pressKey(m, 'b')
			gomega.Expect(m.focusBody).To(gomega.BeTrue())
		})

		ginkgo.It("exits body focus with esc", func() {
			svc := &reviewtest.MockService{}
			m := sized(newTestModel(svc))
			m, _ = pressKey(m, 'b')
			gomega.Expect(m.focusBody).To(gomega.BeTrue())
			m, _ = pressEsc(m)
			gomega.Expect(m.focusBody).To(gomega.BeFalse())
		})
	})

	ginkgo.Describe("submit", func() {
		ginkgo.It("sets submitting flag on ctrl+s", func() {
			svc := &reviewtest.MockService{
				SubmitReviewFn: func(_ context.Context, _ *review.PullRequest, _ *review.PendingReview) error {
					return nil
				},
			}
			m := sized(newTestModel(svc))
			m, cmd := pressCtrlS(m)
			gomega.Expect(m.submitting).To(gomega.BeTrue())
			gomega.Expect(cmd).ToNot(gomega.BeNil())
		})

		ginkgo.It("ignores keys while submitting", func() {
			svc := &reviewtest.MockService{}
			m := sized(newTestModel(svc))
			m.submitting = true
			m, cmd := pressKey(m, '2')
			gomega.Expect(cmd).To(gomega.BeNil())
			// Action should not change while submitting
			gomega.Expect(m.pendingReview.Event).To(gomega.Equal(review.ReviewEventComment))
		})

		ginkgo.It("handles submit error", func() {
			svc := &reviewtest.MockService{}
			m := sized(newTestModel(svc))
			m.submitting = true
			m, _ = m.Update(submitResultMsg{err: fmt.Errorf("network error")})
			gomega.Expect(m.submitting).To(gomega.BeFalse())
			gomega.Expect(m.err).To(gomega.MatchError("network error"))
			gomega.Expect(m.submitted).To(gomega.BeFalse())
		})

		ginkgo.It("sends SubmittedMsg on success", func() {
			svc := &reviewtest.MockService{}
			m := sized(newTestModel(svc))
			m.submitting = true
			var cmd tea.Cmd
			m, cmd = m.Update(submitResultMsg{err: nil})
			gomega.Expect(m.submitted).To(gomega.BeTrue())
			gomega.Expect(m.submitting).To(gomega.BeFalse())
			gomega.Expect(cmd).ToNot(gomega.BeNil())
			msg := cmd()
			gomega.Expect(msg).To(gomega.BeAssignableToTypeOf(SubmittedMsg{}))
		})
	})

	ginkgo.Describe("sync waiting", func() {
		ginkgo.It("waits for in-flight comments before submitting", func() {
			svc := &reviewtest.MockService{}
			m := sized(newTestModel(svc))
			m.pendingReview.AddComment("file.go", 10, "test comment")
			m.pendingReview.Comments[0].SyncStatus = review.SyncInFlight

			m, cmd := pressCtrlS(m)
			gomega.Expect(m.waitingForSync).To(gomega.BeTrue())
			gomega.Expect(cmd).ToNot(gomega.BeNil())
		})

		ginkgo.It("proceeds to submit when no in-flight comments", func() {
			svc := &reviewtest.MockService{}
			m := sized(newTestModel(svc))
			m.pendingReview.AddComment("file.go", 10, "test comment")
			m.pendingReview.Comments[0].SyncStatus = review.SyncComplete

			m, _ = pressCtrlS(m)
			gomega.Expect(m.submitting).To(gomega.BeTrue())
			gomega.Expect(m.waitingForSync).To(gomega.BeFalse())
		})

		ginkgo.It("transitions from waiting to submitting when sync completes", func() {
			svc := &reviewtest.MockService{}
			m := sized(newTestModel(svc))
			m.waitingForSync = true
			m, cmd := m.Update(waitForSyncMsg{})
			gomega.Expect(cmd).ToNot(gomega.BeNil())
		})

		ginkgo.It("continues waiting if comments still in-flight", func() {
			svc := &reviewtest.MockService{}
			m := sized(newTestModel(svc))
			m.waitingForSync = true
			m.pendingReview.AddComment("file.go", 10, "test")
			m.pendingReview.Comments[0].SyncStatus = review.SyncInFlight

			m, cmd := m.Update(waitForSyncMsg{})
			gomega.Expect(cmd).ToNot(gomega.BeNil())
		})
	})

	ginkgo.Describe("View", func() {
		ginkgo.It("returns empty string when width is 0", func() {
			svc := &reviewtest.MockService{}
			m := newTestModel(svc)
			gomega.Expect(m.View()).To(gomega.BeEmpty())
		})

		ginkgo.It("shows comment count", func() {
			svc := &reviewtest.MockService{}
			m := sized(newTestModel(svc))
			m.pendingReview.AddComment("file.go", 10, "fix this")
			m.pendingReview.AddComment("main.go", 20, "and this")
			view := m.View()
			gomega.Expect(view).To(gomega.ContainSubstring("2 inline comments pending"))
		})

		ginkgo.It("shows sync status icons", func() {
			svc := &reviewtest.MockService{}
			m := sized(newTestModel(svc))
			m.pendingReview.AddComment("file.go", 10, "pending comment")
			m.pendingReview.Comments[0].SyncStatus = review.SyncComplete
			view := m.View()
			gomega.Expect(view).To(gomega.ContainSubstring("✓"))
		})

		ginkgo.It("shows failed sync warning", func() {
			svc := &reviewtest.MockService{}
			m := sized(newTestModel(svc))
			m.pendingReview.AddComment("file.go", 10, "failed comment")
			m.pendingReview.Comments[0].SyncStatus = review.SyncFailed
			view := m.View()
			gomega.Expect(view).To(gomega.ContainSubstring("1 comment(s) failed to sync"))
		})

		ginkgo.It("shows error message after submit failure", func() {
			svc := &reviewtest.MockService{}
			m := sized(newTestModel(svc))
			m.err = fmt.Errorf("API error")
			view := m.View()
			gomega.Expect(view).To(gomega.ContainSubstring("Error: API error"))
			gomega.Expect(view).To(gomega.ContainSubstring("ctrl+s to retry"))
		})

		ginkgo.It("shows submitting spinner text", func() {
			svc := &reviewtest.MockService{}
			m := sized(newTestModel(svc))
			m.submitting = true
			view := m.View()
			gomega.Expect(view).To(gomega.ContainSubstring("Submitting review..."))
		})

		ginkgo.It("shows waiting for sync text", func() {
			svc := &reviewtest.MockService{}
			m := sized(newTestModel(svc))
			m.waitingForSync = true
			view := m.View()
			gomega.Expect(view).To(gomega.ContainSubstring("Waiting for comments to sync..."))
		})

		ginkgo.It("shows selected action", func() {
			svc := &reviewtest.MockService{}
			m := sized(newTestModel(svc))
			m.pendingReview.Event = review.ReviewEventApprove
			view := m.View()
			gomega.Expect(view).To(gomega.ContainSubstring("(x)"))
			gomega.Expect(view).To(gomega.ContainSubstring("Approve"))
		})
	})
})
