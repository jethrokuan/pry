package prlist

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/jethrokuan/pry/internal/review"
	"github.com/jethrokuan/pry/internal/review/reviewtest"
)

func newTestModel(svc *reviewtest.MockService, filters ...review.PRFilter) Model {
	if len(filters) == 0 {
		filters = []review.PRFilter{
			{Name: "Default", Qualifier: "is:open"},
		}
	}
	return New(svc, filters)
}

func samplePRs(n int) []review.PullRequest {
	prs := make([]review.PullRequest, n)
	for i := range n {
		prs[i] = review.PullRequest{
			Number: i + 1,
			Title:  fmt.Sprintf("PR %d", i+1),
			Author: "author",
		}
	}
	return prs
}

// loadModel creates a model, sends prsLoadedMsg and UserIdentityMsg,
// and returns the model in a ready (non-loading) state.
func loadModel(svc *reviewtest.MockService, prs []review.PullRequest, filters ...review.PRFilter) Model {
	m := newTestModel(svc, filters...)
	m, _ = m.Update(prsLoadedMsg{tabIdx: 0, prs: prs})
	m, _ = m.Update(UserIdentityMsg{Identity: &review.UserIdentity{Login: "testuser", Teams: nil}})
	return m
}

var _ = ginkgo.Describe("PRList Model", func() {

	ginkgo.Describe("prsLoadedMsg", func() {
		ginkgo.It("stores PRs and resets cursor", func() {
			svc := &reviewtest.MockService{}
			m := newTestModel(svc)
			gomega.Expect(m.tabs[0].loading).To(gomega.BeTrue())

			prs := samplePRs(3)
			m, _ = m.Update(prsLoadedMsg{tabIdx: 0, prs: prs})

			gomega.Expect(m.tabs[0].loading).To(gomega.BeFalse())
			gomega.Expect(m.tabs[0].prs).To(gomega.HaveLen(3))
			gomega.Expect(m.tabs[0].cur()).To(gomega.Equal(0))
			// err field removed; errors are now shown via flash
		})

		ginkgo.It("emits flash on failure", func() {
			svc := &reviewtest.MockService{}
			m := newTestModel(svc)

			m, cmd := m.Update(prsLoadedMsg{tabIdx: 0, err: fmt.Errorf("API failure")})

			gomega.Expect(m.tabs[0].loading).To(gomega.BeFalse())
			gomega.Expect(m.tabs[0].prs).To(gomega.BeNil())
			// Error is communicated via flash command, not stored in tab state.
			gomega.Expect(cmd).NotTo(gomega.BeNil())
		})

		ginkgo.It("routes response to correct tab", func() {
			svc := &reviewtest.MockService{}
			filters := []review.PRFilter{
				{Name: "Open", Qualifier: "is:open"},
				{Name: "Mine", Qualifier: "author:@me"},
			}
			m := newTestModel(svc, filters...)

			// Load tab 1 while tab 0 is active
			prs := samplePRs(2)
			m, _ = m.Update(prsLoadedMsg{tabIdx: 1, prs: prs})

			gomega.Expect(m.tabs[1].prs).To(gomega.HaveLen(2))
			gomega.Expect(m.tabs[1].fetched).To(gomega.BeTrue())
			// Tab 0 should still be loading
			gomega.Expect(m.tabs[0].prs).To(gomega.BeNil())
		})

		ginkgo.It("ignores out-of-range tab index", func() {
			svc := &reviewtest.MockService{}
			m := newTestModel(svc)

			m, _ = m.Update(prsLoadedMsg{tabIdx: 99, prs: samplePRs(1)})
			gomega.Expect(m.tabs[0].prs).To(gomega.BeNil())
		})
	})

	ginkgo.Describe("UserIdentityMsg", func() {
		ginkgo.It("populates userTeams map from identity", func() {
			svc := &reviewtest.MockService{}
			m := newTestModel(svc)

			m, _ = m.Update(UserIdentityMsg{Identity: &review.UserIdentity{
				Login: "user",
				Teams: []string{"org/team-a", "org/team-b"},
			}})

			gomega.Expect(m.userTeams).To(gomega.HaveLen(2))
			gomega.Expect(m.userTeams["org/team-a"]).To(gomega.BeTrue())
			gomega.Expect(m.userTeams["org/team-b"]).To(gomega.BeTrue())
		})

		ginkgo.It("creates empty map when no teams", func() {
			svc := &reviewtest.MockService{}
			m := newTestModel(svc)

			m, _ = m.Update(UserIdentityMsg{Identity: &review.UserIdentity{Login: "user", Teams: nil}})

			gomega.Expect(m.userTeams).NotTo(gomega.BeNil())
			gomega.Expect(m.userTeams).To(gomega.BeEmpty())
		})
	})

	ginkgo.Describe("keyboard navigation", func() {
		ginkgo.It("moves cursor down with j", func() {
			svc := &reviewtest.MockService{}
			m := loadModel(svc, samplePRs(5))

			m, _ = m.Update(tea.KeyPressMsg{Code: 'j'})
			gomega.Expect(m.tabs[0].cur()).To(gomega.Equal(1))

			m, _ = m.Update(tea.KeyPressMsg{Code: 'j'})
			gomega.Expect(m.tabs[0].cur()).To(gomega.Equal(2))
		})

		ginkgo.It("moves cursor up with k", func() {
			svc := &reviewtest.MockService{}
			m := loadModel(svc, samplePRs(5))
			m.tabs[0].setCursor(3)

			m, _ = m.Update(tea.KeyPressMsg{Code: 'k'})
			gomega.Expect(m.tabs[0].cur()).To(gomega.Equal(2))
		})

		ginkgo.It("clamps cursor at top", func() {
			svc := &reviewtest.MockService{}
			m := loadModel(svc, samplePRs(5))
			gomega.Expect(m.tabs[0].cur()).To(gomega.Equal(0))

			m, _ = m.Update(tea.KeyPressMsg{Code: 'k'})
			gomega.Expect(m.tabs[0].cur()).To(gomega.Equal(0))
		})

		ginkgo.It("clamps cursor at bottom", func() {
			svc := &reviewtest.MockService{}
			m := loadModel(svc, samplePRs(3))
			m.tabs[0].setCursor(2)

			m, _ = m.Update(tea.KeyPressMsg{Code: 'j'})
			gomega.Expect(m.tabs[0].cur()).To(gomega.Equal(2))
		})

		ginkgo.It("ignores navigation while loading", func() {
			svc := &reviewtest.MockService{}
			m := newTestModel(svc) // loading=true, cursor is nil

			m, _ = m.Update(tea.KeyPressMsg{Code: 'j'})
			gomega.Expect(m.tabs[0].cursor).To(gomega.BeNil())
		})
	})

	ginkgo.Describe("PR selection", func() {
		ginkgo.It("emits PRSelectedMsg on enter", func() {
			svc := &reviewtest.MockService{}
			prs := samplePRs(3)
			m := loadModel(svc, prs)
			m.tabs[0].setCursor(1)

			_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			gomega.Expect(cmd).NotTo(gomega.BeNil())

			msg := cmd()
			gomega.Expect(msg).To(gomega.BeAssignableToTypeOf(PRSelectedMsg{}))
			gomega.Expect(msg.(PRSelectedMsg).PR.Number).To(gomega.Equal(2))
		})

		ginkgo.It("does nothing on enter with empty list", func() {
			svc := &reviewtest.MockService{}
			m := loadModel(svc, nil)

			_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			gomega.Expect(cmd).To(gomega.BeNil())
		})
	})

	ginkgo.Describe("tab navigation", func() {
		var filters []review.PRFilter

		ginkgo.BeforeEach(func() {
			filters = []review.PRFilter{
				{Name: "Open", Qualifier: "is:open"},
				{Name: "Review Requested", Qualifier: "review-requested:@me"},
				{Name: "Mine", Qualifier: "author:@me"},
			}
		})

		ginkgo.It("switches to next tab with tab key", func() {
			svc := &reviewtest.MockService{}
			m := loadModel(svc, samplePRs(1), filters...)

			m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
			gomega.Expect(m.filterIdx).To(gomega.Equal(1))
			gomega.Expect(m.tabBar.Active()).To(gomega.Equal(1))
			gomega.Expect(m.tabs[1].loading).To(gomega.BeTrue()) // new tab triggers fetch
			gomega.Expect(cmd).NotTo(gomega.BeNil())
		})

		ginkgo.It("switches to prev tab with shift+tab", func() {
			svc := &reviewtest.MockService{}
			m := loadModel(svc, samplePRs(1), filters...)
			m.tabBar.SetActive(2)
			m.filterIdx = 2

			m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
			gomega.Expect(m.filterIdx).To(gomega.Equal(1))
			gomega.Expect(m.tabBar.Active()).To(gomega.Equal(1))
			gomega.Expect(m.tabs[1].loading).To(gomega.BeTrue()) // new tab triggers fetch
			gomega.Expect(cmd).NotTo(gomega.BeNil())
		})

		ginkgo.It("does not go past last tab", func() {
			svc := &reviewtest.MockService{}
			m := loadModel(svc, samplePRs(1), filters...)
			m.tabBar.SetActive(2)
			m.filterIdx = 2

			m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
			gomega.Expect(m.filterIdx).To(gomega.Equal(2))
			gomega.Expect(cmd).To(gomega.BeNil())
		})

		ginkgo.It("does not go before first tab", func() {
			svc := &reviewtest.MockService{}
			m := loadModel(svc, samplePRs(1), filters...)

			m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
			gomega.Expect(m.filterIdx).To(gomega.Equal(0))
			gomega.Expect(cmd).To(gomega.BeNil())
		})

		ginkgo.It("switches tab by number key", func() {
			svc := &reviewtest.MockService{}
			m := loadModel(svc, samplePRs(1), filters...)

			m, cmd := m.Update(tea.KeyPressMsg{Code: '2'})
			gomega.Expect(m.filterIdx).To(gomega.Equal(1))
			gomega.Expect(m.tabBar.Active()).To(gomega.Equal(1))
			gomega.Expect(m.tabs[1].loading).To(gomega.BeTrue()) // new tab triggers fetch
			gomega.Expect(cmd).NotTo(gomega.BeNil())
		})

		ginkgo.It("returns cached data instantly when switching back", func() {
			svc := &reviewtest.MockService{}
			filters := []review.PRFilter{
				{Name: "Open", Qualifier: "is:open"},
				{Name: "Mine", Qualifier: "author:@me"},
			}
			m := loadModel(svc, samplePRs(3), filters...)
			// Tab 0 is loaded with 3 PRs, move cursor
			m.tabs[0].setCursor(2)

			// Switch to tab 1, load it
			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
			m, _ = m.Update(prsLoadedMsg{tabIdx: 1, prs: samplePRs(5)})
			gomega.Expect(m.tabs[1].prs).To(gomega.HaveLen(5))

			// Switch back to tab 0 — cursor should be preserved
			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
			gomega.Expect(m.filterIdx).To(gomega.Equal(0))
			gomega.Expect(m.tabs[0].cur()).To(gomega.Equal(2))
			gomega.Expect(m.tabs[0].prs).To(gomega.HaveLen(3))
		})
	})

	ginkgo.Describe("filter editing", func() {
		ginkgo.It("enters edit mode with /", func() {
			svc := &reviewtest.MockService{}
			m := loadModel(svc, samplePRs(1))

			m, _ = m.Update(tea.KeyPressMsg{Code: '/'})
			gomega.Expect(m.editing).To(gomega.BeTrue())
			gomega.Expect(m.customFilter).To(gomega.BeNil())
		})

		ginkgo.It("exits edit mode with esc", func() {
			svc := &reviewtest.MockService{}
			m := loadModel(svc, samplePRs(1))
			m.editing = true

			m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
			gomega.Expect(m.editing).To(gomega.BeFalse())
			gomega.Expect(cmd).To(gomega.BeNil())
		})

		ginkgo.It("submits custom filter on enter", func() {
			svc := &reviewtest.MockService{}
			m := loadModel(svc, samplePRs(1))
			m.editing = true
			m.filterInput.SetValue("author:octocat")

			m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			gomega.Expect(m.editing).To(gomega.BeFalse())
			gomega.Expect(m.customFilter).NotTo(gomega.BeNil())
			gomega.Expect(m.customFilter.Name).To(gomega.Equal("Custom"))
			gomega.Expect(m.customFilter.Qualifier).To(gomega.Equal("author:octocat"))
			gomega.Expect(m.tabs[0].loading).To(gomega.BeFalse()) // non-blocking refresh (tab has data)
			gomega.Expect(cmd).NotTo(gomega.BeNil())
		})
	})

	ginkgo.Describe("refresh", func() {
		ginkgo.It("triggers reload on r", func() {
			svc := &reviewtest.MockService{}
			m := loadModel(svc, samplePRs(1))

			m, cmd := m.Update(tea.KeyPressMsg{Code: 'r'})
			gomega.Expect(m.tabs[0].loading).To(gomega.BeFalse()) // non-blocking refresh
			gomega.Expect(cmd).NotTo(gomega.BeNil())
		})
	})

	ginkgo.Describe("window size", func() {
		ginkgo.It("stores dimensions from WindowSizeMsg", func() {
			svc := &reviewtest.MockService{}
			m := newTestModel(svc)

			m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
			gomega.Expect(m.width).To(gomega.Equal(120))
			gomega.Expect(m.height).To(gomega.Equal(40))
		})
	})

	ginkgo.Describe("activeFilter", func() {
		ginkgo.It("returns preset filter by default", func() {
			svc := &reviewtest.MockService{}
			filters := []review.PRFilter{
				{Name: "Open", Qualifier: "is:open"},
				{Name: "Mine", Qualifier: "author:@me"},
			}
			m := newTestModel(svc, filters...)
			m.filterIdx = 1

			gomega.Expect(m.activeFilter().Name).To(gomega.Equal("Mine"))
		})

		ginkgo.It("returns custom filter when set", func() {
			svc := &reviewtest.MockService{}
			m := newTestModel(svc)
			m.customFilter = &review.PRFilter{Name: "Custom", Qualifier: "label:bug"}

			gomega.Expect(m.activeFilter().Name).To(gomega.Equal("Custom"))
			gomega.Expect(m.activeFilter().Qualifier).To(gomega.Equal("label:bug"))
		})
	})
})
