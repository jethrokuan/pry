package cache

import "time"

// Cache is a simple key-value store with TTL-based expiration.
type Cache interface {
	// Get deserializes the cached value for key into dest.
	// Returns true if the key exists and has not expired.
	Get(key string, dest any) bool

	// Set serializes val and stores it under key with the given TTL.
	Set(key string, val any, ttl time.Duration)

	// Delete removes a single key.
	Delete(key string)

	// DeleteByPrefix removes all keys whose names start with prefix.
	DeleteByPrefix(prefix string)
}

// Noop is a cache that never stores anything. Use it when caching is disabled.
type Noop struct{}

func (Noop) Get(string, any) bool          { return false }
func (Noop) Set(string, any, time.Duration) {}
func (Noop) Delete(string)                  {}
func (Noop) DeleteByPrefix(string)          {}
