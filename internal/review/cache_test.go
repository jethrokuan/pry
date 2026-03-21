package review_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jethrokuan/pry/internal/review"
	"github.com/jethrokuan/pry/internal/review/reviewtest"
)

var _ = Describe("CachingService", func() {
	var (
		mock  *reviewtest.MockService
		cache *review.CachingService
		ctx   context.Context
		calls int
	)

	BeforeEach(func() {
		ctx = context.Background()
		calls = 0
		mock = &reviewtest.MockService{
			ListPRsFn: func(_ context.Context, filter review.PRFilter) ([]review.PullRequest, error) {
				calls++
				return []review.PullRequest{
					{Number: calls, Title: fmt.Sprintf("PR %d", calls)},
				}, nil
			},
		}
	})

	Describe("with caching enabled", func() {
		BeforeEach(func() {
			cache = review.NewCachingService(mock, 5*time.Minute)
		})

		It("returns cached results on second call with same filter", func() {
			filter := review.PRFilter{Name: "Test", Qualifier: "author:@me"}

			prs1, err := cache.ListPRs(ctx, filter)
			Expect(err).NotTo(HaveOccurred())
			Expect(prs1).To(HaveLen(1))
			Expect(prs1[0].Number).To(Equal(1))

			prs2, err := cache.ListPRs(ctx, filter)
			Expect(err).NotTo(HaveOccurred())
			Expect(prs2).To(HaveLen(1))
			Expect(prs2[0].Number).To(Equal(1)) // same cached result

			Expect(calls).To(Equal(1)) // inner service called only once
		})

		It("caches different filters independently", func() {
			f1 := review.PRFilter{Name: "A", Qualifier: "author:@me"}
			f2 := review.PRFilter{Name: "B", Qualifier: "review-requested:@me"}

			_, err := cache.ListPRs(ctx, f1)
			Expect(err).NotTo(HaveOccurred())
			_, err = cache.ListPRs(ctx, f2)
			Expect(err).NotTo(HaveOccurred())

			Expect(calls).To(Equal(2))

			// Both should be cached now
			_, _ = cache.ListPRs(ctx, f1)
			_, _ = cache.ListPRs(ctx, f2)
			Expect(calls).To(Equal(2))
		})

		It("invalidates all cached entries", func() {
			filter := review.PRFilter{Name: "Test", Qualifier: "author:@me"}

			_, err := cache.ListPRs(ctx, filter)
			Expect(err).NotTo(HaveOccurred())
			Expect(calls).To(Equal(1))

			cache.InvalidateListPRs()

			_, err = cache.ListPRs(ctx, filter)
			Expect(err).NotTo(HaveOccurred())
			Expect(calls).To(Equal(2))
		})

		It("does not cache errors", func() {
			errMock := &reviewtest.MockService{
				ListPRsFn: func(_ context.Context, _ review.PRFilter) ([]review.PullRequest, error) {
					calls++
					if calls == 1 {
						return nil, fmt.Errorf("network error")
					}
					return []review.PullRequest{{Number: 1}}, nil
				},
			}
			c := review.NewCachingService(errMock, 5*time.Minute)
			filter := review.PRFilter{Name: "Test", Qualifier: "q"}

			_, err := c.ListPRs(ctx, filter)
			Expect(err).To(HaveOccurred())

			prs, err := c.ListPRs(ctx, filter)
			Expect(err).NotTo(HaveOccurred())
			Expect(prs).To(HaveLen(1))
			Expect(calls).To(Equal(2))
		})

		It("returns a copy so callers cannot mutate the cache", func() {
			filter := review.PRFilter{Name: "Test", Qualifier: "q"}

			prs1, _ := cache.ListPRs(ctx, filter)
			prs1[0].Title = "mutated"

			prs2, _ := cache.ListPRs(ctx, filter)
			Expect(prs2[0].Title).To(Equal("PR 1")) // original, not mutated
		})
	})

	Describe("with caching disabled (ttl=0)", func() {
		BeforeEach(func() {
			cache = review.NewCachingService(mock, 0)
		})

		It("passes through every call to inner service", func() {
			filter := review.PRFilter{Name: "Test", Qualifier: "q"}

			_, _ = cache.ListPRs(ctx, filter)
			_, _ = cache.ListPRs(ctx, filter)
			_, _ = cache.ListPRs(ctx, filter)

			Expect(calls).To(Equal(3))
		})
	})

	Describe("CacheInvalidator interface", func() {
		It("is implemented by CachingService", func() {
			c := review.NewCachingService(mock, time.Minute)
			var inv review.CacheInvalidator = c
			Expect(inv).NotTo(BeNil())
		})
	})

	Describe("delegated methods", func() {
		It("passes RepoOwner through to inner service", func() {
			mock.Owner = "myorg"
			c := review.NewCachingService(mock, time.Minute)
			Expect(c.RepoOwner()).To(Equal("myorg"))
		})

		It("passes RepoName through to inner service", func() {
			mock.Repo = "myrepo"
			c := review.NewCachingService(mock, time.Minute)
			Expect(c.RepoName()).To(Equal("myrepo"))
		})
	})
})
