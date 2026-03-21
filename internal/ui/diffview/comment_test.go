package diffview

import (
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/jethrokuan/pry/internal/review"
)

var _ = ginkgo.Describe("Comment CRUD state management", func() {
	var m Model

	ginkgo.BeforeEach(func() {
		m = Model{
			pr: &review.PullRequest{
				PendingReview: &review.PendingReview{
					ViewedFiles: make(map[string]bool),
				},
			},
			comments: CommentPanel{
				expanded:          make(map[string]bool),
				existingIndex:     make(map[string][]review.ExistingComment),
				localPendingIndex: make(map[string][]review.InlineComment),
				fileCommentIndex:  make(map[string]bool),
			},
		}
	})

	ginkgo.Describe("addLocalComment", func() {
		ginkgo.It("adds a comment and updates the local pending index", func() {
			id := m.addLocalComment(review.InlineComment{
				Path: "main.go", Line: 10, Side: "RIGHT", Body: "fix this",
			})
			gomega.Expect(id).To(gomega.BeNumerically(">", 0))
			gomega.Expect(m.pr.PendingReview.Comments).To(gomega.HaveLen(1))
			gomega.Expect(m.pr.PendingReview.Comments[0].LocalID).To(gomega.Equal(id))
			gomega.Expect(m.pr.PendingReview.Comments[0].Body).To(gomega.Equal("fix this"))

			pending := m.comments.localPendingIndex[commentIndexKey("main.go", 10, "RIGHT")]
			gomega.Expect(pending).To(gomega.HaveLen(1))
			gomega.Expect(pending[0].LocalID).To(gomega.Equal(id))
		})

		ginkgo.It("assigns unique IDs to multiple comments", func() {
			id1 := m.addLocalComment(review.InlineComment{
				Path: "a.go", Line: 1, Side: "RIGHT", Body: "first",
			})
			id2 := m.addLocalComment(review.InlineComment{
				Path: "a.go", Line: 1, Side: "RIGHT", Body: "second",
			})
			gomega.Expect(id1).NotTo(gomega.Equal(id2))
			gomega.Expect(m.pr.PendingReview.Comments).To(gomega.HaveLen(2))

			pending := m.comments.localPendingIndex[commentIndexKey("a.go", 1, "RIGHT")]
			gomega.Expect(pending).To(gomega.HaveLen(2))
		})

		ginkgo.It("indexes comments on different files separately", func() {
			m.addLocalComment(review.InlineComment{
				Path: "a.go", Line: 5, Side: "RIGHT", Body: "comment a",
			})
			m.addLocalComment(review.InlineComment{
				Path: "b.go", Line: 5, Side: "RIGHT", Body: "comment b",
			})

			gomega.Expect(m.comments.localPendingIndex[commentIndexKey("a.go", 5, "RIGHT")]).To(gomega.HaveLen(1))
			gomega.Expect(m.comments.localPendingIndex[commentIndexKey("b.go", 5, "RIGHT")]).To(gomega.HaveLen(1))
			gomega.Expect(m.comments.fileCommentIndex["a.go"]).To(gomega.BeTrue())
			gomega.Expect(m.comments.fileCommentIndex["b.go"]).To(gomega.BeTrue())
		})
	})

	ginkgo.Describe("removeLocalComment", func() {
		ginkgo.It("removes a comment and clears it from the index", func() {
			id := m.addLocalComment(review.InlineComment{
				Path: "main.go", Line: 10, Side: "RIGHT", Body: "fix this",
			})

			forgeID := m.removeLocalComment(id)
			gomega.Expect(forgeID).To(gomega.Equal(0)) // not synced
			gomega.Expect(m.pr.PendingReview.Comments).To(gomega.BeEmpty())

			pending := m.comments.localPendingIndex[commentIndexKey("main.go", 10, "RIGHT")]
			gomega.Expect(pending).To(gomega.BeEmpty())
		})

		ginkgo.It("returns the ForgeID of a synced comment", func() {
			id := m.addLocalComment(review.InlineComment{
				Path: "main.go", Line: 10, Side: "RIGHT", Body: "synced",
				ForgeID: 42, SyncStatus: review.SyncComplete,
			})

			forgeID := m.removeLocalComment(id)
			gomega.Expect(forgeID).To(gomega.Equal(42))
		})

		ginkgo.It("removes only the targeted comment, leaving others intact", func() {
			id1 := m.addLocalComment(review.InlineComment{
				Path: "main.go", Line: 10, Side: "RIGHT", Body: "first",
			})
			id2 := m.addLocalComment(review.InlineComment{
				Path: "main.go", Line: 10, Side: "RIGHT", Body: "second",
			})

			m.removeLocalComment(id1)
			gomega.Expect(m.pr.PendingReview.Comments).To(gomega.HaveLen(1))
			gomega.Expect(m.pr.PendingReview.Comments[0].LocalID).To(gomega.Equal(id2))

			pending := m.comments.localPendingIndex[commentIndexKey("main.go", 10, "RIGHT")]
			gomega.Expect(pending).To(gomega.HaveLen(1))
			gomega.Expect(pending[0].LocalID).To(gomega.Equal(id2))
		})

		ginkgo.It("clears fileCommentIndex when last comment on a file is removed", func() {
			id := m.addLocalComment(review.InlineComment{
				Path: "only.go", Line: 1, Side: "RIGHT", Body: "lone comment",
			})
			gomega.Expect(m.comments.fileCommentIndex["only.go"]).To(gomega.BeTrue())

			m.removeLocalComment(id)
			gomega.Expect(m.comments.fileCommentIndex["only.go"]).To(gomega.BeFalse())
		})
	})

	ginkgo.Describe("updateLocalComment", func() {
		ginkgo.It("updates the comment body while preserving position", func() {
			id := m.addLocalComment(review.InlineComment{
				Path: "main.go", Line: 10, Side: "RIGHT", Body: "original",
			})

			m.updateLocalComment(id, func(c *review.InlineComment) {
				c.Body = "updated"
			})

			c := m.pr.PendingReview.FindByLocalID(id)
			gomega.Expect(c).NotTo(gomega.BeNil())
			gomega.Expect(c.Body).To(gomega.Equal("updated"))
			gomega.Expect(c.Path).To(gomega.Equal("main.go"))
			gomega.Expect(c.Line).To(gomega.Equal(10))
			gomega.Expect(c.Side).To(gomega.Equal("RIGHT"))
		})

		ginkgo.It("preserves index after update", func() {
			id := m.addLocalComment(review.InlineComment{
				Path: "main.go", Line: 10, Side: "RIGHT", Body: "original",
			})

			m.updateLocalComment(id, func(c *review.InlineComment) {
				c.Body = "updated"
			})

			pending := m.comments.localPendingIndex[commentIndexKey("main.go", 10, "RIGHT")]
			gomega.Expect(pending).To(gomega.HaveLen(1))
			gomega.Expect(pending[0].Body).To(gomega.Equal("updated"))
		})

		ginkgo.It("is a no-op for non-existent LocalID", func() {
			m.addLocalComment(review.InlineComment{
				Path: "main.go", Line: 10, Side: "RIGHT", Body: "original",
			})

			m.updateLocalComment(9999, func(c *review.InlineComment) {
				c.Body = "should not happen"
			})

			gomega.Expect(m.pr.PendingReview.Comments).To(gomega.HaveLen(1))
			gomega.Expect(m.pr.PendingReview.Comments[0].Body).To(gomega.Equal("original"))
		})
	})

	ginkgo.Describe("setExistingComments", func() {
		ginkgo.It("indexes existing comments by path/line/side", func() {
			m.setExistingComments([]review.ExistingComment{
				{ID: 1, Path: "main.go", Line: 5, Side: "RIGHT", Body: "looks good", Author: "alice"},
				{ID: 2, Path: "main.go", Line: 5, Side: "RIGHT", Body: "agreed", Author: "bob"},
				{ID: 3, Path: "util.go", Line: 12, Side: "LEFT", Body: "why?", Author: "carol"},
			})

			gomega.Expect(m.comments.existingIndex[commentIndexKey("main.go", 5, "RIGHT")]).To(gomega.HaveLen(2))
			gomega.Expect(m.comments.existingIndex[commentIndexKey("util.go", 12, "LEFT")]).To(gomega.HaveLen(1))
			gomega.Expect(m.comments.fileCommentIndex["main.go"]).To(gomega.BeTrue())
			gomega.Expect(m.comments.fileCommentIndex["util.go"]).To(gomega.BeTrue())
		})
	})

	ginkgo.Describe("setForgeComments", func() {
		ginkgo.It("indexes forge comments into the existing index", func() {
			m.setForgeComments([]review.ExistingComment{
				{ID: 100, Path: "api.go", Line: 20, Side: "RIGHT", Body: "draft note", Author: "me", IsPending: true},
			})

			gomega.Expect(m.comments.existingIndex[commentIndexKey("api.go", 20, "RIGHT")]).To(gomega.HaveLen(1))
			gomega.Expect(m.comments.fileCommentIndex["api.go"]).To(gomega.BeTrue())
		})

		ginkgo.It("merges with existing comments in the index", func() {
			m.setExistingComments([]review.ExistingComment{
				{ID: 1, Path: "api.go", Line: 20, Side: "RIGHT", Body: "existing", Author: "alice"},
			})
			m.setForgeComments([]review.ExistingComment{
				{ID: 100, Path: "api.go", Line: 20, Side: "RIGHT", Body: "forge", Author: "me"},
			})

			gomega.Expect(m.comments.existingIndex[commentIndexKey("api.go", 20, "RIGHT")]).To(gomega.HaveLen(2))
		})
	})

	ginkgo.Describe("rebuildCommentIndex", func() {
		ginkgo.It("produces correct mappings from all comment sources", func() {
			// Set up existing comments
			m.comments.existing = []review.ExistingComment{
				{ID: 1, Path: "a.go", Line: 10, Side: "RIGHT", Body: "existing"},
			}
			// Set up forge comments
			m.comments.forgeComments = []review.ExistingComment{
				{ID: 2, Path: "a.go", Line: 10, Side: "RIGHT", Body: "forge"},
				{ID: 3, Path: "b.go", Line: 5, Side: "LEFT", Body: "forge b"},
			}
			// Set up local pending comments
			m.pr.PendingReview.AddCommentDirect(review.InlineComment{
				Path: "a.go", Line: 10, Side: "RIGHT", Body: "local pending",
			})

			m.rebuildCommentIndex()

			// Existing + forge on same key
			existing := m.comments.existingIndex[commentIndexKey("a.go", 10, "RIGHT")]
			gomega.Expect(existing).To(gomega.HaveLen(2))

			// Forge on different key
			forgeB := m.comments.existingIndex[commentIndexKey("b.go", 5, "LEFT")]
			gomega.Expect(forgeB).To(gomega.HaveLen(1))

			// Local pending
			local := m.comments.localPendingIndex[commentIndexKey("a.go", 10, "RIGHT")]
			gomega.Expect(local).To(gomega.HaveLen(1))

			// fileCommentIndex covers all files
			gomega.Expect(m.comments.fileCommentIndex["a.go"]).To(gomega.BeTrue())
			gomega.Expect(m.comments.fileCommentIndex["b.go"]).To(gomega.BeTrue())
		})

		ginkgo.It("clears stale entries on rebuild", func() {
			m.addLocalComment(review.InlineComment{
				Path: "old.go", Line: 1, Side: "RIGHT", Body: "stale",
			})
			gomega.Expect(m.comments.fileCommentIndex["old.go"]).To(gomega.BeTrue())

			// Remove via the review directly, then rebuild
			m.pr.PendingReview.Comments = nil
			m.rebuildCommentIndex()

			gomega.Expect(m.comments.localPendingIndex[commentIndexKey("old.go", 1, "RIGHT")]).To(gomega.BeEmpty())
			gomega.Expect(m.comments.fileCommentIndex["old.go"]).To(gomega.BeFalse())
		})
	})

	ginkgo.Describe("restoreForgeComments", func() {
		ginkgo.It("restores pending and forge comments into correct indexes", func() {
			pending := []review.ExistingComment{
				{ID: 50, Path: "a.go", Line: 10, Side: "RIGHT", Body: "restored pending"},
			}
			forge := []review.ExistingComment{
				{ID: 51, Path: "b.go", Line: 20, Side: "LEFT", Body: "forge comment"},
			}

			m.restoreForgeComments(pending, forge)

			// Pending comments are added as local comments
			gomega.Expect(m.pr.PendingReview.Comments).To(gomega.HaveLen(1))
			gomega.Expect(m.pr.PendingReview.Comments[0].ForgeID).To(gomega.Equal(50))
			gomega.Expect(m.pr.PendingReview.Comments[0].SyncStatus).To(gomega.Equal(review.SyncComplete))

			local := m.comments.localPendingIndex[commentIndexKey("a.go", 10, "RIGHT")]
			gomega.Expect(local).To(gomega.HaveLen(1))

			// Forge comments in existing index
			existingB := m.comments.existingIndex[commentIndexKey("b.go", 20, "LEFT")]
			gomega.Expect(existingB).To(gomega.HaveLen(1))

			gomega.Expect(m.comments.fileCommentIndex["a.go"]).To(gomega.BeTrue())
			gomega.Expect(m.comments.fileCommentIndex["b.go"]).To(gomega.BeTrue())
		})
	})

	ginkgo.Describe("commentsForLine", func() {
		ginkgo.It("returns exact side matches", func() {
			m.setExistingComments([]review.ExistingComment{
				{ID: 1, Path: "f.go", Line: 10, Side: "RIGHT", Body: "right"},
				{ID: 2, Path: "f.go", Line: 10, Side: "LEFT", Body: "left"},
			})

			right := m.commentsForLine("f.go", 10, "RIGHT")
			gomega.Expect(right).To(gomega.HaveLen(1))
			gomega.Expect(right[0].Body).To(gomega.Equal("right"))

			left := m.commentsForLine("f.go", 10, "LEFT")
			gomega.Expect(left).To(gomega.HaveLen(1))
			gomega.Expect(left[0].Body).To(gomega.Equal("left"))
		})

		ginkgo.It("includes empty-side comments in any side query", func() {
			m.setExistingComments([]review.ExistingComment{
				{ID: 1, Path: "f.go", Line: 10, Side: "", Body: "no side"},
				{ID: 2, Path: "f.go", Line: 10, Side: "RIGHT", Body: "right"},
			})

			right := m.commentsForLine("f.go", 10, "RIGHT")
			gomega.Expect(right).To(gomega.HaveLen(2))
		})

		ginkgo.It("returns nil for lines with no comments", func() {
			result := m.commentsForLine("f.go", 999, "RIGHT")
			gomega.Expect(result).To(gomega.BeEmpty())
		})
	})

	ginkgo.Describe("localPendingForLine", func() {
		ginkgo.It("returns local pending comments for a specific line", func() {
			m.addLocalComment(review.InlineComment{
				Path: "f.go", Line: 10, Side: "RIGHT", Body: "pending",
			})
			m.addLocalComment(review.InlineComment{
				Path: "f.go", Line: 20, Side: "RIGHT", Body: "other line",
			})

			result := m.localPendingForLine("f.go", 10, "RIGHT")
			gomega.Expect(result).To(gomega.HaveLen(1))
			gomega.Expect(result[0].Body).To(gomega.Equal("pending"))
		})
	})

	ginkgo.Describe("commentKey and commentIndexKey", func() {
		ginkgo.It("produces expected formats", func() {
			gomega.Expect(commentKey("path/file.go", 42)).To(gomega.Equal("path/file.go:42"))
			gomega.Expect(commentIndexKey("path/file.go", 42, "RIGHT")).To(gomega.Equal("path/file.go:42:RIGHT"))
		})
	})
})
