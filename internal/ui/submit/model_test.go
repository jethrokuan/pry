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

const testUser = "testuser"

func newTestModel(svc *reviewtest.MockService) Model {
	pr := &review.PullRequest{
		Number:        42,
		Title:         "Test PR",
		PendingReview: review.NewPendingReview(),
	}
	return New(svc, pr, testUser)
}

func newTestModelWithThreads(svc *reviewtest.MockService, threads []review.Thread) Model {
	pr := &review.PullRequest{
		Number:        42,
		Title:         "Test PR",
		PendingReview: review.NewPendingReview(),
		Threads:       threads,
	}
	return New(svc, pr, testUser)
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

		ginkgo.It("stores the current user", func() {
			svc := &reviewtest.MockService{}
			m := newTestModel(svc)
			gomega.Expect(m.currentUser).To(gomega.Equal(testUser))
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

		ginkgo.It("calls svc.SubmitReview directly", func() {
			var submitted bool
			svc := &reviewtest.MockService{
				SubmitReviewFn: func(_ context.Context, _ *review.PullRequest, _ *review.PendingReview) error {
					submitted = true
					return nil
				},
			}
			m := sized(newTestModel(svc))
			_, cmd := pressCtrlS(m)
			gomega.Expect(cmd).ToNot(gomega.BeNil())
			// Execute the batch command to trigger the submit
			// The batch contains spinner tick and submit func
			msgs := executeBatch(cmd)
			// After executing, the mock should have been called
			gomega.Expect(submitted).To(gomega.BeTrue())
			_ = msgs
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
			threads := []review.Thread{
				{Path: "file.go", Line: 10, Side: "RIGHT", Comments: []review.Comment{
					{ID: 1, Body: "fix this", Author: testUser, IsPending: true},
				}},
				{Path: "main.go", Line: 20, Side: "RIGHT", Comments: []review.Comment{
					{ID: 2, Body: "and this", Author: testUser, IsPending: true},
				}},
			}
			m := sized(newTestModelWithThreads(svc, threads))
			view := m.View()
			gomega.Expect(view).To(gomega.ContainSubstring("2 inline comments pending"))
		})

		ginkgo.It("does not count other users pending comments", func() {
			svc := &reviewtest.MockService{}
			threads := []review.Thread{
				{Path: "file.go", Line: 10, Side: "RIGHT", Comments: []review.Comment{
					{ID: 1, Body: "mine", Author: testUser, IsPending: true},
				}},
				{Path: "file.go", Line: 20, Side: "RIGHT", Comments: []review.Comment{
					{ID: 2, Body: "theirs", Author: "other", IsPending: true},
				}},
			}
			m := sized(newTestModelWithThreads(svc, threads))
			view := m.View()
			gomega.Expect(view).To(gomega.ContainSubstring("1 inline comments pending"))
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

// executeBatch runs a tea.Cmd and collects results. For batch commands,
// it runs all sub-commands. Returns all messages produced.
func executeBatch(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var msgs []tea.Msg
		for _, c := range batch {
			if c != nil {
				msgs = append(msgs, c())
			}
		}
		return msgs
	}
	return []tea.Msg{msg}
}
