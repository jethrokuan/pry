package scrollbar

import (
	"strings"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

func TestScrollbar(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Scrollbar Suite")
}

var _ = ginkgo.Describe("Scrollbar", func() {
	ginkgo.Describe("View", func() {
		ginkgo.It("returns empty when all items fit", func() {
			sb := New()
			sb.Height = 10
			sb.TotalItems = 5
			sb.VisibleItems = 10
			gomega.Expect(sb.View()).To(gomega.BeEmpty())
		})

		ginkgo.It("returns empty when total items is zero", func() {
			sb := New()
			sb.Height = 10
			sb.TotalItems = 0
			sb.VisibleItems = 5
			gomega.Expect(sb.View()).To(gomega.BeEmpty())
		})

		ginkgo.It("returns empty when height is zero", func() {
			sb := New()
			sb.Height = 0
			sb.TotalItems = 20
			sb.VisibleItems = 5
			gomega.Expect(sb.View()).To(gomega.BeEmpty())
		})

		ginkgo.It("renders correct number of lines", func() {
			sb := New()
			sb.Height = 10
			sb.TotalItems = 50
			sb.VisibleItems = 10
			sb.Offset = 0
			view := sb.View()
			lines := strings.Split(view, "\n")
			gomega.Expect(lines).To(gomega.HaveLen(10))
		})

		ginkgo.It("positions thumb at top when offset is 0", func() {
			sb := New()
			sb.Height = 10
			sb.TotalItems = 50
			sb.VisibleItems = 10
			sb.Offset = 0
			sb.ThumbChar = "X"
			sb.TrackChar = "."
			view := sb.View()
			lines := strings.Split(view, "\n")
			// Thumb should start at line 0
			gomega.Expect(lines[0]).To(gomega.ContainSubstring("X"))
		})

		ginkgo.It("positions thumb at bottom when scrolled to end", func() {
			sb := New()
			sb.Height = 10
			sb.TotalItems = 50
			sb.VisibleItems = 10
			sb.Offset = 40 // scrolled to the end
			sb.ThumbChar = "X"
			sb.TrackChar = "."
			view := sb.View()
			lines := strings.Split(view, "\n")
			// Last line should be thumb
			gomega.Expect(lines[len(lines)-1]).To(gomega.ContainSubstring("X"))
		})
	})
})
