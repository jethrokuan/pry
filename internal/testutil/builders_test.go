package testutil_test

import (
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/jethrokuan/pry/internal/diff"
	"github.com/jethrokuan/pry/internal/review"
	"github.com/jethrokuan/pry/internal/testutil"
)

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
	ginkgo.It("NewDiffViewModel creates a model with default PR", func() {
		m := testutil.NewDiffViewModel()
		_ = m.View()
	})

	ginkgo.It("NewDiffViewModelWithPR uses the provided PR", func() {
		pr := testutil.NewPR().BuildPtr()
		m := testutil.NewDiffViewModelWithPR(pr)
		_ = m.View()
	})

	ginkgo.It("NewDiffViewModelWithReview uses the provided review", func() {
		pr := testutil.NewPR().BuildPtr()
		rev := review.NewPendingReview()
		pr.Threads = append(pr.Threads, review.Thread{
			Path: "test.go", Line: 10, Side: "RIGHT",
			Comments: []review.Comment{
				{ID: -1, Body: "looks good", IsPending: true},
			},
		})
		m := testutil.NewDiffViewModelWithReview(pr, rev)
		_ = m.View()
	})

	ginkgo.It("NewPRListModel creates a model with default filter", func() {
		m := testutil.NewPRListModel()
		_ = m.View()
	})

	ginkgo.It("NewPRListModel accepts custom filters", func() {
		filters := []review.PRFilter{
			{Name: "Mine", Qualifier: "author:@me"},
		}
		m := testutil.NewPRListModel(filters...)
		_ = m.View()
	})

	ginkgo.It("NewSubmitModel creates a model with review", func() {
		pr := testutil.NewPR().BuildPtr()
		pr.StartReview()
		m := testutil.NewSubmitModel(pr)
		_ = m.View()
	})
})
