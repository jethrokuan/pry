package testutil

import (
	"fmt"
	"time"

	"github.com/jkuan/pr-review/internal/config"
	"github.com/jkuan/pr-review/internal/diff"
	"github.com/jkuan/pr-review/internal/review"
	"github.com/jkuan/pr-review/internal/ui/diffview"
	"github.com/jkuan/pr-review/internal/ui/prdetail"
	"github.com/jkuan/pr-review/internal/ui/prlist"
	"github.com/jkuan/pr-review/internal/ui/submit"
)

// --- Pull Request builder ---

// PRBuilder builds review.PullRequest values for tests.
type PRBuilder struct {
	pr review.PullRequest
}

// NewPR returns a PRBuilder with sensible defaults.
func NewPR() *PRBuilder {
	return &PRBuilder{
		pr: review.PullRequest{
			Number:    42,
			NodeID:    "PR_node42",
			Title:     "Test PR",
			Author:    "test-user",
			Branch:    "feature-branch",
			Base:      "main",
			State:     "OPEN",
			CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			HeadSHA:   "abc123",
		},
	}
}

func (b *PRBuilder) Number(n int) *PRBuilder      { b.pr.Number = n; return b }
func (b *PRBuilder) Title(t string) *PRBuilder     { b.pr.Title = t; return b }
func (b *PRBuilder) Author(a string) *PRBuilder    { b.pr.Author = a; return b }
func (b *PRBuilder) Branch(br string) *PRBuilder   { b.pr.Branch = br; return b }
func (b *PRBuilder) Base(base string) *PRBuilder   { b.pr.Base = base; return b }
func (b *PRBuilder) NodeID(id string) *PRBuilder   { b.pr.NodeID = id; return b }
func (b *PRBuilder) HeadSHA(s string) *PRBuilder   { b.pr.HeadSHA = s; return b }
func (b *PRBuilder) Body(body string) *PRBuilder   { b.pr.Body = body; return b }
func (b *PRBuilder) Draft(d bool) *PRBuilder       { b.pr.Draft = d; return b }
func (b *PRBuilder) State(s string) *PRBuilder     { b.pr.State = s; return b }
func (b *PRBuilder) Labels(l ...string) *PRBuilder { b.pr.Labels = l; return b }
func (b *PRBuilder) Changes(add, del, files int) *PRBuilder {
	b.pr.Additions = add
	b.pr.Deletions = del
	b.pr.Files = files
	return b
}
func (b *PRBuilder) ReviewDecision(d string) *PRBuilder { b.pr.ReviewDecision = d; return b }

// Build returns the constructed PullRequest.
func (b *PRBuilder) Build() review.PullRequest { return b.pr }

// --- DiffFile helpers ---

// SimpleDiffFile creates a DiffFile with a single hunk containing the given added lines.
func SimpleDiffFile(path string, addedLines ...string) diff.DiffFile {
	lines := make([]diff.DiffLine, 0, len(addedLines))
	for i, content := range addedLines {
		lines = append(lines, diff.DiffLine{
			Type:    diff.LineAddition,
			Content: content,
			NewNum:  i + 1,
		})
	}
	return diff.DiffFile{
		Path:    path,
		OldPath: path,
		Status:  diff.StatusModified,
		Hunks: []diff.Hunk{
			{
				OldStart: 1, OldLines: 0,
				NewStart: 1, NewLines: len(addedLines),
				Header:   fmt.Sprintf("@@ -1,0 +1,%d @@", len(addedLines)),
				Lines:    lines,
			},
		},
	}
}

// --- Model factories ---

// NewDiffViewModel creates a diffview.Model with a mock service and test PR.
// The returned model is in its initial loading state.
func NewDiffViewModel(svc review.Service, opts ...diffview.Option) diffview.Model {
	pr := NewPR().Build()
	rev := review.NewPendingReview(pr.Number, pr.NodeID, pr.HeadSHA)
	return diffview.New(svc, pr, rev, opts...)
}

// NewDiffViewModelWithPR creates a diffview.Model with a specific PR.
func NewDiffViewModelWithPR(svc review.Service, pr review.PullRequest, opts ...diffview.Option) diffview.Model {
	rev := review.NewPendingReview(pr.Number, pr.NodeID, pr.HeadSHA)
	return diffview.New(svc, pr, rev, opts...)
}

// NewDiffViewModelWithReview creates a diffview.Model with a specific PR and pending review.
func NewDiffViewModelWithReview(svc review.Service, pr review.PullRequest, rev *review.PendingReview, opts ...diffview.Option) diffview.Model {
	return diffview.New(svc, pr, rev, opts...)
}

// NewPRListModel creates a prlist.Model with a mock service and optional filters.
func NewPRListModel(svc review.Service, filters ...review.PRFilter) prlist.Model {
	if len(filters) == 0 {
		filters = []review.PRFilter{
			{Name: "Default", Qualifier: "is:open"},
		}
	}
	return prlist.New(svc, filters, config.DefaultColumns())
}

// NewPRDetailModel creates a prdetail.Model for the given PR.
func NewPRDetailModel(pr review.PullRequest) prdetail.Model {
	return prdetail.New(pr)
}

// NewSubmitModel creates a submit.Model with a mock service and pending review.
func NewSubmitModel(svc review.Service, rev *review.PendingReview) submit.Model {
	return submit.New(svc, rev)
}
