package review

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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

// diskCacheEntry is the JSON-serializable form written to disk.
type diskCacheEntry struct {
	Qualifier string        `json:"qualifier"`
	FetchedAt time.Time     `json:"fetched_at"`
	PRs       []PullRequest `json:"prs"`
}

// CachingService wraps a Service and caches ListPRs results with a TTL.
// It maintains an in-memory cache as a fast path and optionally persists
// cache entries to disk so they survive process restarts.
// All other methods are delegated directly to the inner service.
type CachingService struct {
	inner    Service
	ttl      time.Duration
	cacheDir string // empty = no disk persistence

	mu    sync.Mutex
	cache map[string]cacheEntry
}

// NewCachingService wraps the given service with ListPRs caching.
// If ttl <= 0, caching is disabled (all calls pass through).
// If cacheDir is non-empty, cache entries are persisted to disk.
func NewCachingService(inner Service, ttl time.Duration, cacheDir string) *CachingService {
	return &CachingService{
		inner:    inner,
		ttl:      ttl,
		cacheDir: cacheDir,
		cache:    make(map[string]cacheEntry),
	}
}

// InvalidateListPRs clears all cached filter results (memory and disk).
func (c *CachingService) InvalidateListPRs() {
	c.mu.Lock()
	c.cache = make(map[string]cacheEntry)
	c.mu.Unlock()

	c.clearDiskCache()
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

	// Check disk cache before hitting the network.
	if prs, fetchedAt, ok := c.readDiskCache(key); ok && time.Since(fetchedAt) < c.ttl {
		// Populate in-memory cache from disk.
		cached := make([]PullRequest, len(prs))
		copy(cached, prs)
		c.mu.Lock()
		c.cache[key] = cacheEntry{prs: cached, fetchedAt: fetchedAt}
		c.mu.Unlock()

		result := make([]PullRequest, len(prs))
		copy(result, prs)
		return result, nil
	}

	prs, err := c.inner.ListPRs(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Store a copy in the cache so callers can't mutate it.
	cached := make([]PullRequest, len(prs))
	copy(cached, prs)
	now := time.Now()

	c.mu.Lock()
	c.cache[key] = cacheEntry{
		prs:       cached,
		fetchedAt: now,
	}
	c.mu.Unlock()

	c.writeDiskCache(key, prs, now)

	return prs, nil
}

// cacheFilePath returns the disk path for a given cache key (qualifier).
func (c *CachingService) cacheFilePath(qualifier string) string {
	if c.cacheDir == "" {
		return ""
	}
	h := sha256.Sum256([]byte(qualifier))
	name := fmt.Sprintf("prlist_%x.json", h[:8])
	return filepath.Join(c.cacheDir, name)
}

// readDiskCache loads a cache entry from disk. Returns (nil, zero, false) on any error.
func (c *CachingService) readDiskCache(qualifier string) ([]PullRequest, time.Time, bool) {
	path := c.cacheFilePath(qualifier)
	if path == "" {
		return nil, time.Time{}, false
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, time.Time{}, false
	}

	var entry diskCacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		slog.Debug("cache: failed to unmarshal disk cache", "path", path, "err", err)
		return nil, time.Time{}, false
	}

	return entry.PRs, entry.FetchedAt, true
}

// writeDiskCache persists a cache entry to disk. Errors are logged but non-fatal.
func (c *CachingService) writeDiskCache(qualifier string, prs []PullRequest, fetchedAt time.Time) {
	path := c.cacheFilePath(qualifier)
	if path == "" {
		return
	}

	if err := os.MkdirAll(c.cacheDir, 0o755); err != nil {
		slog.Debug("cache: failed to create cache dir", "dir", c.cacheDir, "err", err)
		return
	}

	entry := diskCacheEntry{
		Qualifier: qualifier,
		FetchedAt: fetchedAt,
		PRs:       prs,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		slog.Debug("cache: failed to marshal cache entry", "err", err)
		return
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		slog.Debug("cache: failed to write cache file", "path", path, "err", err)
	}
}

// clearDiskCache removes all cache files from the cache directory.
func (c *CachingService) clearDiskCache() {
	if c.cacheDir == "" {
		return
	}

	entries, err := os.ReadDir(c.cacheDir)
	if err != nil {
		return
	}

	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			os.Remove(filepath.Join(c.cacheDir, e.Name()))
		}
	}
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
