package tabbar

import (
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

func TestTabbar(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Tabbar Suite")
}

var _ = ginkgo.Describe("Tabbar", func() {
	ginkgo.Describe("New", func() {
		ginkgo.It("creates a tab bar with tabs", func() {
			tabs := []Tab{
				{Label: "PRs", Count: 5},
				{Label: "Issues", Count: -1},
			}
			m := New(tabs)
			gomega.Expect(m.Len()).To(gomega.Equal(2))
			gomega.Expect(m.Active()).To(gomega.Equal(0))
		})
	})

	ginkgo.Describe("navigation", func() {
		ginkgo.It("moves to next tab", func() {
			m := New([]Tab{{Label: "A"}, {Label: "B"}, {Label: "C"}})
			gomega.Expect(m.Next()).To(gomega.BeTrue())
			gomega.Expect(m.Active()).To(gomega.Equal(1))
		})

		ginkgo.It("clamps at last tab", func() {
			m := New([]Tab{{Label: "A"}, {Label: "B"}})
			m.SetActive(1)
			gomega.Expect(m.Next()).To(gomega.BeFalse())
			gomega.Expect(m.Active()).To(gomega.Equal(1))
		})

		ginkgo.It("moves to previous tab", func() {
			m := New([]Tab{{Label: "A"}, {Label: "B"}, {Label: "C"}})
			m.SetActive(2)
			gomega.Expect(m.Prev()).To(gomega.BeTrue())
			gomega.Expect(m.Active()).To(gomega.Equal(1))
		})

		ginkgo.It("clamps at first tab", func() {
			m := New([]Tab{{Label: "A"}, {Label: "B"}})
			gomega.Expect(m.Prev()).To(gomega.BeFalse())
			gomega.Expect(m.Active()).To(gomega.Equal(0))
		})
	})

	ginkgo.Describe("SetCount", func() {
		ginkgo.It("updates tab count", func() {
			m := New([]Tab{{Label: "PRs", Count: -1}})
			m.SetCount(0, 42)
			gomega.Expect(m.tabs[0].Count).To(gomega.Equal(42))
		})

		ginkgo.It("ignores out of range index", func() {
			m := New([]Tab{{Label: "PRs"}})
			m.SetCount(5, 42) // should not panic
		})
	})

	ginkgo.Describe("View", func() {
		ginkgo.It("renders tabs with counts", func() {
			m := New([]Tab{
				{Label: "Open", Count: 10},
				{Label: "Mine", Count: -1},
			})
			m.SetWidth(100)
			view := m.View()
			gomega.Expect(view).To(gomega.ContainSubstring("Open (10)"))
			gomega.Expect(view).To(gomega.ContainSubstring("Mine"))
			gomega.Expect(view).NotTo(gomega.ContainSubstring("Mine ("))
		})

		ginkgo.It("returns empty string for no tabs", func() {
			m := New(nil)
			gomega.Expect(m.View()).To(gomega.BeEmpty())
		})
	})
})
