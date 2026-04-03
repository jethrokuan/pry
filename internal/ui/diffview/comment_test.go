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
			comments: CommentPanel{
				expanded:        make(map[string]bool),
				threadIndex:     make(map[string][]review.Thread),
				fileThreadIndex: make(map[string]bool),
			},
		}
	})

	ginkgo.Describe("setThreads", func() {
		ginkgo.It("replaces the full thread list and rebuilds the index", func() {
			m.setThreads([]review.Thread{
				{Path: "main.go", Line: 5, Side: "RIGHT", Comments: []review.Comment{
					{ID: 1, Body: "looks good", Author: "alice"},
				}},
				{Path: "main.go", Line: 5, Side: "RIGHT", Comments: []review.Comment{
					{ID: 2, Body: "agreed", Author: "bob"},
				}},
				{Path: "util.go", Line: 12, Side: "LEFT", Comments: []review.Comment{
					{ID: 3, Body: "why?", Author: "carol"},
				}},
			})

			gomega.Expect(m.comments.threads).To(gomega.HaveLen(3))
			gomega.Expect(m.pr.Threads).To(gomega.HaveLen(3))

			gomega.Expect(m.comments.threadIndex[commentIndexKey("main.go", 5, "RIGHT")]).To(gomega.HaveLen(2))
			gomega.Expect(m.comments.threadIndex[commentIndexKey("util.go", 12, "LEFT")]).To(gomega.HaveLen(1))
			gomega.Expect(m.comments.fileThreadIndex["main.go"]).To(gomega.BeTrue())
			gomega.Expect(m.comments.fileThreadIndex["util.go"]).To(gomega.BeTrue())
		})

		ginkgo.It("replaces previous threads entirely", func() {
			m.setThreads([]review.Thread{
				{Path: "old.go", Line: 1, Side: "RIGHT", Comments: []review.Comment{
					{ID: 1, Body: "old"},
				}},
			})
			m.setThreads([]review.Thread{
				{Path: "new.go", Line: 2, Side: "LEFT", Comments: []review.Comment{
					{ID: 2, Body: "new"},
				}},
			})

			gomega.Expect(m.comments.threads).To(gomega.HaveLen(1))
			gomega.Expect(m.comments.threads[0].Path).To(gomega.Equal("new.go"))
			gomega.Expect(m.comments.fileThreadIndex["old.go"]).To(gomega.BeFalse())
			gomega.Expect(m.comments.fileThreadIndex["new.go"]).To(gomega.BeTrue())
		})

		ginkgo.It("syncs pr.Threads with the panel", func() {
			threads := []review.Thread{
				{Path: "a.go", Line: 1, Side: "RIGHT", Comments: []review.Comment{
					{ID: 10, Body: "hello"},
				}},
			}
			m.setThreads(threads)
			gomega.Expect(m.pr.Threads).To(gomega.HaveLen(1))
			gomega.Expect(m.pr.Threads[0].Comments[0].ID).To(gomega.Equal(10))
		})
	})

	ginkgo.Describe("addOptimisticThread", func() {
		ginkgo.It("adds a thread with a negative temp ID and updates the index", func() {
			tempID := m.addOptimisticThread("main.go", 10, 0, "RIGHT", review.Comment{
				Body: "fix this", Author: "testuser", IsPending: true,
			})
			gomega.Expect(tempID).To(gomega.BeNumerically("<", 0))
			gomega.Expect(m.comments.threads).To(gomega.HaveLen(1))
			gomega.Expect(m.comments.threads[0].Comments[0].ID).To(gomega.Equal(tempID))
			gomega.Expect(m.comments.threads[0].Comments[0].Body).To(gomega.Equal("fix this"))

			indexed := m.comments.threadIndex[commentIndexKey("main.go", 10, "RIGHT")]
			gomega.Expect(indexed).To(gomega.HaveLen(1))
			gomega.Expect(indexed[0].Comments[0].ID).To(gomega.Equal(tempID))
		})

		ginkgo.It("assigns unique IDs to multiple threads", func() {
			id1 := m.addOptimisticThread("a.go", 1, 0, "RIGHT", review.Comment{
				Body: "first", Author: "testuser", IsPending: true,
			})
			id2 := m.addOptimisticThread("a.go", 1, 0, "RIGHT", review.Comment{
				Body: "second", Author: "testuser", IsPending: true,
			})
			gomega.Expect(id1).NotTo(gomega.Equal(id2))
			gomega.Expect(m.comments.threads).To(gomega.HaveLen(2))

			indexed := m.comments.threadIndex[commentIndexKey("a.go", 1, "RIGHT")]
			gomega.Expect(indexed).To(gomega.HaveLen(2))
		})

		ginkgo.It("indexes threads on different files separately", func() {
			m.addOptimisticThread("a.go", 5, 0, "RIGHT", review.Comment{
				Body: "comment a", Author: "testuser", IsPending: true,
			})
			m.addOptimisticThread("b.go", 5, 0, "RIGHT", review.Comment{
				Body: "comment b", Author: "testuser", IsPending: true,
			})

			gomega.Expect(m.comments.threadIndex[commentIndexKey("a.go", 5, "RIGHT")]).To(gomega.HaveLen(1))
			gomega.Expect(m.comments.threadIndex[commentIndexKey("b.go", 5, "RIGHT")]).To(gomega.HaveLen(1))
			gomega.Expect(m.comments.fileThreadIndex["a.go"]).To(gomega.BeTrue())
			gomega.Expect(m.comments.fileThreadIndex["b.go"]).To(gomega.BeTrue())
		})
	})

	ginkgo.Describe("removeCommentByID", func() {
		ginkgo.It("removes a comment and clears it from the index", func() {
			tempID := m.addOptimisticThread("main.go", 10, 0, "RIGHT", review.Comment{
				Body: "fix this", Author: "testuser", IsPending: true,
			})

			m.removeCommentByID(tempID)
			gomega.Expect(m.comments.threads).To(gomega.BeEmpty())

			indexed := m.comments.threadIndex[commentIndexKey("main.go", 10, "RIGHT")]
			gomega.Expect(indexed).To(gomega.BeEmpty())
		})

		ginkgo.It("removes only the targeted thread, leaving others intact", func() {
			id1 := m.addOptimisticThread("main.go", 10, 0, "RIGHT", review.Comment{
				Body: "first", Author: "testuser", IsPending: true,
			})
			id2 := m.addOptimisticThread("main.go", 10, 0, "RIGHT", review.Comment{
				Body: "second", Author: "testuser", IsPending: true,
			})

			m.removeCommentByID(id1)
			gomega.Expect(m.comments.threads).To(gomega.HaveLen(1))
			gomega.Expect(m.comments.threads[0].Comments[0].ID).To(gomega.Equal(id2))

			indexed := m.comments.threadIndex[commentIndexKey("main.go", 10, "RIGHT")]
			gomega.Expect(indexed).To(gomega.HaveLen(1))
			gomega.Expect(indexed[0].Comments[0].ID).To(gomega.Equal(id2))
		})

		ginkgo.It("clears fileThreadIndex when last thread on a file is removed", func() {
			tempID := m.addOptimisticThread("only.go", 1, 0, "RIGHT", review.Comment{
				Body: "lone comment", Author: "testuser", IsPending: true,
			})
			gomega.Expect(m.comments.fileThreadIndex["only.go"]).To(gomega.BeTrue())

			m.removeCommentByID(tempID)
			gomega.Expect(m.comments.fileThreadIndex["only.go"]).To(gomega.BeFalse())
		})

		ginkgo.It("is a no-op for non-existent ID", func() {
			m.addOptimisticThread("a.go", 1, 0, "RIGHT", review.Comment{
				Body: "keep", Author: "testuser", IsPending: true,
			})
			m.removeCommentByID(9999)
			gomega.Expect(m.comments.threads).To(gomega.HaveLen(1))
		})
	})

	ginkgo.Describe("replaceComment", func() {
		ginkgo.It("swaps an optimistic comment with a real one from the server", func() {
			tempID := m.addOptimisticThread("main.go", 10, 0, "RIGHT", review.Comment{
				Body: "draft", Author: "testuser", IsPending: true,
			})

			realComment := review.Comment{
				ID: 42, Body: "draft", Author: "testuser", IsPending: true,
			}
			m.replaceComment(tempID, realComment)

			gomega.Expect(m.comments.threads).To(gomega.HaveLen(1))
			gomega.Expect(m.comments.threads[0].Comments[0].ID).To(gomega.Equal(42))

			// Index should reference the new ID
			indexed := m.comments.threadIndex[commentIndexKey("main.go", 10, "RIGHT")]
			gomega.Expect(indexed).To(gomega.HaveLen(1))
			gomega.Expect(indexed[0].Comments[0].ID).To(gomega.Equal(42))
		})

		ginkgo.It("is a no-op for non-existent temp ID", func() {
			m.addOptimisticThread("a.go", 1, 0, "RIGHT", review.Comment{
				Body: "keep", Author: "testuser", IsPending: true,
			})
			m.replaceComment(9999, review.Comment{ID: 100, Body: "nope"})
			gomega.Expect(m.comments.threads).To(gomega.HaveLen(1))
			gomega.Expect(m.comments.threads[0].Comments[0].Body).To(gomega.Equal("keep"))
		})

		ginkgo.It("syncs pr.Threads after replacement", func() {
			tempID := m.addOptimisticThread("a.go", 1, 0, "RIGHT", review.Comment{
				Body: "temp", Author: "testuser", IsPending: true,
			})
			m.replaceComment(tempID, review.Comment{
				ID: 55, Body: "real",
			})
			gomega.Expect(m.pr.Threads).To(gomega.HaveLen(1))
			gomega.Expect(m.pr.Threads[0].Comments[0].ID).To(gomega.Equal(55))
		})
	})

	ginkgo.Describe("updateCommentBody", func() {
		ginkgo.It("updates the comment body while preserving other fields", func() {
			tempID := m.addOptimisticThread("main.go", 10, 0, "RIGHT", review.Comment{
				Body: "original", Author: "testuser", IsPending: true,
			})

			m.updateCommentBody(tempID, "updated")

			c := m.findCommentByID(tempID)
			gomega.Expect(c).NotTo(gomega.BeNil())
			gomega.Expect(c.Body).To(gomega.Equal("updated"))
			gomega.Expect(c.Author).To(gomega.Equal("testuser"))
		})

		ginkgo.It("rebuilds the index after update", func() {
			tempID := m.addOptimisticThread("main.go", 10, 0, "RIGHT", review.Comment{
				Body: "original", Author: "testuser", IsPending: true,
			})

			m.updateCommentBody(tempID, "updated")

			indexed := m.comments.threadIndex[commentIndexKey("main.go", 10, "RIGHT")]
			gomega.Expect(indexed).To(gomega.HaveLen(1))
			gomega.Expect(indexed[0].Comments[0].Body).To(gomega.Equal("updated"))
		})

		ginkgo.It("is a no-op for non-existent ID", func() {
			m.addOptimisticThread("main.go", 10, 0, "RIGHT", review.Comment{
				Body: "original", Author: "testuser", IsPending: true,
			})

			m.updateCommentBody(9999, "should not happen")

			gomega.Expect(m.comments.threads).To(gomega.HaveLen(1))
			gomega.Expect(m.comments.threads[0].Comments[0].Body).To(gomega.Equal("original"))
		})
	})

	ginkgo.Describe("findCommentByID", func() {
		ginkgo.It("returns a pointer to the matching comment", func() {
			tempID := m.addOptimisticThread("a.go", 1, 0, "RIGHT", review.Comment{
				Body: "find me", Author: "testuser", IsPending: true,
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

	ginkgo.Describe("CommentPanel.RebuildIndex", func() {
		ginkgo.It("produces correct mappings from the thread list", func() {
			m.comments.threads = []review.Thread{
				{Path: "a.go", Line: 10, Side: "RIGHT", Comments: []review.Comment{
					{ID: 1, Body: "existing", Author: "alice"},
					{ID: 2, Body: "pending", Author: "testuser", IsPending: true},
				}},
				{Path: "b.go", Line: 5, Side: "LEFT", Comments: []review.Comment{
					{ID: 3, Body: "other file", Author: "bob"},
				}},
			}

			m.comments.RebuildIndex()

			aThreads := m.comments.threadIndex[commentIndexKey("a.go", 10, "RIGHT")]
			gomega.Expect(aThreads).To(gomega.HaveLen(1))
			gomega.Expect(aThreads[0].Comments).To(gomega.HaveLen(2))

			bThreads := m.comments.threadIndex[commentIndexKey("b.go", 5, "LEFT")]
			gomega.Expect(bThreads).To(gomega.HaveLen(1))

			gomega.Expect(m.comments.fileThreadIndex["a.go"]).To(gomega.BeTrue())
			gomega.Expect(m.comments.fileThreadIndex["b.go"]).To(gomega.BeTrue())
		})

		ginkgo.It("clears stale entries on rebuild", func() {
			m.addOptimisticThread("old.go", 1, 0, "RIGHT", review.Comment{
				Body: "stale", Author: "testuser", IsPending: true,
			})
			gomega.Expect(m.comments.fileThreadIndex["old.go"]).To(gomega.BeTrue())

			// Clear threads and rebuild
			m.comments.threads = nil
			m.comments.RebuildIndex()

			gomega.Expect(m.comments.threadIndex[commentIndexKey("old.go", 1, "RIGHT")]).To(gomega.BeEmpty())
			gomega.Expect(m.comments.fileThreadIndex["old.go"]).To(gomega.BeFalse())
		})
	})

	ginkgo.Describe("CommentsForLine", func() {
		ginkgo.It("returns exact side matches", func() {
			m.setThreads([]review.Thread{
				{Path: "f.go", Line: 10, Side: "RIGHT", Comments: []review.Comment{
					{ID: 1, Body: "right"},
				}},
				{Path: "f.go", Line: 10, Side: "LEFT", Comments: []review.Comment{
					{ID: 2, Body: "left"},
				}},
			})

			right := m.comments.CommentsForLine("f.go", 10, "RIGHT")
			gomega.Expect(right).To(gomega.HaveLen(1))
			gomega.Expect(right[0].Body).To(gomega.Equal("right"))

			left := m.comments.CommentsForLine("f.go", 10, "LEFT")
			gomega.Expect(left).To(gomega.HaveLen(1))
			gomega.Expect(left[0].Body).To(gomega.Equal("left"))
		})

		ginkgo.It("includes empty-side threads in any side query", func() {
			m.setThreads([]review.Thread{
				{Path: "f.go", Line: 10, Side: "", Comments: []review.Comment{
					{ID: 1, Body: "no side"},
				}},
				{Path: "f.go", Line: 10, Side: "RIGHT", Comments: []review.Comment{
					{ID: 2, Body: "right"},
				}},
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

	ginkgo.Describe("navigateThread with pending comments", func() {
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
			m.addOptimisticThread("main.go", 3, 0, "RIGHT", review.Comment{
				Body: "pending fix", Author: "testuser", IsPending: true,
			})

			m.navigateThread(true)

			gomega.Expect(m.nav.cursor.LineIdx).To(gomega.Equal(2))
			gomega.Expect(m.nav.cursor.Kind).To(gomega.Equal(CursorComment))
		})

		ginkgo.It("cycles backward to a pending comment", func() {
			// Cursor at end
			m.nav.cursor.LineIdx = 4

			m.addOptimisticThread("main.go", 3, 0, "RIGHT", review.Comment{
				Body: "pending fix", Author: "testuser", IsPending: true,
			})

			m.navigateThread(false)

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
			m.addOptimisticThread("other.go", 2, 0, "RIGHT", review.Comment{
				Body: "review this", Author: "testuser", IsPending: true,
			})

			// Cursor on first file, no comments here
			m.nav.cursor.FileIdx = 0
			m.nav.cursor.LineIdx = 0
			m.nav.buildDiffLines(m.files)

			m.navigateThread(true)

			// Should have jumped to the second file
			gomega.Expect(m.nav.cursor.FileIdx).To(gomega.Equal(1))
			gomega.Expect(m.nav.cursor.Kind).To(gomega.Equal(CursorComment))
		})
	})
})
