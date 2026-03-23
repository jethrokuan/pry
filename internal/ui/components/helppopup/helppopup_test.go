package helppopup

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

var _ = ginkgo.Describe("HelpPopup", func() {

	ginkgo.Describe("FromBinding", func() {
		ginkgo.It("extracts key and desc from binding", func() {
			b := key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "submit"))
			entry := FromBinding(b)
			gomega.Expect(entry.Key).To(gomega.Equal("ctrl+s"))
			gomega.Expect(entry.Desc).To(gomega.Equal("submit"))
		})
	})

	ginkgo.Describe("Bind", func() {
		ginkgo.It("creates a section from bindings", func() {
			b1 := key.NewBinding(key.WithKeys("j"), key.WithHelp("j", "down"))
			b2 := key.NewBinding(key.WithKeys("k"), key.WithHelp("k", "up"))
			section := Bind("Navigation", b1, b2)
			gomega.Expect(section.Title).To(gomega.Equal("Navigation"))
			gomega.Expect(section.Entries).To(gomega.HaveLen(2))
			gomega.Expect(section.Entries[0].Key).To(gomega.Equal("j"))
			gomega.Expect(section.Entries[1].Key).To(gomega.Equal("k"))
		})
	})

	ginkgo.Describe("Render", func() {
		ginkgo.It("renders title", func() {
			sections := []Section{
				{Title: "Navigation", Entries: []Entry{{Key: "j", Desc: "down"}}},
			}
			result := Render(sections, 100)
			gomega.Expect(result).To(gomega.ContainSubstring("Keybindings"))
		})

		ginkgo.It("renders section titles", func() {
			sections := []Section{
				{Title: "Navigation", Entries: []Entry{{Key: "j", Desc: "down"}}},
			}
			result := Render(sections, 100)
			gomega.Expect(result).To(gomega.ContainSubstring("Navigation"))
		})

		ginkgo.It("renders key-desc pairs", func() {
			sections := []Section{
				{Title: "Nav", Entries: []Entry{
					{Key: "j", Desc: "down"},
					{Key: "k", Desc: "up"},
				}},
			}
			result := Render(sections, 100)
			gomega.Expect(result).To(gomega.ContainSubstring("j"))
			gomega.Expect(result).To(gomega.ContainSubstring("k"))
			gomega.Expect(result).To(gomega.ContainSubstring("down"))
			gomega.Expect(result).To(gomega.ContainSubstring("up"))
		})

		ginkgo.It("renders footer text", func() {
			sections := []Section{
				{Title: "Nav", Entries: []Entry{{Key: "q", Desc: "quit"}}},
			}
			result := Render(sections, 100)
			gomega.Expect(result).To(gomega.ContainSubstring("Press any key"))
		})

		ginkgo.It("uses multi-column layout for many sections", func() {
			// Create enough sections to trigger 2-column layout (totalHeight > 16, >= 4 sections)
			sections := make([]Section, 5)
			for i := range sections {
				entries := make([]Entry, 4)
				for j := range entries {
					entries[j] = Entry{Key: "x", Desc: "action"}
				}
				sections[i] = Section{Title: "Section", Entries: entries}
			}
			result := Render(sections, 200)
			// Should render without panic and contain content
			gomega.Expect(result).To(gomega.ContainSubstring("Section"))
		})

		ginkgo.It("falls back to single column for narrow widths", func() {
			sections := make([]Section, 5)
			for i := range sections {
				entries := make([]Entry, 4)
				for j := range entries {
					entries[j] = Entry{Key: "x", Desc: "action"}
				}
				sections[i] = Section{Title: "Section", Entries: entries}
			}
			result := Render(sections, 50) // narrow
			gomega.Expect(result).To(gomega.ContainSubstring("Section"))
		})
	})

	ginkgo.Describe("Overlay", func() {
		ginkgo.It("composites popup over base", func() {
			base := strings.Repeat(strings.Repeat(".", 40)+"\n", 20)
			popup := "POPUP"
			result := Overlay(base, popup, 40, 20)
			gomega.Expect(result).To(gomega.ContainSubstring("POPUP"))
		})

		ginkgo.It("handles popup larger than base gracefully", func() {
			base := "small"
			popup := "a larger popup content"
			// Should not panic
			gomega.Expect(func() {
				Overlay(base, popup, 10, 5)
			}).ToNot(gomega.Panic())
		})
	})
})
