package cache

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// envelope is the on-disk JSON format for cached entries.
type envelope struct {
	ExpiresAt time.Time       `json:"expires_at"`
	Data      json.RawMessage `json:"data"`
}

// Disk is a file-system-backed cache. Each key maps to a JSON file in dir.
type Disk struct {
	dir string
}

// NewDisk creates a disk cache rooted at dir.
// The directory is created lazily on the first Set.
func NewDisk(dir string) *Disk {
	return &Disk{dir: dir}
}

func (d *Disk) path(key string) string {
	return filepath.Join(d.dir, key+".json")
}

func (d *Disk) Get(key string, dest any) bool {
	data, err := os.ReadFile(d.path(key))
	if err != nil {
		return false
	}

	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		slog.Debug("cache: corrupt entry", "key", key, "err", err)
		return false
	}

	if time.Now().After(env.ExpiresAt) {
		return false
	}

	if err := json.Unmarshal(env.Data, dest); err != nil {
		slog.Debug("cache: failed to unmarshal data", "key", key, "err", err)
		return false
	}
	return true
}

func (d *Disk) Set(key string, val any, ttl time.Duration) {
	raw, err := json.Marshal(val)
	if err != nil {
		slog.Debug("cache: failed to marshal value", "key", key, "err", err)
		return
	}

	env := envelope{
		ExpiresAt: time.Now().Add(ttl),
		Data:      raw,
	}

	data, err := json.Marshal(env)
	if err != nil {
		slog.Debug("cache: failed to marshal envelope", "key", key, "err", err)
		return
	}

	if err := os.MkdirAll(d.dir, 0o755); err != nil {
		slog.Debug("cache: failed to create dir", "dir", d.dir, "err", err)
		return
	}

	if err := os.WriteFile(d.path(key), data, 0o644); err != nil {
		slog.Debug("cache: failed to write", "key", key, "err", err)
	}
}

func (d *Disk) Delete(key string) {
	os.Remove(d.path(key))
}

func (d *Disk) DeleteByPrefix(prefix string) {
	entries, err := os.ReadDir(d.dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), prefix) {
			os.Remove(filepath.Join(d.dir, e.Name()))
		}
	}
}
