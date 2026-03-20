package testutil_test

import (
	"context"
	"errors"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/jkuan/pr-review/internal/diff"
	"github.com/jkuan/pr-review/internal/review"
	"github.com/jkuan/pr-review/internal/testutil"
)

var _ = ginkgo.Describe("MockService", func() {
	ginkgo.It("implements review.Service", func() {
		var svc review.Service = &testutil.MockService{}
		gomega.Expect(svc).NotTo(gomega.BeNil())
	})

	ginkgo.It("returns defaults when no functions are set", func() {
		svc := &testutil.MockService{}
		gomega.Expect(svc.RepoOwner()).To(gomega.Equal("test-owner"))
		gomega.Expect(svc.RepoName()).To(gomega.Equal("test-repo"))

		user, err := svc.CurrentUser(context.Background())
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(user).To(gomega.Equal("test-user"))

		files, err := svc.FetchDiffFiles(context.Background(), 1)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(files).To(gomega.BeNil())
	})

	ginkgo.It("delegates to custom functions when set", func() {
		expectedFiles := []diff.DiffFile{{Path: "foo.go"}}
		svc := &testutil.MockService{
			FetchDiffFilesFn: func(_ context.Context, number int) ([]diff.DiffFile, error) {
				if number == 42 {
					return expectedFiles, nil
				}
				return nil, errors.New("not found")
			},
		}

		files, err := svc.FetchDiffFiles(context.Background(), 42)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(files).To(gomega.Equal(expectedFiles))

		_, err = svc.FetchDiffFiles(context.Background(), 99)
		gomega.Expect(err).To(gomega.MatchError("not found"))
	})
})

var _ = ginkgo.Describe("PRBuilder", func() {
	ginkgo.It("builds a PR with defaults", func() {
		pr := testutil.NewPR().Build()
		gomega.Expect(pr.Number).To(gomega.Equal(42))
		gomega.Expect(pr.Title).To(gomega.Equal("Test PR"))
		gomega.Expect(pr.Author).To(gomega.Equal("test-user"))
		gomega.Expect(pr.Branch).To(gomega.Equal("feature-branch"))
		gomega.Expect(pr.Base).To(gomega.Equal("main"))
		gomega.Expect(pr.State).To(gomega.Equal("OPEN"))
		gomega.Expect(pr.HeadSHA).To(gomega.Equal("abc123"))
	})

	ginkgo.It("overrides fields via builder methods", func() {
		pr := testutil.NewPR().
			Number(99).
			Title("Custom PR").
			Author("alice").
			Draft(true).
			Labels("bug", "urgent").
			Changes(10, 5, 3).
			ReviewDecision("APPROVED").
			Build()

		gomega.Expect(pr.Number).To(gomega.Equal(99))
		gomega.Expect(pr.Title).To(gomega.Equal("Custom PR"))
		gomega.Expect(pr.Author).To(gomega.Equal("alice"))
		gomega.Expect(pr.Draft).To(gomega.BeTrue())
		gomega.Expect(pr.Labels).To(gomega.Equal([]string{"bug", "urgent"}))
		gomega.Expect(pr.Additions).To(gomega.Equal(10))
		gomega.Expect(pr.Deletions).To(gomega.Equal(5))
		gomega.Expect(pr.Files).To(gomega.Equal(3))
		gomega.Expect(pr.ReviewDecision).To(gomega.Equal("APPROVED"))
	})
})

var _ = ginkgo.Describe("SimpleDiffFile", func() {
	ginkgo.It("creates a file with added lines", func() {
		f := testutil.SimpleDiffFile("main.go", "package main", "func main() {}")
		gomega.Expect(f.Path).To(gomega.Equal("main.go"))
		gomega.Expect(f.Status).To(gomega.Equal(diff.StatusModified))
		gomega.Expect(f.Hunks).To(gomega.HaveLen(1))
		gomega.Expect(f.Hunks[0].Lines).To(gomega.HaveLen(2))
		gomega.Expect(f.Hunks[0].Lines[0].Type).To(gomega.Equal(diff.LineAddition))
		gomega.Expect(f.Hunks[0].Lines[0].Content).To(gomega.Equal("package main"))
		gomega.Expect(f.Hunks[0].Lines[1].NewNum).To(gomega.Equal(2))
	})
})

var _ = ginkgo.Describe("Model factories", func() {
	var svc *testutil.MockService

	ginkgo.BeforeEach(func() {
		svc = &testutil.MockService{}
	})

	ginkgo.It("NewDiffViewModel creates a model with default PR", func() {
		m := testutil.NewDiffViewModel(svc)
		_ = m.View()
	})

	ginkgo.It("NewDiffViewModelWithPR uses the provided PR", func() {
		pr := testutil.NewPR().Number(99).Title("Custom").Build()
		m := testutil.NewDiffViewModelWithPR(svc, pr)
		_ = m.View()
	})

	ginkgo.It("NewDiffViewModelWithReview uses the provided review", func() {
		pr := testutil.NewPR().Build()
		rev := review.NewPendingReview(pr.Number, pr.NodeID, pr.HeadSHA)
		rev.AddComment("test.go", 10, "looks good")
		m := testutil.NewDiffViewModelWithReview(svc, pr, rev)
		_ = m.View()
	})

	ginkgo.It("NewPRListModel creates a model with default filter", func() {
		m := testutil.NewPRListModel(svc)
		_ = m.View()
	})

	ginkgo.It("NewPRListModel accepts custom filters", func() {
		filters := []review.PRFilter{
			{Name: "Mine", Qualifier: "author:@me"},
		}
		m := testutil.NewPRListModel(svc, filters...)
		_ = m.View()
	})

	ginkgo.It("NewPRDetailModel creates a model for a PR", func() {
		pr := testutil.NewPR().Body("## Description\nSome changes").Build()
		m := testutil.NewPRDetailModel(pr)
		_ = m.View()
	})

	ginkgo.It("NewSubmitModel creates a model with review", func() {
		rev := review.NewPendingReview(42, "PR_node42", "abc123")
		m := testutil.NewSubmitModel(svc, rev)
		_ = m.View()
	})
})
