package review_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jethrokuan/pry/internal/review"
)

var _ = Describe("PendingReview", func() {
	var rev *review.PendingReview

	BeforeEach(func() {
		rev = review.NewPendingReview()
	})

	Describe("NewPendingReview", func() {
		It("initializes with correct defaults", func() {
			Expect(rev.Event).To(Equal(review.ReviewEventComment))
			Expect(rev.ViewedFiles).NotTo(BeNil())
			Expect(rev.ViewedFiles).To(BeEmpty())
			Expect(rev.ReviewID).To(Equal(0))
			Expect(rev.ReviewNodeID).To(BeEmpty())
			Expect(rev.Body).To(BeEmpty())
		})
	})

	Describe("NextTempID", func() {
		It("returns -1 on first call", func() {
			Expect(rev.NextTempID()).To(Equal(-1))
		})

		It("returns decrementing negative IDs on successive calls", func() {
			id1 := rev.NextTempID()
			id2 := rev.NextTempID()
			id3 := rev.NextTempID()

			Expect(id1).To(Equal(-1))
			Expect(id2).To(Equal(-2))
			Expect(id3).To(Equal(-3))
		})

		It("always returns negative IDs", func() {
			for i := 0; i < 100; i++ {
				Expect(rev.NextTempID()).To(BeNumerically("<", 0))
			}
		})
	})
})

var _ = Describe("PullRequest", func() {
	Describe("StartReview", func() {
		It("creates a new PendingReview on the PR", func() {
			pr := &review.PullRequest{Number: 42}
			Expect(pr.PendingReview).To(BeNil())

			result := pr.StartReview()

			Expect(pr.PendingReview).NotTo(BeNil())
			Expect(pr.PendingReview).To(BeIdenticalTo(result))
			Expect(result.Event).To(Equal(review.ReviewEventComment))
			Expect(result.ViewedFiles).NotTo(BeNil())
		})

		It("replaces any existing PendingReview", func() {
			pr := &review.PullRequest{Number: 42}
			first := pr.StartReview()
			first.Body = "old review"

			second := pr.StartReview()

			Expect(pr.PendingReview).To(BeIdenticalTo(second))
			Expect(second.Body).To(BeEmpty())
		})
	})

	Describe("MergeEnriched", func() {
		It("replaces basic fields from enriched", func() {
			pr := &review.PullRequest{
				Number: 1,
				Title:  "old title",
			}
			enriched := &review.PullRequest{
				Number:    1,
				Title:     "new title",
				Additions: 50,
				Author:    "alice",
			}

			pr.MergeEnriched(enriched)

			Expect(pr.Title).To(Equal("new title"))
			Expect(pr.Additions).To(Equal(50))
			Expect(pr.Author).To(Equal("alice"))
		})

		It("preserves existing PendingReview", func() {
			pendingReview := review.NewPendingReview()
			pendingReview.Body = "my review"

			pr := &review.PullRequest{
				Number:        1,
				PendingReview: pendingReview,
			}
			enriched := &review.PullRequest{
				Number: 1,
				Title:  "updated",
			}

			pr.MergeEnriched(enriched)

			Expect(pr.Title).To(Equal("updated"))
			Expect(pr.PendingReview).To(BeIdenticalTo(pendingReview))
			Expect(pr.PendingReview.Body).To(Equal("my review"))
		})

		It("preserves existing Threads", func() {
			threads := []review.Thread{
				{Path: "a.go", Line: 10, Side: "RIGHT", Comments: []review.Comment{
					{ID: 1, Body: "existing comment"},
				}},
			}

			pr := &review.PullRequest{
				Number:  1,
				Threads: threads,
			}
			enriched := &review.PullRequest{
				Number: 1,
				Title:  "updated",
			}

			pr.MergeEnriched(enriched)

			Expect(pr.Title).To(Equal("updated"))
			Expect(pr.Threads).To(HaveLen(1))
			Expect(pr.Threads[0].Comments[0].Body).To(Equal("existing comment"))
		})

		It("does not preserve nil PendingReview", func() {
			pr := &review.PullRequest{
				Number:        1,
				PendingReview: nil,
			}
			enrichedReview := review.NewPendingReview()
			enriched := &review.PullRequest{
				Number:        1,
				PendingReview: enrichedReview,
			}

			pr.MergeEnriched(enriched)

			Expect(pr.PendingReview).To(BeIdenticalTo(enrichedReview))
		})

		It("does not preserve nil Threads", func() {
			pr := &review.PullRequest{
				Number:  1,
				Threads: nil,
			}
			enrichedThreads := []review.Thread{
				{Path: "a.go", Line: 1, Side: "RIGHT", Comments: []review.Comment{
					{ID: 5, Body: "from enriched"},
				}},
			}
			enriched := &review.PullRequest{
				Number:  1,
				Threads: enrichedThreads,
			}

			pr.MergeEnriched(enriched)

			Expect(pr.Threads).To(HaveLen(1))
			Expect(pr.Threads[0].Comments[0].Body).To(Equal("from enriched"))
		})
	})
})
