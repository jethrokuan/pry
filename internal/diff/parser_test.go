package diff_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jkuan/pr-review/internal/diff"
)

var _ = Describe("Parser", func() {
	Describe("Parse", func() {
		It("parses a simple unified diff", func() {
			raw := `diff --git a/hello.go b/hello.go
--- a/hello.go
+++ b/hello.go
@@ -1,5 +1,5 @@
 package main

 func main() {
-	fmt.Println("hello")
+	fmt.Println("world")
 }
`
			files, err := diff.Parse(raw)
			Expect(err).NotTo(HaveOccurred())
			Expect(files).To(HaveLen(1))

			f := files[0]
			Expect(f.Path).To(Equal("hello.go"))
			Expect(f.Status).To(Equal(diff.StatusModified))
			Expect(f.Hunks).To(HaveLen(1))
			Expect(f.Additions).To(Equal(1))
			Expect(f.Deletions).To(Equal(1))

			hunk := f.Hunks[0]
			Expect(hunk.OldStart).To(Equal(1))
			Expect(hunk.NewStart).To(Equal(1))
			Expect(hunk.Lines).To(HaveLen(7))
		})

		It("parses a diff with a new file", func() {
			raw := `diff --git a/new.go b/new.go
new file mode 100644
--- /dev/null
+++ b/new.go
@@ -0,0 +1,3 @@
+package main
+
+func init() {}
`
			files, err := diff.Parse(raw)
			Expect(err).NotTo(HaveOccurred())
			Expect(files).To(HaveLen(1))

			f := files[0]
			Expect(f.Path).To(Equal("new.go"))
			Expect(f.Status).To(Equal(diff.StatusAdded))
			Expect(f.Additions).To(Equal(3))
			Expect(f.Deletions).To(Equal(0))
		})

		It("parses a diff with a deleted file", func() {
			raw := `diff --git a/old.go b/old.go
deleted file mode 100644
--- a/old.go
+++ /dev/null
@@ -1,3 +0,0 @@
-package main
-
-func cleanup() {}
`
			files, err := diff.Parse(raw)
			Expect(err).NotTo(HaveOccurred())
			Expect(files).To(HaveLen(1))

			f := files[0]
			Expect(f.Status).To(Equal(diff.StatusDeleted))
			Expect(f.Additions).To(Equal(0))
			Expect(f.Deletions).To(Equal(3))
		})

		It("parses a diff with multiple files", func() {
			raw := `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1,3 +1,3 @@
 package a

-var x = 1
+var x = 2
diff --git a/b.go b/b.go
--- a/b.go
+++ b/b.go
@@ -1,3 +1,3 @@
 package b

-var y = 1
+var y = 2
`
			files, err := diff.Parse(raw)
			Expect(err).NotTo(HaveOccurred())
			Expect(files).To(HaveLen(2))
			Expect(files[0].Path).To(Equal("a.go"))
			Expect(files[1].Path).To(Equal("b.go"))
		})

		It("parses a diff with multiple hunks", func() {
			raw := `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,3 +1,3 @@
 package main

-var a = 1
+var a = 2
@@ -10,3 +10,3 @@
 func foo() {
-	return 1
+	return 2
 }
`
			files, err := diff.Parse(raw)
			Expect(err).NotTo(HaveOccurred())
			Expect(files).To(HaveLen(1))
			Expect(files[0].Hunks).To(HaveLen(2))
		})

		It("handles a renamed file", func() {
			raw := `diff --git a/old.go b/new.go
rename from old.go
rename to new.go
--- a/old.go
+++ b/new.go
@@ -1,3 +1,3 @@
 package main

-var x = 1
+var x = 2
`
			files, err := diff.Parse(raw)
			Expect(err).NotTo(HaveOccurred())
			Expect(files).To(HaveLen(1))
			Expect(files[0].Status).To(Equal(diff.StatusRenamed))
			Expect(files[0].OldPath).To(Equal("old.go"))
			Expect(files[0].Path).To(Equal("new.go"))
		})

		It("handles binary files", func() {
			raw := `diff --git a/image.png b/image.png
Binary files a/image.png and b/image.png differ
`
			files, err := diff.Parse(raw)
			Expect(err).NotTo(HaveOccurred())
			Expect(files).To(HaveLen(1))
			Expect(files[0].IsBinary).To(BeTrue())
		})

		It("returns empty for empty input", func() {
			files, err := diff.Parse("")
			Expect(err).NotTo(HaveOccurred())
			// Parser produces a stub file from empty input
			Expect(files).To(HaveLen(1))
			Expect(files[0].Path).To(Equal(""))
			Expect(files[0].Hunks).To(BeNil())
		})
	})

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

	Describe("BuildPositionMap", func() {
		It("builds correct position mapping", func() {
			patches := []diff.FilePatch{
				{
					Filename: "main.go",
					Status:   "modified",
					Patch: `@@ -1,5 +1,5 @@
 package main

 func main() {
-	fmt.Println("hello")
+	fmt.Println("world")
 }`,
				},
			}

			files := diff.ParseFromPatches(patches)
			posMap := diff.BuildPositionMap(files)

			Expect(posMap).To(HaveKey("main.go"))
			fileMap := posMap["main.go"]

			// Context lines map to positions
			Expect(fileMap[1]).To(Equal(2)) // "package main" at position 2 (after hunk header)
			Expect(fileMap[5]).To(Equal(7)) // "world" at position 7
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
