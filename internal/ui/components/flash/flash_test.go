package flash

import (
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

var _ = ginkgo.Describe("Flash Model", func() {

	ginkgo.Describe("New", func() {
		ginkgo.It("starts empty", func() {
			m := New()
			gomega.Expect(m.Empty()).To(gomega.BeTrue())
			gomega.Expect(m.View()).To(gomega.BeEmpty())
		})
	})

	ginkgo.Describe("ShowMsg", func() {
		ginkgo.It("adds a flash message", func() {
			m := New()
			m, _ = m.Update(ShowMsg{ID: "msg1", Text: "hello", Style: StyleInfo})
			gomega.Expect(m.Empty()).To(gomega.BeFalse())
			gomega.Expect(m.items).To(gomega.HaveLen(1))
			gomega.Expect(m.items[0].Text).To(gomega.Equal("hello"))
			gomega.Expect(m.items[0].ID).To(gomega.Equal("msg1"))
		})

		ginkgo.It("replaces existing message with same ID", func() {
			m := New()
			m, _ = m.Update(ShowMsg{ID: "msg1", Text: "first", Style: StyleInfo})
			m, _ = m.Update(ShowMsg{ID: "msg1", Text: "updated", Style: StyleSuccess})
			gomega.Expect(m.items).To(gomega.HaveLen(1))
			gomega.Expect(m.items[0].Text).To(gomega.Equal("updated"))
			gomega.Expect(m.items[0].Style).To(gomega.Equal(StyleSuccess))
		})

		ginkgo.It("stacks multiple messages with different IDs", func() {
			m := New()
			m, _ = m.Update(ShowMsg{ID: "a", Text: "first", Style: StyleInfo})
			m, _ = m.Update(ShowMsg{ID: "b", Text: "second", Style: StyleSuccess})
			m, _ = m.Update(ShowMsg{ID: "c", Text: "third", Style: StyleSpinner})
			gomega.Expect(m.items).To(gomega.HaveLen(3))
		})

		ginkgo.It("returns a tick cmd for expiring messages", func() {
			m := New()
			var cmd interface{}
			m, cmd = m.Update(ShowMsg{ID: "msg1", Text: "temp", Style: StyleInfo, Expires: 1e9}) // 1 second
			gomega.Expect(cmd).ToNot(gomega.BeNil())
		})

		ginkgo.It("returns a spinner tick cmd for spinner style", func() {
			m := New()
			var cmd interface{}
			m, cmd = m.Update(ShowMsg{ID: "spin", Text: "loading", Style: StyleSpinner})
			gomega.Expect(cmd).ToNot(gomega.BeNil())
		})
	})

	ginkgo.Describe("DismissMsg", func() {
		ginkgo.It("removes message by ID", func() {
			m := New()
			m, _ = m.Update(ShowMsg{ID: "msg1", Text: "hello", Style: StyleInfo})
			m, _ = m.Update(ShowMsg{ID: "msg2", Text: "world", Style: StyleInfo})
			gomega.Expect(m.items).To(gomega.HaveLen(2))

			m, _ = m.Update(DismissMsg{ID: "msg1"})
			gomega.Expect(m.items).To(gomega.HaveLen(1))
			gomega.Expect(m.items[0].ID).To(gomega.Equal("msg2"))
		})

		ginkgo.It("is a no-op for unknown ID", func() {
			m := New()
			m, _ = m.Update(ShowMsg{ID: "msg1", Text: "hello", Style: StyleInfo})
			m, _ = m.Update(DismissMsg{ID: "nonexistent"})
			gomega.Expect(m.items).To(gomega.HaveLen(1))
		})
	})

	ginkgo.Describe("expiredMsg", func() {
		ginkgo.It("removes the expired message", func() {
			m := New()
			m, _ = m.Update(ShowMsg{ID: "msg1", Text: "hello", Style: StyleInfo})
			m, _ = m.Update(expiredMsg{ID: "msg1"})
			gomega.Expect(m.Empty()).To(gomega.BeTrue())
		})
	})

	ginkgo.Describe("View", func() {
		ginkgo.It("returns empty string when no messages", func() {
			m := New()
			gomega.Expect(m.View()).To(gomega.BeEmpty())
		})

		ginkgo.It("renders message text", func() {
			m := New()
			m, _ = m.Update(ShowMsg{ID: "msg1", Text: "Operation complete", Style: StyleInfo})
			view := m.View()
			gomega.Expect(view).To(gomega.ContainSubstring("Operation complete"))
		})

		ginkgo.It("renders multiple messages", func() {
			m := New()
			m, _ = m.Update(ShowMsg{ID: "a", Text: "first msg", Style: StyleInfo})
			m, _ = m.Update(ShowMsg{ID: "b", Text: "second msg", Style: StyleInfo})
			view := m.View()
			gomega.Expect(view).To(gomega.ContainSubstring("first msg"))
			gomega.Expect(view).To(gomega.ContainSubstring("second msg"))
		})
	})

	ginkgo.Describe("hasSpinner", func() {
		ginkgo.It("returns false with no spinner items", func() {
			m := New()
			m, _ = m.Update(ShowMsg{ID: "msg1", Text: "info", Style: StyleInfo})
			gomega.Expect(m.hasSpinner()).To(gomega.BeFalse())
		})

		ginkgo.It("returns true with a spinner item", func() {
			m := New()
			m, _ = m.Update(ShowMsg{ID: "spin", Text: "loading", Style: StyleSpinner})
			gomega.Expect(m.hasSpinner()).To(gomega.BeTrue())
		})
	})
})
