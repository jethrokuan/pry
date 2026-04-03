package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"

	"github.com/jethrokuan/pry/internal/review"
)

// FilterConfig defines a PR list filter in the config file.
type FilterConfig struct {
	Name      string `koanf:"name"`
	Qualifier string `koanf:"qualifier"`
}

// FileTreeConfig holds file tree display settings.
type FileTreeConfig struct {
	// OwnerFilter controls whether the CODEOWNERS-based owner filter is enabled
	// by default. nil = default (on when identity is available), true = on, false = off.
	OwnerFilter *bool `koanf:"owner_filter"`
}

// PRListConfig holds PR list layout settings.
type PRListConfig struct {
	SidebarWidth   int  `koanf:"sidebar_width"`   // width of sidebar in columns (default 50)
	SidebarVisible bool `koanf:"sidebar_visible"` // whether sidebar starts visible
}

// CacheConfig holds caching settings.
type CacheConfig struct {
	Enabled bool   `koanf:"enabled"`
	TTL     string `koanf:"ttl"`
}

// AIConfig holds AI review assistant settings.
type AIConfig struct {
	Enabled  *bool  `koanf:"enabled"`  // nil = auto (on when claude found), false = off
	Model    string `koanf:"model"`
	MaxTurns int    `koanf:"max_turns"`
}

// Config holds user configuration for the tool.
type Config struct {
	Editor     string         `koanf:"editor"`
	PageSize   int            `koanf:"page_size"`
	APITimeout string         `koanf:"api_timeout"`
	Filters    []FilterConfig `koanf:"filters"`
	Columns    []string       `koanf:"columns"`
	FileTree   FileTreeConfig `koanf:"file_tree"`
	PRList     PRListConfig   `koanf:"pr_list"`
	Cache      CacheConfig    `koanf:"cache"`
	AI         AIConfig       `koanf:"ai"`
}

// APITimeoutDuration parses the api_timeout string into a time.Duration.
// Returns 0 (no timeout) by default.
func (c Config) APITimeoutDuration() time.Duration {
	if c.APITimeout == "" {
		return 0
	}
	d, err := time.ParseDuration(c.APITimeout)
	if err != nil {
		return 0
	}
	return d
}

// CacheEnabled returns whether caching is enabled (default: true).
func (c Config) CacheEnabled() bool {
	return c.Cache.Enabled
}

// CacheTTLDuration parses the cache TTL string into a time.Duration.
// Returns 5 minutes by default.
func (c Config) CacheTTLDuration() time.Duration {
	if c.Cache.TTL == "" {
		return 5 * time.Minute
	}
	d, err := time.ParseDuration(c.Cache.TTL)
	if err != nil {
		return 5 * time.Minute
	}
	return d
}

// DefaultColumns returns the default PR list columns.
func DefaultColumns() []string {
	return []string{"state", "number", "title", "author", "changes", "ci", "my_review", "my_teams", "updated"}
}

// PRColumns returns the configured columns, falling back to defaults.
func (c Config) PRColumns() []string {
	if len(c.Columns) > 0 {
		return c.Columns
	}
	return DefaultColumns()
}

var defaults = confmap.Provider(map[string]interface{}{
	"page_size":             30,
	"pr_list.sidebar_width": 50,
	"cache.enabled":         true,
}, ".")

// Load reads config from ~/.config/pry/config.toml.
// Falls back to defaults for missing values.
func Load() Config {
	home, err := os.UserHomeDir()
	if err != nil {
		return load(nil)
	}
	path := filepath.Join(home, ".config", "pry", "config.toml")
	return load(&path)
}

// LoadFrom reads config from the given path.
// Falls back to defaults for missing values.
func LoadFrom(path string) Config {
	return load(&path)
}

func load(path *string) Config {
	k := koanf.New(".")

	// Load defaults first.
	if err := k.Load(defaults, nil); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not load defaults: %v\n", err)
	}

	// Overlay with file if provided and exists.
	if path != nil {
		if _, err := os.Stat(*path); err == nil {
			if err := k.Load(file.Provider(*path), toml.Parser()); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not read config %s: %v\n", *path, err)
			}
		}
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not parse config: %v\n", err)
	}
	return cfg
}

// DefaultFilters returns the built-in filter presets.
func DefaultFilters() []FilterConfig {
	return []FilterConfig{
		{Name: "My PRs", Qualifier: "is:open author:@me"},
		{Name: "Assigned to Me", Qualifier: "is:open assignee:@me"},
		{Name: "Needs My Review", Qualifier: "is:open review-requested:@me draft:false"},
		{Name: "Involved", Qualifier: "is:open involves:@me -author:@me"},
	}
}

// PRFilters converts config filters to domain types, falling back to defaults if empty.
func (c Config) PRFilters() []review.PRFilter {
	filters := c.Filters
	if len(filters) == 0 {
		filters = DefaultFilters()
	}
	result := make([]review.PRFilter, len(filters))
	for i, f := range filters {
		result[i] = review.PRFilter{Name: f.Name, Qualifier: f.Qualifier}
	}
	return result
}
