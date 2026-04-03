package prlist

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/jethrokuan/pry/internal/review"
)

func newTestModel(filters ...review.PRFilter) Model {
	if len(filters) == 0 {
		filters = []review.PRFilter{
			{Name: "Default", Qualifier: "is:open"},
		}
	}
	return New(filters)
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
func loadModel(prs []review.PullRequest, filters ...review.PRFilter) Model {
	m := newTestModel(filters...)
	m, _ = m.Update(prsLoadedMsg{tabIdx: 0, prs: prs})
	m, _ = m.Update(UserIdentityMsg{Identity: &review.UserIdentity{Login: "testuser", Teams: nil}})
	return m
}

var _ = ginkgo.Describe("PRList Model", func() {

	ginkgo.Describe("prsLoadedMsg", func() {
		ginkgo.It("stores PRs and resets cursor", func() {
			m := newTestModel()
			gomega.Expect(m.tabs[0].loading).To(gomega.BeTrue())

			prs := samplePRs(3)
			m, _ = m.Update(prsLoadedMsg{tabIdx: 0, prs: prs})

			gomega.Expect(m.tabs[0].loading).To(gomega.BeFalse())
			gomega.Expect(m.tabs[0].prs).To(gomega.HaveLen(3))
			gomega.Expect(m.tabs[0].cur()).To(gomega.Equal(0))
			// err field removed; errors are now shown via flash
		})

		ginkgo.It("emits flash on failure", func() {
			m := newTestModel()

			m, cmd := m.Update(prsLoadedMsg{tabIdx: 0, err: fmt.Errorf("API failure")})

			gomega.Expect(m.tabs[0].loading).To(gomega.BeFalse())
			gomega.Expect(m.tabs[0].prs).To(gomega.BeNil())
			// Error is communicated via flash command, not stored in tab state.
			gomega.Expect(cmd).NotTo(gomega.BeNil())
		})

		ginkgo.It("routes response to correct tab", func() {
			filters := []review.PRFilter{
				{Name: "Open", Qualifier: "is:open"},
				{Name: "Mine", Qualifier: "author:@me"},
			}
			m := newTestModel(filters...)

			// Load tab 1 while tab 0 is active
			prs := samplePRs(2)
			m, _ = m.Update(prsLoadedMsg{tabIdx: 1, prs: prs})

			gomega.Expect(m.tabs[1].prs).To(gomega.HaveLen(2))
			// Tab 0 should still be loading
			gomega.Expect(m.tabs[0].prs).To(gomega.BeNil())
		})

		ginkgo.It("ignores out-of-range tab index", func() {
			m := newTestModel()

			m, _ = m.Update(prsLoadedMsg{tabIdx: 99, prs: samplePRs(1)})
			gomega.Expect(m.tabs[0].prs).To(gomega.BeNil())
		})
	})

	ginkgo.Describe("keyboard navigation", func() {
		ginkgo.It("moves cursor down with j", func() {
			m := loadModel(samplePRs(5))

			m, _ = m.Update(tea.KeyPressMsg{Code: 'j'})
			gomega.Expect(m.tabs[0].cur()).To(gomega.Equal(1))

			m, _ = m.Update(tea.KeyPressMsg{Code: 'j'})
			gomega.Expect(m.tabs[0].cur()).To(gomega.Equal(2))
		})

		ginkgo.It("moves cursor up with k", func() {
			m := loadModel(samplePRs(5))
			m.tabs[0].setCursor(3)

			m, _ = m.Update(tea.KeyPressMsg{Code: 'k'})
			gomega.Expect(m.tabs[0].cur()).To(gomega.Equal(2))
		})

		ginkgo.It("clamps cursor at top", func() {
			m := loadModel(samplePRs(5))
			gomega.Expect(m.tabs[0].cur()).To(gomega.Equal(0))

			m, _ = m.Update(tea.KeyPressMsg{Code: 'k'})
			gomega.Expect(m.tabs[0].cur()).To(gomega.Equal(0))
		})

		ginkgo.It("clamps cursor at bottom", func() {
			m := loadModel(samplePRs(3))
			m.tabs[0].setCursor(2)

			m, _ = m.Update(tea.KeyPressMsg{Code: 'j'})
			gomega.Expect(m.tabs[0].cur()).To(gomega.Equal(2))
		})

		ginkgo.It("ignores navigation while loading", func() {
			m := newTestModel() // loading=true, cursor is nil

			m, _ = m.Update(tea.KeyPressMsg{Code: 'j'})
			gomega.Expect(m.tabs[0].cursor).To(gomega.BeNil())
		})
	})

	ginkgo.Describe("PR selection", func() {
		ginkgo.It("emits PRSelectedMsg on enter", func() {
			prs := samplePRs(3)
			m := loadModel(prs)
			m.tabs[0].setCursor(1)

			_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			gomega.Expect(cmd).NotTo(gomega.BeNil())

			msg := cmd()
			gomega.Expect(msg).To(gomega.BeAssignableToTypeOf(PRSelectedMsg{}))
			gomega.Expect(msg.(PRSelectedMsg).PR.Number).To(gomega.Equal(2))
		})

		ginkgo.It("does nothing on enter with empty list", func() {
			m := loadModel(nil)

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
			m := loadModel(samplePRs(1), filters...)

			m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
			gomega.Expect(m.filterIdx).To(gomega.Equal(1))
			gomega.Expect(m.tabBar.Active()).To(gomega.Equal(1))
			gomega.Expect(m.tabs[1].loading).To(gomega.BeTrue()) // new tab triggers fetch
			gomega.Expect(cmd).NotTo(gomega.BeNil())
		})

		ginkgo.It("switches to prev tab with shift+tab", func() {
			m := loadModel(samplePRs(1), filters...)
			m.tabBar.SetActive(2)
			m.filterIdx = 2

			m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
			gomega.Expect(m.filterIdx).To(gomega.Equal(1))
			gomega.Expect(m.tabBar.Active()).To(gomega.Equal(1))
			gomega.Expect(m.tabs[1].loading).To(gomega.BeTrue()) // new tab triggers fetch
			gomega.Expect(cmd).NotTo(gomega.BeNil())
		})

		ginkgo.It("does not go past last tab", func() {
			m := loadModel(samplePRs(1), filters...)
			m.tabBar.SetActive(2)
			m.filterIdx = 2

			m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
			gomega.Expect(m.filterIdx).To(gomega.Equal(2))
			gomega.Expect(cmd).To(gomega.BeNil())
		})

		ginkgo.It("does not go before first tab", func() {
			m := loadModel(samplePRs(1), filters...)

			m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
			gomega.Expect(m.filterIdx).To(gomega.Equal(0))
			gomega.Expect(cmd).To(gomega.BeNil())
		})

		ginkgo.It("switches tab by number key", func() {
			m := loadModel(samplePRs(1), filters...)

			m, cmd := m.Update(tea.KeyPressMsg{Code: '2'})
			gomega.Expect(m.filterIdx).To(gomega.Equal(1))
			gomega.Expect(m.tabBar.Active()).To(gomega.Equal(1))
			gomega.Expect(m.tabs[1].loading).To(gomega.BeTrue()) // new tab triggers fetch
			gomega.Expect(cmd).NotTo(gomega.BeNil())
		})

		ginkgo.It("returns cached data instantly when switching back", func() {
			filters := []review.PRFilter{
				{Name: "Open", Qualifier: "is:open"},
				{Name: "Mine", Qualifier: "author:@me"},
			}
			m := loadModel(samplePRs(3), filters...)
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
			m := loadModel(samplePRs(1))

			m, _ = m.Update(tea.KeyPressMsg{Code: '/'})
			gomega.Expect(m.editing).To(gomega.BeTrue())
			gomega.Expect(m.customFilter).To(gomega.BeNil())
		})

		ginkgo.It("exits edit mode with esc", func() {
			m := loadModel(samplePRs(1))
			m.editing = true

			m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
			gomega.Expect(m.editing).To(gomega.BeFalse())
			gomega.Expect(cmd).To(gomega.BeNil())
		})

		ginkgo.It("submits custom filter on enter", func() {
			m := loadModel(samplePRs(1))
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
			m := loadModel(samplePRs(1))

			m, cmd := m.Update(tea.KeyPressMsg{Code: 'r'})
			gomega.Expect(m.tabs[0].loading).To(gomega.BeFalse()) // non-blocking refresh
			gomega.Expect(cmd).NotTo(gomega.BeNil())
		})
	})

	ginkgo.Describe("window size", func() {
		ginkgo.It("stores dimensions from WindowSizeMsg", func() {
			m := newTestModel()

			m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
			gomega.Expect(m.width).To(gomega.Equal(120))
			gomega.Expect(m.height).To(gomega.Equal(40))
		})
	})

	ginkgo.Describe("filterAtTrigger", func() {
		ginkgo.It("detects @ at end of input", func() {
			p, idx := filterAtTrigger("author:@oct", 11)
			gomega.Expect(p).To(gomega.Equal("oct"))
			gomega.Expect(idx).To(gomega.Equal(7))
		})

		ginkgo.It("detects @ after other qualifiers", func() {
			p, idx := filterAtTrigger("label:bug author:@jet", 21)
			gomega.Expect(p).To(gomega.Equal("jet"))
			gomega.Expect(idx).To(gomega.Equal(17))
		})

		ginkgo.It("returns -1 when no @ in current token", func() {
			_, idx := filterAtTrigger("sometext", 8)
			gomega.Expect(idx).To(gomega.Equal(-1))
		})

		ginkgo.It("detects @ with empty prefix", func() {
			p, idx := filterAtTrigger("author:@", 8)
			gomega.Expect(p).To(gomega.Equal(""))
			gomega.Expect(idx).To(gomega.Equal(7))
		})

		ginkgo.It("detects @ when cursor is mid-input", func() {
			p, _ := filterAtTrigger("author:@oct label:bug", 11)
			gomega.Expect(p).To(gomega.Equal("oct"))
		})

		ginkgo.It("stops at space boundary", func() {
			_, idx := filterAtTrigger("label:bug noatsign", 18)
			gomega.Expect(idx).To(gomega.Equal(-1))
		})
	})

	ginkgo.Describe("filter autocomplete", func() {
		ginkgo.It("shows suggestions when typing @", func() {
			m := loadModel(samplePRs(1))
			m, _ = m.Update(MentionableUsersMsg{Users: []review.MentionableUser{
				{Login: "octocat", Name: "Octo Cat"},
				{Login: "jethro", Name: "Jethro Kuan"},
			}})

			m, _ = m.Update(tea.KeyPressMsg{Code: '/'})
			gomega.Expect(m.editing).To(gomega.BeTrue())

			m.filterInput.SetValue("author:@oct")
			m.filterInput.SetCursor(11)
			m.updateFilterAutocomplete()

			gomega.Expect(m.filterAC.IsActive()).To(gomega.BeTrue())
			gomega.Expect(m.filterAC.Selected().Value).To(gomega.Equal("@octocat"))
		})

		ginkgo.It("hides autocomplete when no @ in token", func() {
			m := loadModel(samplePRs(1))
			m, _ = m.Update(MentionableUsersMsg{Users: []review.MentionableUser{
				{Login: "octocat", Name: "Octo Cat"},
			}})

			m, _ = m.Update(tea.KeyPressMsg{Code: '/'})
			m.filterInput.SetValue("label:bug")
			m.filterInput.SetCursor(9)
			m.updateFilterAutocomplete()

			gomega.Expect(m.filterAC.IsActive()).To(gomega.BeFalse())
		})

		ginkgo.It("completes @prefix on tab", func() {
			m := loadModel(samplePRs(1))
			m, _ = m.Update(MentionableUsersMsg{Users: []review.MentionableUser{
				{Login: "octocat", Name: "Octo Cat"},
				{Login: "jethro", Name: "Jethro Kuan"},
			}})

			m, _ = m.Update(tea.KeyPressMsg{Code: '/'})
			m.filterInput.SetValue("author:@oct")
			m.filterInput.SetCursor(11)
			m.updateFilterAutocomplete()

			gomega.Expect(m.filterAC.IsActive()).To(gomega.BeTrue())

			m.completeFilterAutocomplete()

			gomega.Expect(m.filterInput.Value()).To(gomega.Equal("author:@octocat "))
			gomega.Expect(m.filterAC.IsActive()).To(gomega.BeFalse())
		})

		ginkgo.It("completes @prefix in middle of input", func() {
			m := loadModel(samplePRs(1))
			m, _ = m.Update(MentionableUsersMsg{Users: []review.MentionableUser{
				{Login: "octocat", Name: "Octo Cat"},
			}})

			m, _ = m.Update(tea.KeyPressMsg{Code: '/'})
			m.filterInput.SetValue("author:@oct label:bug")
			m.filterInput.SetCursor(11)
			m.updateFilterAutocomplete()

			gomega.Expect(m.filterAC.IsActive()).To(gomega.BeTrue())

			m.completeFilterAutocomplete()

			gomega.Expect(m.filterInput.Value()).To(gomega.Equal("author:@octocat label:bug"))
		})

		ginkgo.It("works with reviewer: qualifier", func() {
			m := loadModel(samplePRs(1))
			m, _ = m.Update(MentionableUsersMsg{Users: []review.MentionableUser{
				{Login: "octocat", Name: "Octo Cat"},
			}})

			m, _ = m.Update(tea.KeyPressMsg{Code: '/'})
			m.filterInput.SetValue("reviewer:@oct")
			m.filterInput.SetCursor(13)
			m.updateFilterAutocomplete()

			gomega.Expect(m.filterAC.IsActive()).To(gomega.BeTrue())

			m.completeFilterAutocomplete()

			gomega.Expect(m.filterInput.Value()).To(gomega.Equal("reviewer:@octocat "))
		})

		ginkgo.It("shows all users with bare @", func() {
			m := loadModel(samplePRs(1))
			m, _ = m.Update(MentionableUsersMsg{Users: []review.MentionableUser{
				{Login: "octocat", Name: "Octo Cat"},
				{Login: "jethro", Name: "Jethro Kuan"},
			}})

			m, _ = m.Update(tea.KeyPressMsg{Code: '/'})
			m.filterInput.SetValue("author:@")
			m.filterInput.SetCursor(8)
			m.updateFilterAutocomplete()

			gomega.Expect(m.filterAC.IsActive()).To(gomega.BeTrue())
		})
	})

	ginkgo.Describe("activeFilter", func() {
		ginkgo.It("returns preset filter by default", func() {
			filters := []review.PRFilter{
				{Name: "Open", Qualifier: "is:open"},
				{Name: "Mine", Qualifier: "author:@me"},
			}
			m := newTestModel(filters...)
			m.filterIdx = 1

			gomega.Expect(m.activeFilter().Name).To(gomega.Equal("Mine"))
		})

		ginkgo.It("returns custom filter when set", func() {
			m := newTestModel()
			m.customFilter = &review.PRFilter{Name: "Custom", Qualifier: "label:bug"}

			gomega.Expect(m.activeFilter().Name).To(gomega.Equal("Custom"))
			gomega.Expect(m.activeFilter().Qualifier).To(gomega.Equal("label:bug"))
		})
	})
})
