package reviewtest_test

import (
	"context"
	"errors"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jethrokuan/pry/internal/diff"
	"github.com/jethrokuan/pry/internal/review"
	"github.com/jethrokuan/pry/internal/review/reviewtest"
)

func TestReviewtest(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Reviewtest Suite")
}

var _ = Describe("MockService", func() {
	var svc review.Service

	BeforeEach(func() {
		svc = &reviewtest.MockService{
			Owner: "test-org",
			Repo:  "test-repo",
		}
	})

	It("implements review.Service", func() {
		// Compile-time check is implicit via the typed variable above.
		Expect(svc).NotTo(BeNil())
	})

	It("returns configured owner and repo", func() {
		Expect(svc.RepoOwner()).To(Equal("test-org"))
		Expect(svc.RepoName()).To(Equal("test-repo"))
	})

	It("returns zero values when no Fn is set", func() {
		ctx := context.Background()

		user, err := svc.CurrentUser(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(user).To(BeEmpty())

		prs, err := svc.ListPRs(ctx, review.PRFilter{})
		Expect(err).NotTo(HaveOccurred())
		Expect(prs).To(BeNil())

		files, err := svc.FetchDiffFiles(ctx, 1)
		Expect(err).NotTo(HaveOccurred())
		Expect(files).To(BeNil())
	})

	It("delegates to Fn fields when set", func() {
		ctx := context.Background()
		mock := svc.(*reviewtest.MockService)

		mock.CurrentUserFn = func(ctx context.Context) (string, error) {
			return "octocat", nil
		}
		user, err := svc.CurrentUser(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(user).To(Equal("octocat"))

		mock.ListPRsFn = func(ctx context.Context, filter review.PRFilter) ([]review.PullRequest, error) {
			return []review.PullRequest{{Number: 42, Title: "Test PR"}}, nil
		}
		prs, err := svc.ListPRs(ctx, review.PRFilter{})
		Expect(err).NotTo(HaveOccurred())
		Expect(prs).To(HaveLen(1))
		Expect(prs[0].Number).To(Equal(42))
	})

	It("can simulate errors", func() {
		ctx := context.Background()
		mock := svc.(*reviewtest.MockService)
		testErr := errors.New("API rate limited")

		mock.FetchDiffFilesFn = func(ctx context.Context, number int) ([]diff.DiffFile, error) {
			return nil, testErr
		}
		_, err := svc.FetchDiffFiles(ctx, 1)
		Expect(err).To(MatchError("API rate limited"))
	})

	It("passes arguments through to Fn fields", func() {
		ctx := context.Background()
		mock := svc.(*reviewtest.MockService)

		var capturedNumber int
		mock.GetPRFn = func(ctx context.Context, number int) (*review.PullRequest, error) {
			capturedNumber = number
			return &review.PullRequest{Number: number}, nil
		}
		pr, err := svc.GetPR(ctx, 99)
		Expect(err).NotTo(HaveOccurred())
		Expect(pr.Number).To(Equal(99))
		Expect(capturedNumber).To(Equal(99))
	})
})
