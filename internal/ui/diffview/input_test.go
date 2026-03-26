package diffview

import (
	tea "charm.land/bubbletea/v2"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/jethrokuan/pry/internal/diff"
	"github.com/jethrokuan/pry/internal/review"
	"github.com/jethrokuan/pry/internal/ui/components/flash"
)

// testFiles returns a small set of DiffFiles for testing navigation and input handling.
func testFiles() []diff.DiffFile {
	return []diff.DiffFile{
		{
			Path:   "src/main.go",
			Status: diff.StatusModified,
			Hunks: []diff.Hunk{
				{
					OldStart: 1, OldLines: 3, NewStart: 1, NewLines: 4,
					Header: "@@ -1,3 +1,4 @@",
					Lines: []diff.DiffLine{
						{Type: diff.LineContext, OldNum: 1, NewNum: 1, Content: " package main"},
						{Type: diff.LineAddition, OldNum: 0, NewNum: 2, Content: "+import \"fmt\""},
						{Type: diff.LineContext, OldNum: 2, NewNum: 3, Content: " func main() {"},
						{Type: diff.LineContext, OldNum: 3, NewNum: 4, Content: " }"},
					},
				},
				{
					OldStart: 10, OldLines: 2, NewStart: 11, NewLines: 3,
					Header: "@@ -10,2 +11,3 @@",
					Lines: []diff.DiffLine{
						{Type: diff.LineContext, OldNum: 10, NewNum: 11, Content: " // end"},
						{Type: diff.LineAddition, OldNum: 0, NewNum: 12, Content: "+// added"},
						{Type: diff.LineContext, OldNum: 11, NewNum: 13, Content: " "},
					},
				},
			},
		},
		{
			Path:   "src/util.go",
			Status: diff.StatusModified,
			Hunks: []diff.Hunk{
				{
					OldStart: 1, OldLines: 2, NewStart: 1, NewLines: 2,
					Header: "@@ -1,2 +1,2 @@",
					Lines: []diff.DiffLine{
						{Type: diff.LineDeletion, OldNum: 1, NewNum: 0, Content: "-old line"},
						{Type: diff.LineAddition, OldNum: 0, NewNum: 1, Content: "+new line"},
					},
				},
			},
		},
		{
			Path:   "README.md",
			Status: diff.StatusModified,
			Hunks: []diff.Hunk{
				{
					OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 2,
					Header: "@@ -1,1 +1,2 @@",
					Lines: []diff.DiffLine{
						{Type: diff.LineContext, OldNum: 1, NewNum: 1, Content: " # README"},
						{Type: diff.LineAddition, OldNum: 0, NewNum: 2, Content: "+Description"},
					},
				},
			},
		},
	}
}

// newInputTestModel creates a Model pre-loaded with test files and initialized viewports.
// The model starts in diff focus on the first file.
func newInputTestModel() Model {
	pr := &review.PullRequest{
		Number: 1,
		Title:  "Test PR",
		Author: "test",
		Base:   "main",
	}
	pr.StartReview()
	m := New(nil, pr)
	m.loading = false

	// Inject test files
	m.files = testFiles()
	m.filter.recompute(m.files)
	m.nav.cachedTree = buildTree(m.files, m.filter.includedFiles)
	m.nav.rebuildTreeRows()
	m.nav.buildDiffLines(m.files)

	// Initialize viewports with window size
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return m
}

var _ = ginkgo.Describe("Input Handling", func() {

	ginkgo.Describe("Tree focus navigation", func() {
		ginkgo.It("should move tree cursor down with j/down key", func() {
			m := newInputTestModel()
			m.nav.focus = FocusFileTree
			m.nav.treeCursor = 0

			m, _ = m.Update(tea.KeyPressMsg{Code: 'j'})
			gomega.Expect(m.nav.treeCursor).To(gomega.Equal(1))
		})

		ginkgo.It("should move tree cursor up with k/up key", func() {
			m := newInputTestModel()
			m.nav.focus = FocusFileTree
			m.nav.treeCursor = 2

			m, _ = m.Update(tea.KeyPressMsg{Code: 'k'})
			gomega.Expect(m.nav.treeCursor).To(gomega.Equal(1))
		})

		ginkgo.It("should not move tree cursor below last row", func() {
			m := newInputTestModel()
			m.nav.focus = FocusFileTree
			lastIdx := len(m.nav.treeRows) - 1
			m.nav.treeCursor = lastIdx

			m, _ = m.Update(tea.KeyPressMsg{Code: 'j'})
			gomega.Expect(m.nav.treeCursor).To(gomega.Equal(lastIdx))
		})

		ginkgo.It("should not move tree cursor above first row", func() {
			m := newInputTestModel()
			m.nav.focus = FocusFileTree
			m.nav.treeCursor = 0

			m, _ = m.Update(tea.KeyPressMsg{Code: 'k'})
			gomega.Expect(m.nav.treeCursor).To(gomega.Equal(0))
		})

		ginkgo.It("should switch to diff focus on Enter when on a file row", func() {
			m := newInputTestModel()
			m.nav.focus = FocusFileTree
			// Find a file row (not a directory)
			for i, row := range m.nav.treeRows {
				if row.node.fileIdx >= 0 {
					m.nav.treeCursor = i
					break
				}
			}

			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			gomega.Expect(m.nav.focus).To(gomega.Equal(FocusDiff))
		})

		ginkgo.It("should activate search filter with / key in tree focus", func() {
			m := newInputTestModel()
			m.nav.focus = FocusFileTree

			m, _ = m.Update(tea.KeyPressMsg{Code: '/'})
			gomega.Expect(m.search.filterActive).To(gomega.BeTrue())
		})

		ginkgo.It("should show help with ? key", func() {
			m := newInputTestModel()
			m.nav.focus = FocusFileTree

			m, _ = m.Update(tea.KeyPressMsg{Code: '?'})
			gomega.Expect(m.showHelp).To(gomega.BeTrue())
		})
	})

	ginkgo.Describe("Diff focus navigation", func() {
		ginkgo.It("should move diff cursor down with j key", func() {
			m := newInputTestModel()
			m.nav.focus = FocusDiff
			m.nav.cursor.LineIdx = 0

			m, _ = m.Update(tea.KeyPressMsg{Code: 'j'})
			gomega.Expect(m.nav.cursor.LineIdx).To(gomega.Equal(1))
		})

		ginkgo.It("should move diff cursor up with k key", func() {
			m := newInputTestModel()
			m.nav.focus = FocusDiff
			m.nav.cursor.LineIdx = 2

			m, _ = m.Update(tea.KeyPressMsg{Code: 'k'})
			gomega.Expect(m.nav.cursor.LineIdx).To(gomega.Equal(1))
		})

		ginkgo.It("should not move diff cursor below last line", func() {
			m := newInputTestModel()
			m.nav.focus = FocusDiff
			lastIdx := len(m.nav.diffLines) - 1
			m.nav.cursor.LineIdx = lastIdx

			m, _ = m.Update(tea.KeyPressMsg{Code: 'j'})
			gomega.Expect(m.nav.cursor.LineIdx).To(gomega.Equal(lastIdx))
		})

		ginkgo.It("should not move diff cursor above first line", func() {
			m := newInputTestModel()
			m.nav.focus = FocusDiff
			m.nav.cursor.LineIdx = 0

			m, _ = m.Update(tea.KeyPressMsg{Code: 'k'})
			gomega.Expect(m.nav.cursor.LineIdx).To(gomega.Equal(0))
		})

		ginkgo.It("should activate search with / key", func() {
			m := newInputTestModel()
			m.nav.focus = FocusDiff

			m, _ = m.Update(tea.KeyPressMsg{Code: '/'})
			gomega.Expect(m.search.active).To(gomega.BeTrue())
		})

		ginkgo.It("should activate file filter with ctrl+p", func() {
			m := newInputTestModel()
			m.nav.focus = FocusDiff

			m, _ = m.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
			gomega.Expect(m.search.filterActive).To(gomega.BeTrue())
		})

		ginkgo.It("should show help with ? key", func() {
			m := newInputTestModel()
			m.nav.focus = FocusDiff

			m, _ = m.Update(tea.KeyPressMsg{Code: '?'})
			gomega.Expect(m.showHelp).To(gomega.BeTrue())
		})
	})

	ginkgo.Describe("Focus toggling with t key", func() {
		ginkgo.It("should switch from diff to tree focus", func() {
			m := newInputTestModel()
			m.nav.focus = FocusDiff
			m.nav.showTree = true

			m, _ = m.Update(tea.KeyPressMsg{Code: 't'})
			gomega.Expect(m.nav.focus).To(gomega.Equal(FocusFileTree))
		})

		ginkgo.It("should switch from tree focus back to diff", func() {
			m := newInputTestModel()
			m.nav.focus = FocusFileTree
			m.nav.showTree = true

			m, _ = m.Update(tea.KeyPressMsg{Code: 't'})
			gomega.Expect(m.nav.focus).To(gomega.Equal(FocusDiff))
		})

		ginkgo.It("should show tree and focus it when tree is hidden", func() {
			m := newInputTestModel()
			m.nav.showTree = false
			m.nav.focus = FocusDiff

			m, _ = m.Update(tea.KeyPressMsg{Code: 't'})
			gomega.Expect(m.nav.showTree).To(gomega.BeTrue())
			gomega.Expect(m.nav.focus).To(gomega.Equal(FocusFileTree))
		})
	})

	ginkgo.Describe("File navigation with f/F keys", func() {
		ginkgo.It("should navigate to next file with f key", func() {
			m := newInputTestModel()
			m.nav.focus = FocusDiff
			m.nav.cursor.FileIdx = 0

			m, _ = m.Update(tea.KeyPressMsg{Code: 'f'})
			gomega.Expect(m.nav.cursor.FileIdx).To(gomega.Equal(1))
			gomega.Expect(m.nav.cursor.LineIdx).To(gomega.Equal(0), "diff cursor should reset to 0")
		})

		ginkgo.It("should navigate to previous file with F key", func() {
			m := newInputTestModel()
			m.nav.focus = FocusDiff
			m.nav.cursor.FileIdx = 1
			m.nav.buildDiffLines(m.files)

			m, _ = m.Update(tea.KeyPressMsg{Code: 'F'})
			gomega.Expect(m.nav.cursor.FileIdx).To(gomega.Equal(0))
		})

		ginkgo.It("should wrap around at last file", func() {
			m := newInputTestModel()
			m.nav.focus = FocusDiff
			m.nav.cursor.FileIdx = len(m.files) - 1
			m.nav.buildDiffLines(m.files)

			m, _ = m.Update(tea.KeyPressMsg{Code: 'f'})
			gomega.Expect(m.nav.cursor.FileIdx).To(gomega.Equal(0))
		})
	})

	ginkgo.Describe("Hunk navigation with h/H keys", func() {
		ginkgo.It("should navigate to next hunk with h key", func() {
			m := newInputTestModel()
			m.nav.focus = FocusDiff
			m.nav.cursor.LineIdx = 0
			// First file has two hunks. Cursor starts in hunk 0.

			m, _ = m.Update(tea.KeyPressMsg{Code: 'h'})
			gomega.Expect(m.nav.diffLines[m.nav.cursor.LineIdx].hunkIdx).To(gomega.Equal(1))
		})

		ginkgo.It("should navigate to previous hunk with H key", func() {
			m := newInputTestModel()
			m.nav.focus = FocusDiff
			// Start at the second hunk
			for i, dl := range m.nav.diffLines {
				if dl.hunkIdx == 1 {
					m.nav.cursor.LineIdx = i + 1 // somewhere inside hunk 1
					break
				}
			}

			m, _ = m.Update(tea.KeyPressMsg{Code: 'H'})
			// Should go to start of hunk 1
			gomega.Expect(m.nav.diffLines[m.nav.cursor.LineIdx].hunkIdx).To(gomega.Equal(1))
			// Should be at the first line of hunk 1
			isFirstOfHunk := m.nav.cursor.LineIdx == 0 || m.nav.diffLines[m.nav.cursor.LineIdx-1].hunkIdx != 1
			gomega.Expect(isFirstOfHunk).To(gomega.BeTrue())
		})
	})

	ginkgo.Describe("Visual selection", func() {
		ginkgo.It("should extend visual selection when moving down", func() {
			m := newInputTestModel()
			m.nav.focus = FocusDiff
			m.nav.cursor.LineIdx = 1
			m.nav.visualMode = true
			m.nav.visualStart = CursorTarget{Kind: CursorLine, LineIdx: 1}
			m.nav.visualEnd = CursorTarget{Kind: CursorLine, LineIdx: 1}

			// Move down in visual mode — should extend selection
			m, _ = m.Update(tea.KeyPressMsg{Code: 'j'})
			gomega.Expect(m.nav.visualEnd.LineIdx).To(gomega.Equal(2))
			gomega.Expect(m.nav.visualStart.LineIdx).To(gomega.Equal(1))
		})

		ginkgo.It("should extend visual selection when moving up", func() {
			m := newInputTestModel()
			m.nav.focus = FocusDiff
			m.nav.cursor.LineIdx = 3
			m.nav.visualMode = true
			m.nav.visualStart = CursorTarget{Kind: CursorLine, LineIdx: 3}
			m.nav.visualEnd = CursorTarget{Kind: CursorLine, LineIdx: 3}

			m, _ = m.Update(tea.KeyPressMsg{Code: 'k'})
			gomega.Expect(m.nav.visualEnd.LineIdx).To(gomega.Equal(2))
			gomega.Expect(m.nav.visualStart.LineIdx).To(gomega.Equal(3))
		})

		ginkgo.It("should exit visual mode on esc", func() {
			m := newInputTestModel()
			m.nav.focus = FocusDiff
			m.nav.visualMode = true
			m.nav.visualStart = CursorTarget{Kind: CursorLine, LineIdx: 1}
			m.nav.visualEnd = CursorTarget{Kind: CursorLine, LineIdx: 3}

			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
			gomega.Expect(m.nav.visualMode).To(gomega.BeFalse())
		})
	})

	ginkgo.Describe("Search mode", func() {
		ginkgo.It("should enter search mode with / and accept input", func() {
			m := newInputTestModel()
			m.nav.focus = FocusDiff

			// Activate search
			m, _ = m.Update(tea.KeyPressMsg{Code: '/'})
			gomega.Expect(m.search.active).To(gomega.BeTrue())

			// Type search text
			m, _ = m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
			m, _ = m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
			m, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
			gomega.Expect(m.search.input).To(gomega.Equal("fmt"))

			// Submit search
			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			gomega.Expect(m.search.active).To(gomega.BeFalse())
			gomega.Expect(m.search.query).To(gomega.Equal("fmt"))
		})

		ginkgo.It("should cancel search with esc", func() {
			m := newInputTestModel()
			m.nav.focus = FocusDiff

			m, _ = m.Update(tea.KeyPressMsg{Code: '/'})
			m, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})

			gomega.Expect(m.search.active).To(gomega.BeFalse())
			gomega.Expect(m.search.input).To(gomega.Equal(""))
		})

		ginkgo.It("should handle backspace in search", func() {
			m := newInputTestModel()
			m.search.active = true
			m.search.input = "abc"

			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
			gomega.Expect(m.search.input).To(gomega.Equal("ab"))
		})
	})

	ginkgo.Describe("Goto line mode", func() {
		ginkgo.It("should enter goto mode and jump to line", func() {
			m := newInputTestModel()
			m.search.gotoActive = true
			m.search.gotoInput = ""

			// Type line number "3"
			m, _ = m.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
			gomega.Expect(m.search.gotoInput).To(gomega.Equal("3"))

			// Submit
			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			gomega.Expect(m.search.gotoActive).To(gomega.BeFalse())
			// Cursor should be on a line with newLine or oldLine == 3
			if m.nav.cursor.LineIdx < len(m.nav.diffLines) {
				dl := m.nav.diffLines[m.nav.cursor.LineIdx]
				gomega.Expect(dl.newLine == 3 || dl.oldLine == 3).To(gomega.BeTrue())
			}
		})

		ginkgo.It("should cancel goto with esc", func() {
			m := newInputTestModel()
			m.search.gotoActive = true
			m.search.gotoInput = "42"

			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
			gomega.Expect(m.search.gotoActive).To(gomega.BeFalse())
			gomega.Expect(m.search.gotoInput).To(gomega.Equal(""))
		})

		ginkgo.It("should only accept digit input", func() {
			m := newInputTestModel()
			m.search.gotoActive = true
			m.search.gotoInput = ""

			// Type a letter — should be ignored
			m, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
			gomega.Expect(m.search.gotoInput).To(gomega.Equal(""))

			// Type a digit
			m, _ = m.Update(tea.KeyPressMsg{Code: '5', Text: "5"})
			gomega.Expect(m.search.gotoInput).To(gomega.Equal("5"))
		})
	})

	ginkgo.Describe("Help overlay", func() {
		ginkgo.It("should dismiss help on any key", func() {
			m := newInputTestModel()
			m.showHelp = true

			m, _ = m.Update(tea.KeyPressMsg{Code: 'q'})
			gomega.Expect(m.showHelp).To(gomega.BeFalse())
		})
	})

	ginkgo.Describe("PR info popup", func() {
		ginkgo.It("should open with i key and close with esc", func() {
			m := newInputTestModel()
			m.nav.focus = FocusDiff

			m, _ = m.Update(tea.KeyPressMsg{Code: 'i'})
			gomega.Expect(m.prInfoActive).To(gomega.BeTrue())

			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
			gomega.Expect(m.prInfoActive).To(gomega.BeFalse())
		})

		ginkgo.It("should close with i key again", func() {
			m := newInputTestModel()
			m.nav.focus = FocusDiff

			m, _ = m.Update(tea.KeyPressMsg{Code: 'i'})
			gomega.Expect(m.prInfoActive).To(gomega.BeTrue())

			m, _ = m.Update(tea.KeyPressMsg{Code: 'i'})
			gomega.Expect(m.prInfoActive).To(gomega.BeFalse())
		})
	})

	ginkgo.Describe("Narrow prefix mode (T)", func() {
		ginkgo.It("should activate narrow prefix mode with T key", func() {
			m := newInputTestModel()
			m.nav.focus = FocusDiff

			m, cmd := m.Update(tea.KeyPressMsg{Code: 'T'})
			gomega.Expect(m.narrowPrefixActive).To(gomega.BeTrue())
			msg := cmd()
			fmsg, ok := msg.(flash.ShowMsg)
			gomega.Expect(ok).To(gomega.BeTrue())
			gomega.Expect(fmsg.Text).To(gomega.ContainSubstring("[o]wner"))
		})

		ginkgo.It("should open regex filter with T then f", func() {
			m := newInputTestModel()
			m.nav.focus = FocusDiff

			m, _ = m.Update(tea.KeyPressMsg{Code: 'T'})
			m, _ = m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
			gomega.Expect(m.narrowPrefixActive).To(gomega.BeFalse())
			gomega.Expect(m.filter.regexActive).To(gomega.BeTrue())
		})

		ginkgo.It("should dismiss on unknown key after T", func() {
			m := newInputTestModel()
			m.nav.focus = FocusDiff

			m, _ = m.Update(tea.KeyPressMsg{Code: 'T'})
			m, _ = m.Update(tea.KeyPressMsg{Code: 'z', Text: "z"})
			gomega.Expect(m.narrowPrefixActive).To(gomega.BeFalse())
		})
	})

	ginkgo.Describe("Narrow regex filter mode", func() {
		ginkgo.It("should accept input and apply on enter", func() {
			m := newInputTestModel()
			m.filter.regexActive = true
			m.filter.regexInput = ""

			m, _ = m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
			m, _ = m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
			m, _ = m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
			gomega.Expect(m.filter.regexInput).To(gomega.Equal("src"))

			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			gomega.Expect(m.filter.regexActive).To(gomega.BeFalse())
			gomega.Expect(m.filter.regexPattern).To(gomega.Equal("src"))
		})

		ginkgo.It("should cancel on esc", func() {
			m := newInputTestModel()
			m.filter.regexActive = true
			m.filter.regexInput = "abc"

			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
			gomega.Expect(m.filter.regexActive).To(gomega.BeFalse())
		})
	})

	ginkgo.Describe("File filter mode", func() {
		ginkgo.It("should filter files by input and select on enter", func() {
			m := newInputTestModel()
			m.activateFileFilter()

			// Type "util"
			m, _ = m.Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
			m, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
			m, _ = m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
			m, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})

			// Should have filtered to just src/util.go
			gomega.Expect(len(m.search.filterFiles)).To(gomega.Equal(1))
			gomega.Expect(m.files[m.search.filterFiles[0]].Path).To(gomega.Equal("src/util.go"))

			// Select it
			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			gomega.Expect(m.search.filterActive).To(gomega.BeFalse())
			gomega.Expect(m.nav.cursor.FileIdx).To(gomega.Equal(1)) // index of src/util.go
		})

		ginkgo.It("should navigate filter list with up/down", func() {
			m := newInputTestModel()
			m.activateFileFilter()

			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
			gomega.Expect(m.search.filterCursor).To(gomega.Equal(1))

			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
			gomega.Expect(m.search.filterCursor).To(gomega.Equal(0))
		})

		ginkgo.It("should cancel with esc", func() {
			m := newInputTestModel()
			m.activateFileFilter()

			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
			gomega.Expect(m.search.filterActive).To(gomega.BeFalse())
		})
	})

	ginkgo.Describe("Hunk folding with tab", func() {
		ginkgo.It("should collapse a hunk in diff focus", func() {
			m := newInputTestModel()
			m.nav.focus = FocusDiff
			m.nav.cursor.LineIdx = 0
			linesBefore := len(m.nav.diffLines)

			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})

			// After collapsing hunk 0, there should be fewer lines (hunk replaced by placeholder)
			gomega.Expect(len(m.nav.diffLines)).To(gomega.BeNumerically("<", linesBefore))
			// The first hunk should be collapsed
			hk := hunkKey(m.files[0].Path, 0)
			gomega.Expect(m.nav.collapsedHunks[hk]).To(gomega.BeTrue())
		})

		ginkgo.It("should expand a collapsed hunk", func() {
			m := newInputTestModel()
			m.nav.focus = FocusDiff
			// Collapse hunk 0 first
			hk := hunkKey(m.files[0].Path, 0)
			m.nav.collapsedHunks[hk] = true
			m.nav.buildDiffLines(m.files)
			m.nav.cursor.LineIdx = 0

			linesBefore := len(m.nav.diffLines)
			gomega.Expect(m.nav.diffLines[0].collapsed).To(gomega.BeTrue())

			// Tab on collapsed placeholder should expand
			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
			gomega.Expect(len(m.nav.diffLines)).To(gomega.BeNumerically(">", linesBefore))
			gomega.Expect(m.nav.collapsedHunks[hk]).To(gomega.BeFalse())
		})
	})

	ginkgo.Describe("Toggle all folds with shift+tab", func() {
		ginkgo.It("should collapse all hunks in diff focus", func() {
			m := newInputTestModel()
			m.nav.focus = FocusDiff

			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})

			// All hunks for current file should be collapsed
			file := m.files[m.nav.cursor.FileIdx]
			for hi := range file.Hunks {
				hk := hunkKey(file.Path, hi)
				gomega.Expect(m.nav.collapsedHunks[hk]).To(gomega.BeTrue())
			}
		})
	})

	ginkgo.Describe("Quit handling", func() {
		ginkgo.It("should require double ctrl+c when there are pending comments", func() {
			m := newInputTestModel()
			m.nav.focus = FocusDiff
			// Add a pending comment so hasUnsavedWork returns true
			m.comments.comments = append(m.comments.comments, review.Comment{
				ID:        -1,
				Path:      "src/main.go",
				Line:      1,
				Body:      "test comment",
				Author:    m.currentUser,
				IsPending: true,
			})

			m, _ = m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
			gomega.Expect(m.confirmQuit).To(gomega.BeTrue())
		})
	})

	ginkgo.Describe("Mode precedence", func() {
		ginkgo.It("should prioritize goto mode over diff keys", func() {
			m := newInputTestModel()
			m.search.gotoActive = true
			oldCursor := m.nav.cursor.LineIdx

			// 'j' in goto mode should not move diff cursor
			m, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
			gomega.Expect(m.nav.cursor.LineIdx).To(gomega.Equal(oldCursor))
		})

		ginkgo.It("should prioritize search mode over diff keys", func() {
			m := newInputTestModel()
			m.search.active = true
			m.search.input = ""

			// 'j' in search mode should append to input, not move cursor
			m, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
			gomega.Expect(m.search.input).To(gomega.Equal("j"))
		})

		ginkgo.It("should prioritize filter mode over diff keys", func() {
			m := newInputTestModel()
			m.activateFileFilter()

			// 'j' in filter mode should append to input
			m, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
			gomega.Expect(m.search.filterInput).To(gomega.Equal("j"))
		})
	})

	ginkgo.Describe("Loading state", func() {
		ginkgo.It("should ignore keypresses when loading", func() {
			m := newInputTestModel()
			m.loading = true
			oldCursor := m.nav.cursor.LineIdx

			m, _ = m.Update(tea.KeyPressMsg{Code: 'j'})
			gomega.Expect(m.nav.cursor.LineIdx).To(gomega.Equal(oldCursor))
		})
	})

	ginkgo.Describe("Back/Esc handling", func() {
		ginkgo.It("should clear search query on esc if search is active", func() {
			m := newInputTestModel()
			m.nav.focus = FocusDiff
			m.search.query = "something"

			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
			gomega.Expect(m.search.query).To(gomega.Equal(""))
		})

		ginkgo.It("should exit visual mode on esc", func() {
			m := newInputTestModel()
			m.nav.focus = FocusDiff
			m.nav.visualMode = true

			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
			gomega.Expect(m.nav.visualMode).To(gomega.BeFalse())
		})
	})

	ginkgo.Describe("Tree fold toggle (tab in tree)", func() {
		ginkgo.It("should toggle folder collapse on tab when on a folder", func() {
			m := newInputTestModel()
			m.nav.focus = FocusFileTree

			// Find a folder row
			var folderIdx int = -1
			for i, row := range m.nav.treeRows {
				if row.node.fileIdx == -1 {
					folderIdx = i
					break
				}
			}
			if folderIdx < 0 {
				ginkgo.Skip("No folder rows in test tree")
			}

			m.nav.treeCursor = folderIdx
			dirPath := m.nav.treeRows[folderIdx].node.dirPath
			rowsBefore := len(m.nav.treeRows)

			// Toggle collapse
			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})

			// Should now be collapsed
			gomega.Expect(m.nav.collapsedDirs[dirPath]).To(gomega.BeTrue())
			gomega.Expect(len(m.nav.treeRows)).To(gomega.BeNumerically("<", rowsBefore))
		})
	})

	ginkgo.Describe("Comment select mode", func() {
		ginkgo.It("should deselect comment and return to diff on esc", func() {
			m := newInputTestModel()
			m.nav.focus = FocusDiff
			m.nav.cursor = CursorTarget{Kind: CursorComment, LineIdx: 0, CommentIdx: 1}

			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
			gomega.Expect(m.nav.cursor.Kind).To(gomega.Equal(CursorLine))
		})

		ginkgo.It("should deselect comment when pressing up past first comment", func() {
			m := newInputTestModel()
			m.nav.focus = FocusDiff
			m.nav.cursor = CursorTarget{Kind: CursorComment, LineIdx: 0, CommentIdx: 0}

			m, _ = m.Update(tea.KeyPressMsg{Code: 'k'})
			gomega.Expect(m.nav.cursor.Kind).To(gomega.Equal(CursorLine))
		})
	})
})
