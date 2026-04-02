package diffview

import (
	"charm.land/bubbles/v2/textarea"

	"github.com/jethrokuan/pry/internal/review"
	"github.com/jethrokuan/pry/internal/ui/components/autocomplete"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

var _ = ginkgo.Describe("Mention autocomplete", func() {
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
		makeEditor := func(users []review.MentionableUser, value string) InlineEditor {
			e := InlineEditor{}
			e.SetMentionUsers(users)
			ta := textarea.New()
			ta.SetWidth(80)
			ta.SetHeight(5)
			ta.SetValue(value)
			e.ta = ta
			return e
		}

		ginkgo.It("activates mention when @ is typed with matching users", func() {
			e := makeEditor([]review.MentionableUser{
				{Login: "alice", Name: "Alice"},
				{Login: "bob", Name: "Bob"},
			}, "@al")

			e.updateMentionState()

			gomega.Expect(e.mentionAC.IsActive()).To(gomega.BeTrue())
			gomega.Expect(e.mentionAC.Selected().Value).To(gomega.Equal("alice"))
		})

		ginkgo.It("deactivates when no users match", func() {
			e := makeEditor([]review.MentionableUser{
				{Login: "alice", Name: "Alice"},
				{Login: "bob", Name: "Bob"},
			}, "@zzz")

			e.updateMentionState()

			gomega.Expect(e.mentionAC.IsActive()).To(gomega.BeFalse())
		})

		ginkgo.It("deactivates when mentionAll is empty", func() {
			e := InlineEditor{}
			e.mentionAC = autocomplete.New()
			ta := textarea.New()
			ta.SetWidth(80)
			ta.SetHeight(5)
			ta.SetValue("@alice")
			e.ta = ta

			e.updateMentionState()

			gomega.Expect(e.mentionAC.IsActive()).To(gomega.BeFalse())
		})
	})
})
