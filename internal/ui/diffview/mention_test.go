package diffview

import (
	"charm.land/bubbles/v2/textarea"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

var _ = ginkgo.Describe("Mention autocomplete", func() {
	ginkgo.Describe("filterMentionUsers", func() {
		users := []string{"alice", "bob", "alicia", "Charlie", "ALEX"}

		ginkgo.It("returns all users for empty prefix", func() {
			matches := filterMentionUsers(users, "")
			gomega.Expect(matches).To(gomega.Equal(users))
		})

		ginkgo.It("filters by prefix case-insensitively", func() {
			matches := filterMentionUsers(users, "al")
			gomega.Expect(matches).To(gomega.ConsistOf("alice", "alicia", "ALEX"))
		})

		ginkgo.It("returns nil for no matches", func() {
			matches := filterMentionUsers(users, "zz")
			gomega.Expect(matches).To(gomega.BeNil())
		})

		ginkgo.It("returns nil for nil users", func() {
			matches := filterMentionUsers(nil, "a")
			gomega.Expect(matches).To(gomega.BeNil())
		})
	})

	ginkgo.Describe("mentionTrigger", func() {
		newTA := func(value string) textarea.Model {
			ta := textarea.New()
			ta.SetWidth(80)
			ta.SetHeight(5)
			if value != "" {
				ta.SetValue(value)
			}
			return ta
		}

		ginkgo.It("returns -1 for empty textarea", func() {
			ta := newTA("")
			_, idx := mentionTrigger(ta)
			gomega.Expect(idx).To(gomega.Equal(-1))
		})

		ginkgo.It("detects @ at start of text", func() {
			ta := newTA("@ali")
			prefix, idx := mentionTrigger(ta)
			gomega.Expect(idx).To(gomega.Equal(0))
			gomega.Expect(prefix).To(gomega.Equal("ali"))
		})

		ginkgo.It("detects @ after space", func() {
			ta := newTA("hello @bob")
			prefix, idx := mentionTrigger(ta)
			gomega.Expect(idx).To(gomega.Equal(6))
			gomega.Expect(prefix).To(gomega.Equal("bob"))
		})

		ginkgo.It("returns -1 when no @ is present", func() {
			ta := newTA("hello world")
			_, idx := mentionTrigger(ta)
			gomega.Expect(idx).To(gomega.Equal(-1))
		})

		ginkgo.It("returns -1 for @ in middle of word", func() {
			ta := newTA("email@example")
			_, idx := mentionTrigger(ta)
			gomega.Expect(idx).To(gomega.Equal(-1))
		})
	})

	ginkgo.Describe("updateMentionState", func() {
		ginkgo.It("activates mention when @ is typed with matching users", func() {
			e := InlineEditor{
				mentionAll: []string{"alice", "bob"},
			}
			ta := textarea.New()
			ta.SetWidth(80)
			ta.SetHeight(5)
			ta.SetValue("@al")
			e.ta = ta

			e.updateMentionState()

			gomega.Expect(e.mentionActive).To(gomega.BeTrue())
			gomega.Expect(e.mentionMatches).To(gomega.ConsistOf("alice"))
			gomega.Expect(e.mentionCursor).To(gomega.Equal(0))
		})

		ginkgo.It("deactivates when no users match", func() {
			e := InlineEditor{
				mentionAll:    []string{"alice", "bob"},
				mentionActive: true,
			}
			ta := textarea.New()
			ta.SetWidth(80)
			ta.SetHeight(5)
			ta.SetValue("@zzz")
			e.ta = ta

			e.updateMentionState()

			gomega.Expect(e.mentionActive).To(gomega.BeFalse())
		})

		ginkgo.It("deactivates when mentionAll is empty", func() {
			e := InlineEditor{
				mentionActive: true,
			}
			ta := textarea.New()
			ta.SetWidth(80)
			ta.SetHeight(5)
			ta.SetValue("@alice")
			e.ta = ta

			e.updateMentionState()

			gomega.Expect(e.mentionActive).To(gomega.BeFalse())
		})

		ginkgo.It("resets cursor when matches shrink", func() {
			e := InlineEditor{
				mentionAll:    []string{"alice", "alicia"},
				mentionCursor: 1,
			}
			ta := textarea.New()
			ta.SetWidth(80)
			ta.SetHeight(5)
			ta.SetValue("@alice")
			e.ta = ta

			e.updateMentionState()

			gomega.Expect(e.mentionActive).To(gomega.BeTrue())
			gomega.Expect(e.mentionMatches).To(gomega.HaveLen(1))
			gomega.Expect(e.mentionCursor).To(gomega.Equal(0))
		})
	})
})
