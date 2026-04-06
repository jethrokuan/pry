package app

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jethrokuan/pry/internal/config"
	"github.com/jethrokuan/pry/internal/review"
	"github.com/jethrokuan/pry/internal/ui/diffview"
	"github.com/jethrokuan/pry/internal/ui/prlist"
	"github.com/jethrokuan/pry/internal/ui/submit"
)

// defaultFilters provides a minimal filter set so prlist.Init() doesn't panic.
var defaultFilters = []review.PRFilter{{Name: "Default", Qualifier: "is:open"}}

func newTestModel() Model {
	return New(config.Config{}, defaultFilters)
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

	Describe("userIdentityMsg", func() {
		It("stores the user identity", func() {
			identity := &review.UserIdentity{
				Login: "testuser",
				Teams: []string{"org/team1"},
			}
			m = update(m, userIdentityMsg{identity: identity})

			Expect(m.userIdentity).NotTo(BeNil())
			Expect(m.userIdentity.Login).To(Equal("testuser"))
			Expect(m.userIdentity.Teams).To(ConsistOf("org/team1"))
		})

		It("ignores errors", func() {
			m = update(m, userIdentityMsg{err: context.DeadlineExceeded})
			Expect(m.userIdentity).To(BeNil())
		})

		It("forwards identity to diffview when on diff screen", func() {
			m = update(m, prlist.PRSelectedMsg{PR: testPR()})
			Expect(m.screen).To(Equal(ScreenDiffView))

			identity := &review.UserIdentity{
				Login: "testuser",
				Teams: []string{"org/team1"},
			}
			m = update(m, userIdentityMsg{identity: identity})

			Expect(m.userIdentity).NotTo(BeNil())
			Expect(m.userIdentity.Login).To(Equal("testuser"))
		})
	})

	Describe("NewWithPR", func() {
		It("starts on the diff view screen", func() {
			m := NewWithPR(config.Config{}, 99, defaultFilters)

			Expect(m.screen).To(Equal(ScreenDiffView))
			Expect(m.initialPR).To(Equal(99))
			Expect(m.selectedPR.PendingReview).NotTo(BeNil())
			Expect(m.selectedPR.PendingReview.Event).To(Equal(review.ReviewEventComment))
		})
	})

	Describe("GoToPRMsg", func() {
		BeforeEach(func() {
			m = update(m, prlist.GoToPRMsg{Number: 99})
		})

		It("transitions to the diff view screen", func() {
			Expect(m.screen).To(Equal(ScreenDiffView))
		})

		It("creates a PR with only the number set", func() {
			Expect(m.selectedPR).NotTo(BeNil())
			Expect(m.selectedPR.Number).To(Equal(99))
			Expect(m.selectedPR.Title).To(BeEmpty())
		})

		It("starts a pending review", func() {
			Expect(m.selectedPR.PendingReview).NotTo(BeNil())
			Expect(m.selectedPR.PendingReview.Event).To(Equal(review.ReviewEventComment))
		})
	})

	Describe("prBodyLoadedMsg state preservation", func() {
		BeforeEach(func() {
			m = update(m, prlist.PRSelectedMsg{PR: testPR()})
			Expect(m.screen).To(Equal(ScreenDiffView))
		})

		It("preserves PendingReview when enriching PR", func() {
			originalReview := m.selectedPR.PendingReview
			Expect(originalReview).NotTo(BeNil())

			enriched := &review.PullRequest{
				Number: 42,
				Title:  "Enriched Title",
				Body:   "Full body text",
			}
			m = update(m, prBodyLoadedMsg{pr: enriched})

			Expect(m.selectedPR.Title).To(Equal("Enriched Title"))
			Expect(m.selectedPR.Body).To(Equal("Full body text"))
			Expect(m.selectedPR.PendingReview).To(Equal(originalReview))
		})

		It("preserves Threads when enriching PR", func() {
			m.selectedPR.Threads = []review.Thread{
				{Path: "main.go", Line: 10, Comments: []review.Comment{{Body: "fix this"}}},
			}

			enriched := &review.PullRequest{
				Number: 42,
				Title:  "Enriched Title",
			}
			m = update(m, prBodyLoadedMsg{pr: enriched})

			Expect(m.selectedPR.Threads).To(HaveLen(1))
			Expect(m.selectedPR.Threads[0].Path).To(Equal("main.go"))
		})

		It("does not update PR when error occurs", func() {
			m = update(m, prBodyLoadedMsg{err: fmt.Errorf("network error")})

			Expect(m.selectedPR.Number).To(Equal(42))
			Expect(m.selectedPR.Title).To(Equal("Test PR"))
		})

		It("does not update PR when pr is nil", func() {
			m = update(m, prBodyLoadedMsg{pr: nil})

			Expect(m.selectedPR.Number).To(Equal(42))
			Expect(m.selectedPR.Title).To(Equal("Test PR"))
		})
	})

	Describe("mentionableUsersMsg", func() {
		It("stores mentionable users", func() {
			users := []review.MentionableUser{
				{Login: "alice", Name: "Alice"},
				{Login: "bob", Name: "Bob"},
			}
			m = update(m, mentionableUsersMsg{users: users})

			Expect(m.mentionableUsers).To(HaveLen(2))
			Expect(m.mentionableUsers[0].Login).To(Equal("alice"))
			Expect(m.mentionableUsers[1].Login).To(Equal("bob"))
		})

		It("ignores errors", func() {
			m = update(m, mentionableUsersMsg{err: fmt.Errorf("fetch failed")})
			Expect(m.mentionableUsers).To(BeNil())
		})

		It("forwards to diffview when on diff screen", func() {
			m = update(m, prlist.PRSelectedMsg{PR: testPR()})
			Expect(m.screen).To(Equal(ScreenDiffView))

			users := []review.MentionableUser{{Login: "alice"}}
			m = update(m, mentionableUsersMsg{users: users})

			Expect(m.mentionableUsers).To(HaveLen(1))
		})
	})

	Describe("CmdPanicMsg", func() {
		It("does not crash or change screen", func() {
			originalScreen := m.screen
			m = update(m, CmdPanicMsg{Err: fmt.Errorf("something panicked")})

			Expect(m.screen).To(Equal(originalScreen))
		})
	})

	Describe("prBodyLoadedMsg on wrong screen", func() {
		It("is ignored when on PR list screen", func() {
			Expect(m.screen).To(Equal(ScreenPRList))

			enriched := &review.PullRequest{Number: 42, Title: "Should be ignored"}
			m = update(m, prBodyLoadedMsg{pr: enriched})

			Expect(m.selectedPR).To(BeNil())
		})
	})

	Describe("SubmitReviewMsg preserves review state", func() {
		It("copies pending review to selectedPR before entering submit screen", func() {
			m = update(m, prlist.PRSelectedMsg{PR: testPR()})
			Expect(m.screen).To(Equal(ScreenDiffView))
			Expect(m.selectedPR.PendingReview).NotTo(BeNil())

			m = update(m, diffview.SubmitReviewMsg{})
			Expect(m.screen).To(Equal(ScreenSubmit))
			Expect(m.selectedPR.PendingReview).NotTo(BeNil())
		})
	})

	Describe("SubmittedMsg returns to PR list", func() {
		It("returns to PR list and clears selected PR after submission", func() {
			m = update(m, prlist.PRSelectedMsg{PR: testPR()})
			m = update(m, diffview.SubmitReviewMsg{})
			Expect(m.screen).To(Equal(ScreenSubmit))

			m = update(m, submit.SubmittedMsg{})
			Expect(m.screen).To(Equal(ScreenPRList))
			Expect(m.selectedPR).To(BeNil())
		})
	})

	Describe("windowSizeCmd", func() {
		It("returns nil when width is zero", func() {
			Expect(m.width).To(Equal(0))
			cmd := m.windowSizeCmd()
			Expect(cmd).To(BeNil())
		})

		It("returns a command when dimensions are set", func() {
			m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
			cmd := m.windowSizeCmd()
			Expect(cmd).NotTo(BeNil())

			msg := cmd()
			wsm, ok := msg.(tea.WindowSizeMsg)
			Expect(ok).To(BeTrue())
			Expect(wsm.Width).To(Equal(120))
			Expect(wsm.Height).To(Equal(40))
		})
	})

	Describe("diffviewOpts", func() {
		It("includes user identity when set", func() {
			m.userIdentity = &review.UserIdentity{
				Login: "testuser",
				Teams: []string{"org/team1"},
			}
			opts := m.diffviewOpts()
			Expect(len(opts)).To(BeNumerically(">=", 2))
		})

		It("includes mentionable users when set", func() {
			m.mentionableUsers = []review.MentionableUser{{Login: "alice"}}
			opts := m.diffviewOpts()
			Expect(len(opts)).To(BeNumerically(">=", 1))
		})

		It("returns no identity or mentionable options by default", func() {
			m.aiAgent = nil
			m.useJJ = false
			opts := m.diffviewOpts()
			Expect(opts).To(BeEmpty())
		})

		It("includes owner filter option when disabled in config", func() {
			disabled := false
			m.cfg.FileTree.OwnerFilter = &disabled
			opts := m.diffviewOpts()
			Expect(len(opts)).To(BeNumerically(">=", 1))
		})
	})

	Describe("userIdentityMsg on PR list screen", func() {
		It("forwards identity to prlist when on PR list screen", func() {
			Expect(m.screen).To(Equal(ScreenPRList))

			identity := &review.UserIdentity{
				Login: "testuser",
				Teams: []string{"org/team1"},
			}
			m = update(m, userIdentityMsg{identity: identity})

			Expect(m.userIdentity).NotTo(BeNil())
			Expect(m.userIdentity.Login).To(Equal("testuser"))
		})
	})

	Describe("WindowSizeMsg", func() {
		It("stores width and height", func() {
			m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})

			Expect(m.width).To(Equal(120))
			Expect(m.height).To(Equal(40))
		})
	})

	Describe("screen routing falls through to sub-model", func() {
		It("forwards unhandled messages to prlist on PR list screen", func() {
			m = update(m, tea.WindowSizeMsg{Width: 80, Height: 24})
			Expect(m.screen).To(Equal(ScreenPRList))
		})

		It("forwards unhandled messages to diffview on diff screen", func() {
			m = update(m, prlist.PRSelectedMsg{PR: testPR()})
			Expect(m.screen).To(Equal(ScreenDiffView))

			m = update(m, tea.WindowSizeMsg{Width: 80, Height: 24})
			Expect(m.screen).To(Equal(ScreenDiffView))
		})

		It("forwards unhandled messages to submit on submit screen", func() {
			m = update(m, prlist.PRSelectedMsg{PR: testPR()})
			m = update(m, diffview.SubmitReviewMsg{})
			Expect(m.screen).To(Equal(ScreenSubmit))

			m = update(m, tea.WindowSizeMsg{Width: 80, Height: 24})
			Expect(m.screen).To(Equal(ScreenSubmit))
		})
	})

	Describe("full navigation cycle", func() {
		It("can navigate PRList → DiffView → Submit → PRList", func() {
			Expect(m.screen).To(Equal(ScreenPRList))

			m = update(m, prlist.PRSelectedMsg{PR: testPR()})
			Expect(m.screen).To(Equal(ScreenDiffView))

			m = update(m, diffview.SubmitReviewMsg{})
			Expect(m.screen).To(Equal(ScreenSubmit))

			m = update(m, submit.SubmittedMsg{})
			Expect(m.screen).To(Equal(ScreenPRList))
			Expect(m.selectedPR).To(BeNil())
		})

		It("can navigate PRList → DiffView → Submit → DiffView → PRList", func() {
			m = update(m, prlist.PRSelectedMsg{PR: testPR()})
			m = update(m, diffview.SubmitReviewMsg{})
			Expect(m.screen).To(Equal(ScreenSubmit))

			m = update(m, submit.CancelledMsg{})
			Expect(m.screen).To(Equal(ScreenDiffView))

			m = update(m, diffview.BackMsg{})
			Expect(m.screen).To(Equal(ScreenPRList))
			Expect(m.selectedPR).To(BeNil())
		})
	})
})
