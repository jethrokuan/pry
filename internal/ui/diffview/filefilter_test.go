package diffview

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/jkuan/pr-review/internal/codeowners"
	"github.com/jkuan/pr-review/internal/diff"
)

var _ = ginkgo.Describe("FileFilter", func() {
	files := []diff.DiffFile{
		{Path: "src/main.go"},
		{Path: "src/utils/helper.go"},
		{Path: "docs/readme.md"},
		{Path: "tests/main_test.go"},
		{Path: "src/api/handler.go"},
	}

	ginkgo.Describe("regex filter", func() {
		ginkgo.It("includes all files when no filter is set", func() {
			ff := FileFilter{}
			ff.recompute(files)
			gomega.Expect(ff.filteredCount).To(gomega.Equal(5))
			for i := range files {
				gomega.Expect(ff.isIncluded(i)).To(gomega.BeTrue())
			}
		})

		ginkgo.It("filters files by regex pattern", func() {
			ff := FileFilter{}
			ff.setRegex(`\.go$`)
			ff.recompute(files)
			gomega.Expect(ff.filteredCount).To(gomega.Equal(4))
			gomega.Expect(ff.isIncluded(0)).To(gomega.BeTrue())  // main.go
			gomega.Expect(ff.isIncluded(1)).To(gomega.BeTrue())  // helper.go
			gomega.Expect(ff.isIncluded(2)).To(gomega.BeFalse()) // readme.md
			gomega.Expect(ff.isIncluded(3)).To(gomega.BeTrue())  // main_test.go
			gomega.Expect(ff.isIncluded(4)).To(gomega.BeTrue())  // handler.go
		})

		ginkgo.It("filters by path component", func() {
			ff := FileFilter{}
			ff.setRegex(`src/`)
			ff.recompute(files)
			gomega.Expect(ff.filteredCount).To(gomega.Equal(3))
			gomega.Expect(ff.isIncluded(0)).To(gomega.BeTrue())  // src/main.go
			gomega.Expect(ff.isIncluded(1)).To(gomega.BeTrue())  // src/utils/helper.go
			gomega.Expect(ff.isIncluded(2)).To(gomega.BeFalse()) // docs/readme.md
			gomega.Expect(ff.isIncluded(3)).To(gomega.BeFalse()) // tests/main_test.go
			gomega.Expect(ff.isIncluded(4)).To(gomega.BeTrue())  // src/api/handler.go
		})

		ginkgo.It("handles invalid regex gracefully", func() {
			ff := FileFilter{}
			ff.setRegex(`[invalid`)
			ff.recompute(files)
			// Invalid regex = no filter applied
			gomega.Expect(ff.filteredCount).To(gomega.Equal(5))
		})

		ginkgo.It("clearing regex re-includes all files", func() {
			ff := FileFilter{}
			ff.setRegex(`\.go$`)
			ff.recompute(files)
			gomega.Expect(ff.filteredCount).To(gomega.Equal(4))

			ff.setRegex("")
			ff.recompute(files)
			gomega.Expect(ff.filteredCount).To(gomega.Equal(5))
		})
	})

	ginkgo.Describe("isActive", func() {
		ginkgo.It("returns false when no filters set", func() {
			ff := FileFilter{}
			gomega.Expect(ff.isActive()).To(gomega.BeFalse())
		})

		ginkgo.It("returns true when regex is set", func() {
			ff := FileFilter{}
			ff.setRegex("test")
			gomega.Expect(ff.isActive()).To(gomega.BeTrue())
		})

		ginkgo.It("returns true when owner filter is enabled", func() {
			ff := FileFilter{ownerEnabled: true, ownerIdentities: []string{"@team"}}
			gomega.Expect(ff.isActive()).To(gomega.BeTrue())
		})
	})

	ginkgo.Describe("toggleOwner", func() {
		ginkgo.It("returns not-available message when ownerIdentities is empty", func() {
			ff := FileFilter{}
			label := ff.toggleOwner()
			gomega.Expect(label).To(gomega.ContainSubstring("not available"))
			gomega.Expect(ff.ownerEnabled).To(gomega.BeFalse())
		})

		ginkgo.It("toggles off when currently enabled", func() {
			ff := FileFilter{ownerEnabled: true, ownerIdentities: []string{"@team"}, codeowners: &codeowners.Codeowners{}}
			label := ff.toggleOwner()
			gomega.Expect(label).To(gomega.Equal("off"))
			gomega.Expect(ff.ownerEnabled).To(gomega.BeFalse())
		})

		ginkgo.It("toggles on when codeowners is available", func() {
			ff := FileFilter{ownerIdentities: []string{"@team"}, codeowners: &codeowners.Codeowners{}}
			label := ff.toggleOwner()
			gomega.Expect(label).To(gomega.Equal("@team"))
			gomega.Expect(ff.ownerEnabled).To(gomega.BeTrue())
		})

		ginkgo.It("returns CODEOWNERS-not-found when codeowners is nil", func() {
			ff := FileFilter{ownerIdentities: []string{"@team"}}
			// Note: codeowners is nil and loadCodeowners will fail in test env
			label := ff.toggleOwner()
			gomega.Expect(label).To(gomega.ContainSubstring("CODEOWNERS"))
			gomega.Expect(ff.ownerEnabled).To(gomega.BeFalse())
		})
	})

	ginkgo.Describe("clearAll", func() {
		ginkgo.It("clears all filters", func() {
			ff := FileFilter{}
			ff.setRegex("test")
			ff.ownerEnabled = true
			ff.ownerIdentities = []string{"@team"}
			ff.clearAll()
			gomega.Expect(ff.isActive()).To(gomega.BeFalse())
			gomega.Expect(ff.regexPattern).To(gomega.BeEmpty())
			gomega.Expect(ff.regexCompiled).To(gomega.BeNil())
			gomega.Expect(ff.ownerEnabled).To(gomega.BeFalse())
		})
	})

	ginkgo.Describe("statusText", func() {
		ginkgo.It("returns empty when no filters active", func() {
			ff := FileFilter{}
			gomega.Expect(ff.statusText()).To(gomega.BeEmpty())
		})

		ginkgo.It("shows regex pattern", func() {
			ff := FileFilter{}
			ff.setRegex(`\.go$`)
			gomega.Expect(ff.statusText()).To(gomega.Equal(`regex:\.go$`))
		})

		ginkgo.It("shows owner pattern", func() {
			ff := FileFilter{ownerEnabled: true, ownerIdentities: []string{"@org/team"}}
			gomega.Expect(ff.statusText()).To(gomega.Equal("owner:@org/team"))
		})

		ginkgo.It("shows both when stacked", func() {
			ff := FileFilter{ownerEnabled: true, ownerIdentities: []string{"@org/team"}}
			ff.setRegex("src/")
			gomega.Expect(ff.statusText()).To(gomega.Equal("regex:src/ + owner:@org/team"))
		})
	})

	ginkgo.Describe("matchesAll with owner filter", func() {
		ginkgo.It("excludes all files when codeowners is nil", func() {
			ff := FileFilter{ownerEnabled: true, ownerIdentities: []string{"@team"}}
			// codeowners is nil — cannot determine ownership
			gomega.Expect(ff.matchesAll("src/main.go")).To(gomega.BeFalse())
		})

		ginkgo.It("filters by codeowners when available", func() {
			co, _ := makeTempCodeowners("*.go @go-team\ndocs/ @docs-team\n")
			ff := FileFilter{ownerEnabled: true, ownerIdentities: []string{"@go-team"}, codeowners: co}
			gomega.Expect(ff.matchesAll("src/main.go")).To(gomega.BeTrue())
			gomega.Expect(ff.matchesAll("docs/readme.md")).To(gomega.BeFalse())
		})

		ginkgo.It("matches when any owner identity matches", func() {
			co, _ := makeTempCodeowners("*.go @go-team\ndocs/ @docs-team\n")
			ff := FileFilter{ownerEnabled: true, ownerIdentities: []string{"@me", "@docs-team"}, codeowners: co}
			gomega.Expect(ff.matchesAll("docs/readme.md")).To(gomega.BeTrue())
			gomega.Expect(ff.matchesAll("src/main.go")).To(gomega.BeFalse()) // owned by @go-team, not @me or @docs-team
		})
	})

	ginkgo.Describe("buildTree with filter", func() {
		ginkgo.It("builds tree with only included files", func() {
			included := map[int]bool{0: true, 4: true} // main.go, handler.go
			tree := buildTree(files, included)
			indices := collectFileIndices(tree)
			gomega.Expect(indices).To(gomega.ConsistOf(0, 4))
		})

		ginkgo.It("builds full tree when included is nil", func() {
			tree := buildTree(files, nil)
			indices := collectFileIndices(tree)
			gomega.Expect(indices).To(gomega.HaveLen(5))
		})
	})
})

func TestFileFilter(t *testing.T) {
	// This is handled by the diffview_suite_test.go
	// Just making sure it gets picked up by the suite runner
}

// makeTempCodeowners creates a Codeowners from a content string for testing.
func makeTempCodeowners(content string) (*codeowners.Codeowners, error) {
	tmpDir := os.TempDir()
	path := filepath.Join(tmpDir, "CODEOWNERS_test")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return nil, err
	}
	defer os.Remove(path)
	return codeowners.Parse(path)
}
