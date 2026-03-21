package review

import (
	"context"
	"sync"
	"time"

	"github.com/jethrokuan/pry/internal/diff"
)

// CacheInvalidator is implemented by services that support cache invalidation.
type CacheInvalidator interface {
	// InvalidateListPRs clears all cached ListPRs results.
	InvalidateListPRs()
}

type cacheEntry struct {
	prs       []PullRequest
	fetchedAt time.Time
}

// CachingService wraps a Service and caches ListPRs results with a TTL.
// All other methods are delegated directly to the inner service.
type CachingService struct {
	inner Service
	ttl   time.Duration

	mu    sync.Mutex
	cache map[string]cacheEntry
}

// NewCachingService wraps the given service with ListPRs caching.
// If ttl <= 0, caching is disabled (all calls pass through).
func NewCachingService(inner Service, ttl time.Duration) *CachingService {
	return &CachingService{
		inner: inner,
		ttl:   ttl,
		cache: make(map[string]cacheEntry),
	}
}

// InvalidateListPRs clears all cached filter results.
func (c *CachingService) InvalidateListPRs() {
	c.mu.Lock()
	c.cache = make(map[string]cacheEntry)
	c.mu.Unlock()
}

func (c *CachingService) ListPRs(ctx context.Context, filter PRFilter) ([]PullRequest, error) {
	if c.ttl <= 0 {
		return c.inner.ListPRs(ctx, filter)
	}

	key := filter.Qualifier

	c.mu.Lock()
	if entry, ok := c.cache[key]; ok && time.Since(entry.fetchedAt) < c.ttl {
		// Return a copy so callers can't mutate the cache.
		result := make([]PullRequest, len(entry.prs))
		copy(result, entry.prs)
		c.mu.Unlock()
		return result, nil
	}
	c.mu.Unlock()

	prs, err := c.inner.ListPRs(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Store a copy in the cache so callers can't mutate it.
	cached := make([]PullRequest, len(prs))
	copy(cached, prs)

	c.mu.Lock()
	c.cache[key] = cacheEntry{
		prs:       cached,
		fetchedAt: time.Now(),
	}
	c.mu.Unlock()

	return prs, nil
}

// Delegated methods — pass through to inner service.

func (c *CachingService) RepoOwner() string                    { return c.inner.RepoOwner() }
func (c *CachingService) RepoName() string                     { return c.inner.RepoName() }
func (c *CachingService) CurrentUser(ctx context.Context) (string, error) {
	return c.inner.CurrentUser(ctx)
}
func (c *CachingService) UserTeams(ctx context.Context) ([]string, error) {
	return c.inner.UserTeams(ctx)
}
func (c *CachingService) GetPR(ctx context.Context, number int) (*PullRequest, error) {
	return c.inner.GetPR(ctx, number)
}
func (c *CachingService) FetchDiffFiles(ctx context.Context, number int) ([]diff.DiffFile, error) {
	return c.inner.FetchDiffFiles(ctx, number)
}
func (c *CachingService) FetchExistingComments(ctx context.Context, number int) ([]ExistingComment, error) {
	return c.inner.FetchExistingComments(ctx, number)
}
func (c *CachingService) FetchPendingReview(ctx context.Context, number int) (int, string, []ExistingComment, error) {
	return c.inner.FetchPendingReview(ctx, number)
}
func (c *CachingService) CreatePendingReview(ctx context.Context, prNumber int) (int, string, error) {
	return c.inner.CreatePendingReview(ctx, prNumber)
}
func (c *CachingService) AddReviewComment(ctx context.Context, reviewNodeID string, comment InlineComment) (int, error) {
	return c.inner.AddReviewComment(ctx, reviewNodeID, comment)
}
func (c *CachingService) DeleteReviewComment(ctx context.Context, prNumber, commentID int) error {
	return c.inner.DeleteReviewComment(ctx, prNumber, commentID)
}
func (c *CachingService) EditReviewComment(ctx context.Context, prNumber, commentID int, body string) error {
	return c.inner.EditReviewComment(ctx, prNumber, commentID, body)
}
func (c *CachingService) SubmitReview(ctx context.Context, pr *PullRequest, review *PendingReview) error {
	return c.inner.SubmitReview(ctx, pr, review)
}
func (c *CachingService) FetchViewedFiles(ctx context.Context, prNodeID string) (map[string]bool, error) {
	return c.inner.FetchViewedFiles(ctx, prNodeID)
}
func (c *CachingService) MarkFileAsViewed(ctx context.Context, prNodeID, path string) error {
	return c.inner.MarkFileAsViewed(ctx, prNodeID, path)
}
func (c *CachingService) UnmarkFileAsViewed(ctx context.Context, prNodeID, path string) error {
	return c.inner.UnmarkFileAsViewed(ctx, prNodeID, path)
}
func (c *CachingService) ListMentionableUsers(ctx context.Context) ([]string, error) {
	return c.inner.ListMentionableUsers(ctx)
}
func (c *CachingService) UploadImage(ctx context.Context, data []byte, filename string) (string, error) {
	return c.inner.UploadImage(ctx, data, filename)
}
