package sidebar

import (
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

var _ = ginkgo.Describe("Sidebar Model", func() {

	ginkgo.Describe("New", func() {
		ginkgo.It("starts uninitialized", func() {
			m := New()
			gomega.Expect(m.ready).To(gomega.BeFalse())
			gomega.Expect(m.width).To(gomega.Equal(0))
			gomega.Expect(m.height).To(gomega.Equal(0))
		})
	})

	ginkgo.Describe("SetSize", func() {
		ginkgo.It("initializes viewport on first call", func() {
			m := New()
			m.SetSize(60, 30)
			gomega.Expect(m.ready).To(gomega.BeTrue())
			gomega.Expect(m.width).To(gomega.Equal(60))
			gomega.Expect(m.height).To(gomega.Equal(30))
		})

		ginkgo.It("updates dimensions on subsequent calls", func() {
			m := New()
			m.SetSize(60, 30)
			m.SetSize(80, 40)
			gomega.Expect(m.width).To(gomega.Equal(80))
			gomega.Expect(m.height).To(gomega.Equal(40))
		})

		ginkgo.It("flushes pending content when initialized", func() {
			m := New()
			m.SetContent("buffered content")
			gomega.Expect(m.pendingContent).To(gomega.Equal("buffered content"))
			m.SetSize(60, 30)
			gomega.Expect(m.pendingContent).To(gomega.BeEmpty())
		})
	})

	ginkgo.Describe("SetContent", func() {
		ginkgo.It("buffers content before initialization", func() {
			m := New()
			m.SetContent("early content")
			gomega.Expect(m.pendingContent).To(gomega.Equal("early content"))
		})

		ginkgo.It("sets viewport content after initialization", func() {
			m := New()
			m.SetSize(60, 30)
			m.SetContent("viewport content")
			gomega.Expect(m.pendingContent).To(gomega.BeEmpty())
		})
	})

	ginkgo.Describe("View", func() {
		ginkgo.It("returns empty string when width is 0", func() {
			m := New()
			gomega.Expect(m.View()).To(gomega.BeEmpty())
		})

		ginkgo.It("renders content after initialization", func() {
			m := New()
			m.SetSize(60, 10)
			m.SetContent("sidebar content here")
			view := m.View()
			gomega.Expect(view).To(gomega.ContainSubstring("sidebar content here"))
		})
	})

	ginkgo.Describe("Scrolling", func() {
		ginkgo.It("scrolls down without panic", func() {
			m := New()
			m.SetSize(60, 5)
			m.SetContent("line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10")
			gomega.Expect(func() { m.ScrollDown(3) }).ToNot(gomega.Panic())
		})

		ginkgo.It("scrolls up without panic", func() {
			m := New()
			m.SetSize(60, 5)
			m.SetContent("line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10")
			m.ScrollDown(5)
			gomega.Expect(func() { m.ScrollUp(3) }).ToNot(gomega.Panic())
		})
	})
})
