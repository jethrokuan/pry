package diffview

import (
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/jethrokuan/pry/internal/diff"
	"github.com/jethrokuan/pry/internal/review"
)

var _ = ginkgo.Describe("Comment CRUD state management", func() {
	var m Model

	ginkgo.BeforeEach(func() {
		pendingReview := review.NewPendingReview()
		m = Model{
			pr: &review.PullRequest{
				PendingReview: pendingReview,
			},
			pendingReview: pendingReview,
			currentUser:   "testuser",
			inflight:      make(map[int]bool),
			mdCache:       make(map[mdCacheKey]string),
			comments: CommentPanel{
				expanded:         make(map[string]bool),
				commentIndex:     make(map[string][]review.Comment),
				fileCommentIndex: make(map[string]bool),
			},
		}
	})

	ginkgo.Describe("setComments", func() {
		ginkgo.It("replaces the full comment list and rebuilds the index", func() {
			m.setComments([]review.Comment{
				{ID: 1, Path: "main.go", Line: 5, Side: "RIGHT", Body: "looks good", Author: "alice"},
				{ID: 2, Path: "main.go", Line: 5, Side: "RIGHT", Body: "agreed", Author: "bob"},
				{ID: 3, Path: "util.go", Line: 12, Side: "LEFT", Body: "why?", Author: "carol"},
			})

			gomega.Expect(m.comments.comments).To(gomega.HaveLen(3))
			gomega.Expect(m.pr.Comments).To(gomega.HaveLen(3))

			gomega.Expect(m.comments.commentIndex[commentIndexKey("main.go", 5, "RIGHT")]).To(gomega.HaveLen(2))
			gomega.Expect(m.comments.commentIndex[commentIndexKey("util.go", 12, "LEFT")]).To(gomega.HaveLen(1))
			gomega.Expect(m.comments.fileCommentIndex["main.go"]).To(gomega.BeTrue())
			gomega.Expect(m.comments.fileCommentIndex["util.go"]).To(gomega.BeTrue())
		})

		ginkgo.It("replaces previous comments entirely", func() {
			m.setComments([]review.Comment{
				{ID: 1, Path: "old.go", Line: 1, Side: "RIGHT", Body: "old"},
			})
			m.setComments([]review.Comment{
				{ID: 2, Path: "new.go", Line: 2, Side: "LEFT", Body: "new"},
			})

			gomega.Expect(m.comments.comments).To(gomega.HaveLen(1))
			gomega.Expect(m.comments.comments[0].Path).To(gomega.Equal("new.go"))
			gomega.Expect(m.comments.fileCommentIndex["old.go"]).To(gomega.BeFalse())
			gomega.Expect(m.comments.fileCommentIndex["new.go"]).To(gomega.BeTrue())
		})

		ginkgo.It("syncs pr.Comments with the panel", func() {
			comments := []review.Comment{
				{ID: 10, Path: "a.go", Line: 1, Side: "RIGHT", Body: "hello"},
			}
			m.setComments(comments)
			gomega.Expect(m.pr.Comments).To(gomega.HaveLen(1))
			gomega.Expect(m.pr.Comments[0].ID).To(gomega.Equal(10))
		})
	})

	ginkgo.Describe("addOptimisticComment", func() {
		ginkgo.It("adds a comment with a negative temp ID and updates the index", func() {
			tempID := m.addOptimisticComment(review.Comment{
				Path: "main.go", Line: 10, Side: "RIGHT", Body: "fix this",
				Author: "testuser", IsPending: true,
			})
			gomega.Expect(tempID).To(gomega.BeNumerically("<", 0))
			gomega.Expect(m.comments.comments).To(gomega.HaveLen(1))
			gomega.Expect(m.comments.comments[0].ID).To(gomega.Equal(tempID))
			gomega.Expect(m.comments.comments[0].Body).To(gomega.Equal("fix this"))

			indexed := m.comments.commentIndex[commentIndexKey("main.go", 10, "RIGHT")]
			gomega.Expect(indexed).To(gomega.HaveLen(1))
			gomega.Expect(indexed[0].ID).To(gomega.Equal(tempID))
		})

		ginkgo.It("assigns unique IDs to multiple comments", func() {
			id1 := m.addOptimisticComment(review.Comment{
				Path: "a.go", Line: 1, Side: "RIGHT", Body: "first",
				Author: "testuser", IsPending: true,
			})
			id2 := m.addOptimisticComment(review.Comment{
				Path: "a.go", Line: 1, Side: "RIGHT", Body: "second",
				Author: "testuser", IsPending: true,
			})
			gomega.Expect(id1).NotTo(gomega.Equal(id2))
			gomega.Expect(m.comments.comments).To(gomega.HaveLen(2))

			indexed := m.comments.commentIndex[commentIndexKey("a.go", 1, "RIGHT")]
			gomega.Expect(indexed).To(gomega.HaveLen(2))
		})

		ginkgo.It("indexes comments on different files separately", func() {
			m.addOptimisticComment(review.Comment{
				Path: "a.go", Line: 5, Side: "RIGHT", Body: "comment a",
				Author: "testuser", IsPending: true,
			})
			m.addOptimisticComment(review.Comment{
				Path: "b.go", Line: 5, Side: "RIGHT", Body: "comment b",
				Author: "testuser", IsPending: true,
			})

			gomega.Expect(m.comments.commentIndex[commentIndexKey("a.go", 5, "RIGHT")]).To(gomega.HaveLen(1))
			gomega.Expect(m.comments.commentIndex[commentIndexKey("b.go", 5, "RIGHT")]).To(gomega.HaveLen(1))
			gomega.Expect(m.comments.fileCommentIndex["a.go"]).To(gomega.BeTrue())
			gomega.Expect(m.comments.fileCommentIndex["b.go"]).To(gomega.BeTrue())
		})
	})

	ginkgo.Describe("removeCommentByID", func() {
		ginkgo.It("removes a comment and clears it from the index", func() {
			tempID := m.addOptimisticComment(review.Comment{
				Path: "main.go", Line: 10, Side: "RIGHT", Body: "fix this",
				Author: "testuser", IsPending: true,
			})

			m.removeCommentByID(tempID)
			gomega.Expect(m.comments.comments).To(gomega.BeEmpty())

			indexed := m.comments.commentIndex[commentIndexKey("main.go", 10, "RIGHT")]
			gomega.Expect(indexed).To(gomega.BeEmpty())
		})

		ginkgo.It("removes only the targeted comment, leaving others intact", func() {
			id1 := m.addOptimisticComment(review.Comment{
				Path: "main.go", Line: 10, Side: "RIGHT", Body: "first",
				Author: "testuser", IsPending: true,
			})
			id2 := m.addOptimisticComment(review.Comment{
				Path: "main.go", Line: 10, Side: "RIGHT", Body: "second",
				Author: "testuser", IsPending: true,
			})

			m.removeCommentByID(id1)
			gomega.Expect(m.comments.comments).To(gomega.HaveLen(1))
			gomega.Expect(m.comments.comments[0].ID).To(gomega.Equal(id2))

			indexed := m.comments.commentIndex[commentIndexKey("main.go", 10, "RIGHT")]
			gomega.Expect(indexed).To(gomega.HaveLen(1))
			gomega.Expect(indexed[0].ID).To(gomega.Equal(id2))
		})

		ginkgo.It("clears fileCommentIndex when last comment on a file is removed", func() {
			tempID := m.addOptimisticComment(review.Comment{
				Path: "only.go", Line: 1, Side: "RIGHT", Body: "lone comment",
				Author: "testuser", IsPending: true,
			})
			gomega.Expect(m.comments.fileCommentIndex["only.go"]).To(gomega.BeTrue())

			m.removeCommentByID(tempID)
			gomega.Expect(m.comments.fileCommentIndex["only.go"]).To(gomega.BeFalse())
		})

		ginkgo.It("is a no-op for non-existent ID", func() {
			m.addOptimisticComment(review.Comment{
				Path: "a.go", Line: 1, Side: "RIGHT", Body: "keep",
				Author: "testuser", IsPending: true,
			})
			m.removeCommentByID(9999)
			gomega.Expect(m.comments.comments).To(gomega.HaveLen(1))
		})
	})

	ginkgo.Describe("replaceComment", func() {
		ginkgo.It("swaps an optimistic comment with a real one from the server", func() {
			tempID := m.addOptimisticComment(review.Comment{
				Path: "main.go", Line: 10, Side: "RIGHT", Body: "draft",
				Author: "testuser", IsPending: true,
			})

			realComment := review.Comment{
				ID: 42, Path: "main.go", Line: 10, Side: "RIGHT",
				Body: "draft", Author: "testuser", IsPending: true,
			}
			m.replaceComment(tempID, realComment)

			gomega.Expect(m.comments.comments).To(gomega.HaveLen(1))
			gomega.Expect(m.comments.comments[0].ID).To(gomega.Equal(42))

			// Index should reference the new ID
			indexed := m.comments.commentIndex[commentIndexKey("main.go", 10, "RIGHT")]
			gomega.Expect(indexed).To(gomega.HaveLen(1))
			gomega.Expect(indexed[0].ID).To(gomega.Equal(42))
		})

		ginkgo.It("is a no-op for non-existent temp ID", func() {
			m.addOptimisticComment(review.Comment{
				Path: "a.go", Line: 1, Side: "RIGHT", Body: "keep",
				Author: "testuser", IsPending: true,
			})
			m.replaceComment(9999, review.Comment{ID: 100, Body: "nope"})
			gomega.Expect(m.comments.comments).To(gomega.HaveLen(1))
			gomega.Expect(m.comments.comments[0].Body).To(gomega.Equal("keep"))
		})

		ginkgo.It("syncs pr.Comments after replacement", func() {
			tempID := m.addOptimisticComment(review.Comment{
				Path: "a.go", Line: 1, Side: "RIGHT", Body: "temp",
				Author: "testuser", IsPending: true,
			})
			m.replaceComment(tempID, review.Comment{
				ID: 55, Path: "a.go", Line: 1, Side: "RIGHT", Body: "real",
			})
			gomega.Expect(m.pr.Comments).To(gomega.HaveLen(1))
			gomega.Expect(m.pr.Comments[0].ID).To(gomega.Equal(55))
		})
	})

	ginkgo.Describe("updateCommentBody", func() {
		ginkgo.It("updates the comment body while preserving other fields", func() {
			tempID := m.addOptimisticComment(review.Comment{
				Path: "main.go", Line: 10, Side: "RIGHT", Body: "original",
				Author: "testuser", IsPending: true,
			})

			m.updateCommentBody(tempID, "updated")

			c := m.findCommentByID(tempID)
			gomega.Expect(c).NotTo(gomega.BeNil())
			gomega.Expect(c.Body).To(gomega.Equal("updated"))
			gomega.Expect(c.Path).To(gomega.Equal("main.go"))
			gomega.Expect(c.Line).To(gomega.Equal(10))
			gomega.Expect(c.Side).To(gomega.Equal("RIGHT"))
		})

		ginkgo.It("rebuilds the index after update", func() {
			tempID := m.addOptimisticComment(review.Comment{
				Path: "main.go", Line: 10, Side: "RIGHT", Body: "original",
				Author: "testuser", IsPending: true,
			})

			m.updateCommentBody(tempID, "updated")

			indexed := m.comments.commentIndex[commentIndexKey("main.go", 10, "RIGHT")]
			gomega.Expect(indexed).To(gomega.HaveLen(1))
			gomega.Expect(indexed[0].Body).To(gomega.Equal("updated"))
		})

		ginkgo.It("is a no-op for non-existent ID", func() {
			m.addOptimisticComment(review.Comment{
				Path: "main.go", Line: 10, Side: "RIGHT", Body: "original",
				Author: "testuser", IsPending: true,
			})

			m.updateCommentBody(9999, "should not happen")

			gomega.Expect(m.comments.comments).To(gomega.HaveLen(1))
			gomega.Expect(m.comments.comments[0].Body).To(gomega.Equal("original"))
		})
	})

	ginkgo.Describe("findCommentByID", func() {
		ginkgo.It("returns a pointer to the matching comment", func() {
			tempID := m.addOptimisticComment(review.Comment{
				Path: "a.go", Line: 1, Side: "RIGHT", Body: "find me",
				Author: "testuser", IsPending: true,
			})

			c := m.findCommentByID(tempID)
			gomega.Expect(c).NotTo(gomega.BeNil())
			gomega.Expect(c.Body).To(gomega.Equal("find me"))
		})

		ginkgo.It("returns nil for non-existent ID", func() {
			c := m.findCommentByID(9999)
			gomega.Expect(c).To(gomega.BeNil())
		})
	})

	ginkgo.Describe("mergePendingComments", func() {
		ginkgo.It("adds pending comments that are not already present", func() {
			m.setComments([]review.Comment{
				{ID: 1, Path: "a.go", Line: 10, Side: "RIGHT", Body: "existing", Author: "alice"},
			})

			m.mergePendingComments([]review.Comment{
				{ID: 2, Path: "a.go", Line: 10, Side: "RIGHT", Body: "pending", Author: "testuser", IsPending: true},
				{ID: 3, Path: "b.go", Line: 5, Side: "LEFT", Body: "another", Author: "testuser", IsPending: true},
			})

			gomega.Expect(m.comments.comments).To(gomega.HaveLen(3))
			gomega.Expect(m.comments.fileCommentIndex["a.go"]).To(gomega.BeTrue())
			gomega.Expect(m.comments.fileCommentIndex["b.go"]).To(gomega.BeTrue())
		})

		ginkgo.It("skips duplicates by ID", func() {
			m.setComments([]review.Comment{
				{ID: 1, Path: "a.go", Line: 10, Side: "RIGHT", Body: "existing", Author: "alice"},
			})

			m.mergePendingComments([]review.Comment{
				{ID: 1, Path: "a.go", Line: 10, Side: "RIGHT", Body: "duplicate", Author: "alice"},
			})

			gomega.Expect(m.comments.comments).To(gomega.HaveLen(1))
			gomega.Expect(m.comments.comments[0].Body).To(gomega.Equal("existing"))
		})

		ginkgo.It("syncs pr.Comments after merge", func() {
			m.setComments([]review.Comment{
				{ID: 1, Path: "a.go", Line: 1, Side: "RIGHT", Body: "one"},
			})
			m.mergePendingComments([]review.Comment{
				{ID: 2, Path: "b.go", Line: 2, Side: "LEFT", Body: "two"},
			})
			gomega.Expect(m.pr.Comments).To(gomega.HaveLen(2))
		})
	})

	ginkgo.Describe("CommentPanel.RebuildIndex", func() {
		ginkgo.It("produces correct mappings from the comment list", func() {
			m.comments.comments = []review.Comment{
				{ID: 1, Path: "a.go", Line: 10, Side: "RIGHT", Body: "existing", Author: "alice"},
				{ID: 2, Path: "a.go", Line: 10, Side: "RIGHT", Body: "pending", Author: "testuser", IsPending: true},
				{ID: 3, Path: "b.go", Line: 5, Side: "LEFT", Body: "other file", Author: "bob"},
			}

			m.comments.RebuildIndex()

			aComments := m.comments.commentIndex[commentIndexKey("a.go", 10, "RIGHT")]
			gomega.Expect(aComments).To(gomega.HaveLen(2))

			bComments := m.comments.commentIndex[commentIndexKey("b.go", 5, "LEFT")]
			gomega.Expect(bComments).To(gomega.HaveLen(1))

			gomega.Expect(m.comments.fileCommentIndex["a.go"]).To(gomega.BeTrue())
			gomega.Expect(m.comments.fileCommentIndex["b.go"]).To(gomega.BeTrue())
		})

		ginkgo.It("clears stale entries on rebuild", func() {
			m.addOptimisticComment(review.Comment{
				Path: "old.go", Line: 1, Side: "RIGHT", Body: "stale",
				Author: "testuser", IsPending: true,
			})
			gomega.Expect(m.comments.fileCommentIndex["old.go"]).To(gomega.BeTrue())

			// Clear comments and rebuild
			m.comments.comments = nil
			m.comments.RebuildIndex()

			gomega.Expect(m.comments.commentIndex[commentIndexKey("old.go", 1, "RIGHT")]).To(gomega.BeEmpty())
			gomega.Expect(m.comments.fileCommentIndex["old.go"]).To(gomega.BeFalse())
		})
	})

	ginkgo.Describe("CommentsForLine", func() {
		ginkgo.It("returns exact side matches", func() {
			m.setComments([]review.Comment{
				{ID: 1, Path: "f.go", Line: 10, Side: "RIGHT", Body: "right"},
				{ID: 2, Path: "f.go", Line: 10, Side: "LEFT", Body: "left"},
			})

			right := m.comments.CommentsForLine("f.go", 10, "RIGHT")
			gomega.Expect(right).To(gomega.HaveLen(1))
			gomega.Expect(right[0].Body).To(gomega.Equal("right"))

			left := m.comments.CommentsForLine("f.go", 10, "LEFT")
			gomega.Expect(left).To(gomega.HaveLen(1))
			gomega.Expect(left[0].Body).To(gomega.Equal("left"))
		})

		ginkgo.It("includes empty-side comments in any side query", func() {
			m.setComments([]review.Comment{
				{ID: 1, Path: "f.go", Line: 10, Side: "", Body: "no side"},
				{ID: 2, Path: "f.go", Line: 10, Side: "RIGHT", Body: "right"},
			})

			right := m.comments.CommentsForLine("f.go", 10, "RIGHT")
			gomega.Expect(right).To(gomega.HaveLen(2))
		})

		ginkgo.It("returns nil for lines with no comments", func() {
			result := m.comments.CommentsForLine("f.go", 999, "RIGHT")
			gomega.Expect(result).To(gomega.BeEmpty())
		})
	})

	ginkgo.Describe("commentKey and commentIndexKey", func() {
		ginkgo.It("produces expected formats", func() {
			gomega.Expect(commentKey("path/file.go", 42)).To(gomega.Equal("path/file.go:42"))
			gomega.Expect(commentIndexKey("path/file.go", 42, "RIGHT")).To(gomega.Equal("path/file.go:42:RIGHT"))
		})
	})

	ginkgo.Describe("navigateComment with pending comments", func() {
		ginkgo.BeforeEach(func() {
			// Set up a file with diffLines
			m.files = []diff.DiffFile{
				{
					Path: "main.go",
					Hunks: []diff.Hunk{
						{
							OldStart: 1, OldLines: 5, NewStart: 1, NewLines: 5,
							Lines: []diff.DiffLine{
								{Type: diff.LineContext, OldNum: 1, NewNum: 1, Content: " line1"},
								{Type: diff.LineContext, OldNum: 2, NewNum: 2, Content: " line2"},
								{Type: diff.LineAddition, OldNum: 0, NewNum: 3, Content: "+line3"},
								{Type: diff.LineContext, OldNum: 3, NewNum: 4, Content: " line4"},
								{Type: diff.LineContext, OldNum: 4, NewNum: 5, Content: " line5"},
							},
						},
					},
				},
			}
			m.nav.collapsedHunks = make(map[string]bool)
			m.nav.buildDiffLines(m.files)
		})

		ginkgo.It("cycles to a pending comment on a different line", func() {
			// Cursor starts at line 0
			m.nav.cursor.LineIdx = 0

			// Add a pending comment on newLine=3 (diffLine index 2)
			m.addOptimisticComment(review.Comment{
				Path: "main.go", Line: 3, Side: "RIGHT", Body: "pending fix",
				Author: "testuser", IsPending: true,
			})

			m.navigateComment(true, false)

			gomega.Expect(m.nav.cursor.LineIdx).To(gomega.Equal(2))
			gomega.Expect(m.nav.cursor.Kind).To(gomega.Equal(CursorComment))
		})

		ginkgo.It("cycles backward to a pending comment", func() {
			// Cursor at end
			m.nav.cursor.LineIdx = 4

			m.addOptimisticComment(review.Comment{
				Path: "main.go", Line: 3, Side: "RIGHT", Body: "pending fix",
				Author: "testuser", IsPending: true,
			})

			m.navigateComment(false, false)

			gomega.Expect(m.nav.cursor.LineIdx).To(gomega.Equal(2))
			gomega.Expect(m.nav.cursor.Kind).To(gomega.Equal(CursorComment))
		})

		ginkgo.It("finds pending comments across files", func() {
			m.files = append(m.files, diff.DiffFile{
				Path: "other.go",
				Hunks: []diff.Hunk{
					{
						OldStart: 1, OldLines: 2, NewStart: 1, NewLines: 2,
						Lines: []diff.DiffLine{
							{Type: diff.LineContext, OldNum: 1, NewNum: 1, Content: " a"},
							{Type: diff.LineAddition, OldNum: 0, NewNum: 2, Content: "+b"},
						},
					},
				},
			})

			// Add pending comment on the second file
			m.addOptimisticComment(review.Comment{
				Path: "other.go", Line: 2, Side: "RIGHT", Body: "review this",
				Author: "testuser", IsPending: true,
			})

			// Cursor on first file, no comments here
			m.nav.cursor.FileIdx = 0
			m.nav.cursor.LineIdx = 0
			m.nav.buildDiffLines(m.files)

			m.navigateComment(true, false)

			// Should have jumped to the second file
			gomega.Expect(m.nav.cursor.FileIdx).To(gomega.Equal(1))
			gomega.Expect(m.nav.cursor.Kind).To(gomega.Equal(CursorComment))
		})
	})
})
