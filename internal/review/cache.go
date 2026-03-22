package review

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/jethrokuan/pry/internal/diff"
	"github.com/viccon/sturdyc"
)

// CacheInvalidator is implemented by services that support cache invalidation.
type CacheInvalidator interface {
	// InvalidateListPRs clears all cached ListPRs results.
	InvalidateListPRs()
}

// diskCacheEntry is the JSON-serializable form written to disk.
type diskCacheEntry struct {
	Qualifier string        `json:"qualifier"`
	FetchedAt time.Time     `json:"fetched_at"`
	PRs       []PullRequest `json:"prs"`
}

// CachingService wraps a Service and caches ListPRs and GetPR results.
// ListPRs results are also persisted to disk so they survive process restarts.
// All other methods are delegated directly to the inner service.
type CachingService struct {
	inner    Service
	ttl      time.Duration
	cacheDir string // empty = no disk persistence

	listCache *sturdyc.Client[[]PullRequest]
	prCache   *sturdyc.Client[*PullRequest]
}

const (
	cacheCapacity     = 100
	cacheShards       = 4
	evictionPercent   = 10
)

// NewCachingService wraps the given service with ListPRs and GetPR caching.
// If ttl <= 0, caching is disabled (all calls pass through).
// If cacheDir is non-empty, ListPRs entries are persisted to disk.
func NewCachingService(inner Service, ttl time.Duration, cacheDir string) *CachingService {
	cs := &CachingService{
		inner:    inner,
		ttl:      ttl,
		cacheDir: cacheDir,
	}
	if ttl > 0 {
		cs.listCache = sturdyc.New[[]PullRequest](cacheCapacity, cacheShards, ttl, evictionPercent)
		cs.prCache = sturdyc.New[*PullRequest](cacheCapacity, cacheShards, ttl, evictionPercent)
	}
	return cs
}

// InvalidateListPRs clears all cached results (memory and disk).
func (c *CachingService) InvalidateListPRs() {
	for _, key := range c.listCache.ScanKeys() {
		c.listCache.Delete(key)
	}
	for _, key := range c.prCache.ScanKeys() {
		c.prCache.Delete(key)
	}
	c.clearDiskCache()
}

func (c *CachingService) ListPRs(ctx context.Context, filter PRFilter) ([]PullRequest, error) {
	if c.ttl <= 0 {
		return c.inner.ListPRs(ctx, filter)
	}

	key := filter.Qualifier

	// Check disk cache on miss (sturdyc doesn't know about disk).
	prs, err := c.listCache.GetOrFetch(ctx, key, func(ctx context.Context) ([]PullRequest, error) {
		if prs, fetchedAt, ok := c.readDiskCache(key); ok && time.Since(fetchedAt) < c.ttl {
			return prs, nil
		}

		prs, err := c.inner.ListPRs(ctx, filter)
		if err != nil {
			return nil, err
		}

		c.writeDiskCache(key, prs)
		return prs, nil
	})
	if err != nil {
		return nil, err
	}

	// Return a copy so callers can't mutate the cache.
	result := make([]PullRequest, len(prs))
	copy(result, prs)
	return result, nil
}

func (c *CachingService) GetPR(ctx context.Context, number int) (*PullRequest, error) {
	if c.ttl <= 0 {
		return c.inner.GetPR(ctx, number)
	}

	key := fmt.Sprintf("pr:%d", number)

	pr, err := c.prCache.GetOrFetch(ctx, key, func(ctx context.Context) (*PullRequest, error) {
		return c.inner.GetPR(ctx, number)
	})
	if err != nil {
		return nil, err
	}

	result := *pr // shallow copy
	return &result, nil
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
func (c *CachingService) writeDiskCache(qualifier string, prs []PullRequest) {
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
		FetchedAt: time.Now(),
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

const mentionableCacheTTL = 24 * time.Hour

// mentionableDiskEntry is the JSON-serializable form for mentionable users.
type mentionableDiskEntry struct {
	FetchedAt time.Time `json:"fetched_at"`
	Users     []string  `json:"users"`
}

func (c *CachingService) mentionableCachePath() string {
	if c.cacheDir == "" {
		return ""
	}
	return filepath.Join(c.cacheDir, "mentionable_users.json")
}

func (c *CachingService) readMentionableCache() ([]string, bool) {
	path := c.mentionableCachePath()
	if path == "" {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var entry mentionableDiskEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, false
	}
	if time.Since(entry.FetchedAt) >= mentionableCacheTTL {
		return nil, false
	}
	return entry.Users, true
}

func (c *CachingService) writeMentionableCache(users []string) {
	path := c.mentionableCachePath()
	if path == "" {
		return
	}
	if err := os.MkdirAll(c.cacheDir, 0o755); err != nil {
		return
	}
	data, err := json.Marshal(mentionableDiskEntry{
		FetchedAt: time.Now(),
		Users:     users,
	})
	if err != nil {
		return
	}
	os.WriteFile(path, data, 0o644)
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

func (c *CachingService) RepoOwner() string { return c.inner.RepoOwner() }
func (c *CachingService) RepoName() string  { return c.inner.RepoName() }
func (c *CachingService) CurrentUser(ctx context.Context) (string, error) {
	return c.inner.CurrentUser(ctx)
}
func (c *CachingService) UserTeams(ctx context.Context) ([]string, error) {
	return c.inner.UserTeams(ctx)
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
	if c.cacheDir != "" {
		if users, ok := c.readMentionableCache(); ok {
			return users, nil
		}
	}
	users, err := c.inner.ListMentionableUsers(ctx)
	if err != nil {
		return nil, err
	}
	if c.cacheDir != "" {
		c.writeMentionableCache(users)
	}
	return users, nil
}
func (c *CachingService) UploadImage(ctx context.Context, data []byte, filename string) (string, error) {
	return c.inner.UploadImage(ctx, data, filename)
}
