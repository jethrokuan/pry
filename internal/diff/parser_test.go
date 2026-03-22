package diff_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jethrokuan/pry/internal/diff"
)

var _ = Describe("Parser", func() {
	Describe("ParseFromPatches", func() {
		It("parses a simple patch", func() {
			patches := []diff.FilePatch{
				{
					Filename:  "main.go",
					Status:    "modified",
					Additions: 1,
					Deletions: 1,
					Patch: `@@ -1,3 +1,3 @@
 package main

-var x = 1
+var x = 2`,
				},
			}

			files := diff.ParseFromPatches(patches)
			Expect(files).To(HaveLen(1))
			Expect(files[0].Path).To(Equal("main.go"))
			Expect(files[0].Status).To(Equal(diff.StatusModified))
			Expect(files[0].Hunks).To(HaveLen(1))
			Expect(files[0].Hunks[0].Lines).To(HaveLen(4))
		})

		It("handles added files", func() {
			patches := []diff.FilePatch{
				{
					Filename:  "new.go",
					Status:    "added",
					Additions: 2,
					Patch: `@@ -0,0 +1,2 @@
+package new
+func init() {}`,
				},
			}

			files := diff.ParseFromPatches(patches)
			Expect(files).To(HaveLen(1))
			Expect(files[0].Status).To(Equal(diff.StatusAdded))
		})

		It("handles removed files", func() {
			patches := []diff.FilePatch{
				{
					Filename:  "old.go",
					Status:    "removed",
					Deletions: 1,
					Patch: `@@ -1,1 +0,0 @@
-package old`,
				},
			}

			files := diff.ParseFromPatches(patches)
			Expect(files).To(HaveLen(1))
			Expect(files[0].Status).To(Equal(diff.StatusDeleted))
		})

		It("handles renamed files", func() {
			patches := []diff.FilePatch{
				{
					Filename:         "new_name.go",
					PreviousFilename: "old_name.go",
					Status:           "renamed",
					Patch: `@@ -1,3 +1,3 @@
 package main

-var x = 1
+var x = 2`,
				},
			}

			files := diff.ParseFromPatches(patches)
			Expect(files).To(HaveLen(1))
			Expect(files[0].Status).To(Equal(diff.StatusRenamed))
			Expect(files[0].OldPath).To(Equal("old_name.go"))
		})

		It("handles binary files with empty patch", func() {
			patches := []diff.FilePatch{
				{
					Filename: "image.png",
					Status:   "modified",
					Patch:    "",
				},
			}

			files := diff.ParseFromPatches(patches)
			Expect(files).To(HaveLen(1))
			Expect(files[0].IsBinary).To(BeTrue())
		})
	})

	Describe("Line numbering", func() {
		It("assigns correct line numbers to additions and deletions", func() {
			patches := []diff.FilePatch{
				{
					Filename: "test.go",
					Status:   "modified",
					Patch: `@@ -1,4 +1,4 @@
 line1
-old2
+new2
 line3`,
				},
			}

			files := diff.ParseFromPatches(patches)
			Expect(files).To(HaveLen(1))

			lines := files[0].Hunks[0].Lines
			Expect(lines).To(HaveLen(4))

			// Context line: both old and new numbers
			Expect(lines[0].OldNum).To(Equal(1))
			Expect(lines[0].NewNum).To(Equal(1))
			Expect(lines[0].Type).To(Equal(diff.LineContext))

			// Deletion: only old number
			Expect(lines[1].OldNum).To(Equal(2))
			Expect(lines[1].NewNum).To(Equal(0))
			Expect(lines[1].Type).To(Equal(diff.LineDeletion))

			// Addition: only new number
			Expect(lines[2].OldNum).To(Equal(0))
			Expect(lines[2].NewNum).To(Equal(2))
			Expect(lines[2].Type).To(Equal(diff.LineAddition))

			// Context line after change
			Expect(lines[3].OldNum).To(Equal(3))
			Expect(lines[3].NewNum).To(Equal(3))
		})
	})
})
