package review_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jkuan/pr-review/internal/review"
)

var _ = Describe("PendingReview", func() {
	var rev *review.PendingReview

	BeforeEach(func() {
		rev = review.NewPendingReview(42, "PR_node123", "abc123")
	})

	Describe("NewPendingReview", func() {
		It("initializes with correct defaults", func() {
			Expect(rev.PRNumber).To(Equal(42))
			Expect(rev.PRNodeID).To(Equal("PR_node123"))
			Expect(rev.CommitID).To(Equal("abc123"))
			Expect(rev.Comments).To(BeEmpty())
			Expect(rev.Event).To(Equal(review.ReviewEventComment))
			Expect(rev.ViewedFiles).NotTo(BeNil())
			Expect(rev.ReviewID).To(Equal(0))
		})
	})

	Describe("AddComment", func() {
		It("adds a single-line comment on the RIGHT side", func() {
			rev.AddComment("main.go", 10, "looks good")

			Expect(rev.Comments).To(HaveLen(1))
			c := rev.Comments[0]
			Expect(c.Path).To(Equal("main.go"))
			Expect(c.Line).To(Equal(10))
			Expect(c.Side).To(Equal("RIGHT"))
			Expect(c.Body).To(Equal("looks good"))
			Expect(c.StartLine).To(Equal(0))
		})

		It("assigns unique LocalIDs", func() {
			id1 := rev.AddComment("a.go", 1, "first")
			id2 := rev.AddComment("b.go", 2, "second")
			id3 := rev.AddComment("a.go", 5, "third")

			Expect(id1).NotTo(Equal(id2))
			Expect(id2).NotTo(Equal(id3))
			Expect(rev.Comments[0].LocalID).To(Equal(id1))
			Expect(rev.Comments[1].LocalID).To(Equal(id2))
			Expect(rev.Comments[2].LocalID).To(Equal(id3))
		})

		It("sets SyncStatus to SyncPending", func() {
			rev.AddComment("a.go", 1, "first")
			Expect(rev.Comments[0].SyncStatus).To(Equal(review.SyncPending))
		})

		It("accumulates multiple comments", func() {
			rev.AddComment("a.go", 1, "first")
			rev.AddComment("b.go", 2, "second")
			rev.AddComment("a.go", 5, "third")

			Expect(rev.Comments).To(HaveLen(3))
		})
	})

	Describe("AddMultiLineComment", func() {
		It("adds a comment spanning multiple lines", func() {
			rev.AddMultiLineComment("main.go", 5, 10, "refactor this block")

			Expect(rev.Comments).To(HaveLen(1))
			c := rev.Comments[0]
			Expect(c.StartLine).To(Equal(5))
			Expect(c.Line).To(Equal(10))
			Expect(c.Body).To(Equal("refactor this block"))
		})
	})

	Describe("AddSuggestion", func() {
		It("wraps the suggestion in a code fence", func() {
			rev.AddSuggestion("main.go", 10, "fmt.Println(\"fixed\")")

			Expect(rev.Comments).To(HaveLen(1))
			Expect(rev.Comments[0].Body).To(Equal("```suggestion\nfmt.Println(\"fixed\")\n```"))
		})
	})

	Describe("AddMultiLineSuggestion", func() {
		It("wraps multi-line suggestion in a code fence", func() {
			rev.AddMultiLineSuggestion("main.go", 5, 10, "line1\nline2")

			Expect(rev.Comments).To(HaveLen(1))
			c := rev.Comments[0]
			Expect(c.StartLine).To(Equal(5))
			Expect(c.Line).To(Equal(10))
			Expect(c.Body).To(Equal("```suggestion\nline1\nline2\n```"))
		})
	})

	Describe("RemoveComment", func() {
		BeforeEach(func() {
			rev.AddComment("a.go", 1, "first")
			rev.AddComment("b.go", 2, "second")
			rev.AddComment("c.go", 3, "third")
		})

		It("removes a comment by index", func() {
			rev.RemoveComment(1)

			Expect(rev.Comments).To(HaveLen(2))
			Expect(rev.Comments[0].Body).To(Equal("first"))
			Expect(rev.Comments[1].Body).To(Equal("third"))
		})

		It("removes the first comment", func() {
			rev.RemoveComment(0)

			Expect(rev.Comments).To(HaveLen(2))
			Expect(rev.Comments[0].Body).To(Equal("second"))
		})

		It("removes the last comment", func() {
			rev.RemoveComment(2)

			Expect(rev.Comments).To(HaveLen(2))
			Expect(rev.Comments[1].Body).To(Equal("second"))
		})

		It("does nothing for negative index", func() {
			rev.RemoveComment(-1)
			Expect(rev.Comments).To(HaveLen(3))
		})

		It("does nothing for out-of-bounds index", func() {
			rev.RemoveComment(10)
			Expect(rev.Comments).To(HaveLen(3))
		})
	})

	Describe("RemoveCommentByLocalID", func() {
		BeforeEach(func() {
			rev.AddComment("a.go", 1, "first")
			rev.AddComment("b.go", 2, "second")
			rev.AddComment("c.go", 3, "third")
		})

		It("removes a comment by LocalID", func() {
			localID := rev.Comments[1].LocalID
			rev.RemoveCommentByLocalID(localID)

			Expect(rev.Comments).To(HaveLen(2))
			Expect(rev.Comments[0].Body).To(Equal("first"))
			Expect(rev.Comments[1].Body).To(Equal("third"))
		})

		It("returns the ForgeID of the removed comment", func() {
			rev.Comments[1].ForgeID = 999
			localID := rev.Comments[1].LocalID
			forgeID := rev.RemoveCommentByLocalID(localID)
			Expect(forgeID).To(Equal(999))
		})

		It("returns 0 for unsynced comment", func() {
			localID := rev.Comments[0].LocalID
			forgeID := rev.RemoveCommentByLocalID(localID)
			Expect(forgeID).To(Equal(0))
		})

		It("does nothing for unknown LocalID", func() {
			forgeID := rev.RemoveCommentByLocalID(9999)
			Expect(forgeID).To(Equal(0))
			Expect(rev.Comments).To(HaveLen(3))
		})
	})

	Describe("FindByLocalID", func() {
		It("finds an existing comment", func() {
			id := rev.AddComment("a.go", 1, "hello")
			c := rev.FindByLocalID(id)
			Expect(c).NotTo(BeNil())
			Expect(c.Body).To(Equal("hello"))
		})

		It("returns nil for unknown ID", func() {
			Expect(rev.FindByLocalID(9999)).To(BeNil())
		})
	})

	Describe("InFlightCount", func() {
		It("returns 0 when no comments", func() {
			Expect(rev.InFlightCount()).To(Equal(0))
		})

		It("counts only in-flight comments", func() {
			rev.AddComment("a.go", 1, "first")
			rev.AddComment("b.go", 2, "second")
			rev.AddComment("c.go", 3, "third")

			rev.Comments[0].SyncStatus = review.SyncInFlight
			rev.Comments[1].SyncStatus = review.SyncComplete
			rev.Comments[2].SyncStatus = review.SyncInFlight

			Expect(rev.InFlightCount()).To(Equal(2))
		})
	})

	Describe("SetSyncStatus", func() {
		It("updates sync status and ForgeID", func() {
			id := rev.AddComment("a.go", 1, "hello")
			rev.SetSyncStatus(id, review.SyncComplete, 42, nil)

			c := rev.FindByLocalID(id)
			Expect(c.SyncStatus).To(Equal(review.SyncComplete))
			Expect(c.ForgeID).To(Equal(42))
			Expect(c.SyncError).To(BeNil())
		})

		It("records sync errors", func() {
			id := rev.AddComment("a.go", 1, "hello")
			err := fmt.Errorf("network error")
			rev.SetSyncStatus(id, review.SyncFailed, 0, err)

			c := rev.FindByLocalID(id)
			Expect(c.SyncStatus).To(Equal(review.SyncFailed))
			Expect(c.SyncError).To(MatchError("network error"))
		})
	})

	Describe("UnsyncedComments", func() {
		It("returns pending and failed comments", func() {
			rev.AddComment("a.go", 1, "pending")
			rev.AddComment("b.go", 2, "inflight")
			rev.AddComment("c.go", 3, "complete")
			rev.AddComment("d.go", 4, "failed")

			rev.Comments[1].SyncStatus = review.SyncInFlight
			rev.Comments[2].SyncStatus = review.SyncComplete
			rev.Comments[3].SyncStatus = review.SyncFailed

			unsynced := rev.UnsyncedComments()
			Expect(unsynced).To(HaveLen(2))
			Expect(unsynced[0].Body).To(Equal("pending"))
			Expect(unsynced[1].Body).To(Equal("failed"))
		})
	})

	Describe("AddCommentDirect", func() {
		It("assigns a LocalID to the comment", func() {
			c := review.InlineComment{
				Path:       "x.go",
				Line:       5,
				Side:       "RIGHT",
				Body:       "direct",
				ForgeID:   100,
				SyncStatus: review.SyncComplete,
			}
			id := rev.AddCommentDirect(c)

			Expect(rev.Comments).To(HaveLen(1))
			Expect(rev.Comments[0].LocalID).To(Equal(id))
			Expect(rev.Comments[0].ForgeID).To(Equal(100))
			Expect(rev.Comments[0].SyncStatus).To(Equal(review.SyncComplete))
		})
	})
})

var _ = Describe("PRFilter", func() {
	It("has correct string representations", func() {
		Expect(review.FilterReviewRequested.String()).To(Equal("Needs My Review"))
		Expect(review.FilterAllOpen.String()).To(Equal("All Open"))
		Expect(review.FilterAuthored.String()).To(Equal("Authored by Me"))
	})
})
