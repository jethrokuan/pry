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

// diskCacheEntry is the JSON-serializable form written to disk.
type diskCacheEntry struct {
	Qualifier string        `json:"qualifier"`
	FetchedAt time.Time     `json:"fetched_at"`
	PRs       []PullRequest `json:"prs"`
}

type listCacheEntry struct {
	prs       []PullRequest
	fetchedAt time.Time
}

type prCacheEntry struct {
	pr        *PullRequest
	fetchedAt time.Time
}

// CachingService wraps a Service and caches ListPRs and GetPR results.
// ListPRs results are also persisted to disk so they survive process restarts.
// All other methods are delegated directly to the inner service.
type CachingService struct {
	inner    Service
	ttl      time.Duration
	cacheDir string // empty = no disk persistence

	mu        sync.Mutex
	listCache map[string]listCacheEntry

	prMu    sync.Mutex
	prCache map[int]prCacheEntry
}

// NewCachingService wraps the given service with ListPRs and GetPR caching.
// If ttl <= 0, caching is disabled (all calls pass through).
// If cacheDir is non-empty, ListPRs entries are persisted to disk.
func NewCachingService(inner Service, ttl time.Duration, cacheDir string) *CachingService {
	return &CachingService{
		inner:     inner,
		ttl:       ttl,
		cacheDir:  cacheDir,
		listCache: make(map[string]listCacheEntry),
		prCache:   make(map[int]prCacheEntry),
	}
}

// InvalidateListPRs clears all cached results (memory and disk).
func (c *CachingService) InvalidateListPRs() {
	c.mu.Lock()
	c.listCache = make(map[string]listCacheEntry)
	c.mu.Unlock()

	c.prMu.Lock()
	c.prCache = make(map[int]prCacheEntry)
	c.prMu.Unlock()

	c.clearDiskCache()
}

func (c *CachingService) ListPRs(ctx context.Context, filter PRFilter) ([]PullRequest, error) {
	if c.ttl <= 0 {
		return c.inner.ListPRs(ctx, filter)
	}

	key := filter.Qualifier

	// Check in-memory cache.
	c.mu.Lock()
	if entry, ok := c.listCache[key]; ok && time.Since(entry.fetchedAt) < c.ttl {
		result := make([]PullRequest, len(entry.prs))
		copy(result, entry.prs)
		c.mu.Unlock()
		return result, nil
	}
	c.mu.Unlock()

	// Check disk cache.
	if prs, fetchedAt, ok := c.readDiskCache(key); ok && time.Since(fetchedAt) < c.ttl {
		c.mu.Lock()
		c.listCache[key] = listCacheEntry{prs: prs, fetchedAt: fetchedAt}
		c.mu.Unlock()

		result := make([]PullRequest, len(prs))
		copy(result, prs)
		return result, nil
	}

	// Fetch from upstream.
	prs, err := c.inner.ListPRs(ctx, filter)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	c.mu.Lock()
	c.listCache[key] = listCacheEntry{prs: prs, fetchedAt: now}
	c.mu.Unlock()

	c.writeDiskCache(key, prs)

	result := make([]PullRequest, len(prs))
	copy(result, prs)
	return result, nil
}

func (c *CachingService) GetPR(ctx context.Context, number int) (*PullRequest, error) {
	if c.ttl <= 0 {
		return c.inner.GetPR(ctx, number)
	}

	// Check in-memory cache.
	c.prMu.Lock()
	if entry, ok := c.prCache[number]; ok && time.Since(entry.fetchedAt) < c.ttl {
		result := *entry.pr // shallow copy
		c.prMu.Unlock()
		return &result, nil
	}
	c.prMu.Unlock()

	// Fetch from upstream.
	pr, err := c.inner.GetPR(ctx, number)
	if err != nil {
		return nil, err
	}

	c.prMu.Lock()
	c.prCache[number] = prCacheEntry{pr: pr, fetchedAt: time.Now()}
	c.prMu.Unlock()

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
