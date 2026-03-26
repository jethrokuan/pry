package diffview

import (
	"charm.land/bubbles/v2/textarea"

	"github.com/jethrokuan/pry/internal/review"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

var _ = ginkgo.Describe("Mention autocomplete", func() {
	ginkgo.Describe("filterMentionUsers", func() {
		users := []review.MentionableUser{
			{Login: "alice", Name: "Alice Smith"},
			{Login: "bob", Name: ""},
			{Login: "alicia", Name: "Alicia Keys"},
			{Login: "Charlie", Name: "Charlie Brown"},
			{Login: "ALEX", Name: "Alex Johnson"},
		}

		ginkgo.It("returns all users for empty prefix", func() {
			matches := filterMentionUsers(users, "")
			gomega.Expect(matches).To(gomega.Equal(users))
		})

		ginkgo.It("filters by login prefix case-insensitively", func() {
			matches := filterMentionUsers(users, "al")
			gomega.Expect(matches).To(gomega.HaveLen(3))
			logins := make([]string, len(matches))
			for i, m := range matches {
				logins[i] = m.Login
			}
			gomega.Expect(logins).To(gomega.ConsistOf("alice", "alicia", "ALEX"))
		})

		ginkgo.It("filters by display name prefix", func() {
			matches := filterMentionUsers(users, "char")
			gomega.Expect(matches).To(gomega.HaveLen(1))
			gomega.Expect(matches[0].Login).To(gomega.Equal("Charlie"))
		})

		ginkgo.It("matches login or name, not just login", func() {
			// "Alex" matches ALEX by login prefix and also by name prefix
			matches := filterMentionUsers(users, "alex")
			gomega.Expect(matches).To(gomega.HaveLen(1))
			gomega.Expect(matches[0].Login).To(gomega.Equal("ALEX"))
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
				mentionAll: []review.MentionableUser{
					{Login: "alice", Name: "Alice"},
					{Login: "bob", Name: "Bob"},
				},
			}
			ta := textarea.New()
			ta.SetWidth(80)
			ta.SetHeight(5)
			ta.SetValue("@al")
			e.ta = ta

			e.updateMentionState()

			gomega.Expect(e.mentionActive).To(gomega.BeTrue())
			gomega.Expect(e.mentionMatches).To(gomega.HaveLen(1))
			gomega.Expect(e.mentionMatches[0].Login).To(gomega.Equal("alice"))
			gomega.Expect(e.mentionCursor).To(gomega.Equal(0))
		})

		ginkgo.It("deactivates when no users match", func() {
			e := InlineEditor{
				mentionAll: []review.MentionableUser{
					{Login: "alice", Name: "Alice"},
					{Login: "bob", Name: "Bob"},
				},
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
				mentionAll: []review.MentionableUser{
					{Login: "alice", Name: "Alice"},
					{Login: "alicia", Name: "Alicia"},
				},
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
