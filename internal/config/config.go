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

// Config holds user configuration for the tool.
type Config struct {
	Editor   string         `koanf:"editor"`
	UseDelta bool           `koanf:"use_delta"`
	PageSize int            `koanf:"page_size"`
	CacheTTL string         `koanf:"cache_ttl"`
	Filters  []FilterConfig `koanf:"filters"`
	Columns  []string       `koanf:"columns"`
	FileTree FileTreeConfig `koanf:"file_tree"`
	PRList   PRListConfig   `koanf:"pr_list"`
}

// CacheTTLDuration parses the CacheTTL string into a time.Duration.
// Returns 5 minutes by default. Set to "0" to disable caching.
func (c Config) CacheTTLDuration() time.Duration {
	if c.CacheTTL == "" {
		return 5 * time.Minute
	}
	d, err := time.ParseDuration(c.CacheTTL)
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
	"use_delta":            true,
	"page_size":            50,
	"pr_list.sidebar_width": 50,
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
		{Name: "Needs My Review", Qualifier: "review-requested:@me"},
		{Name: "My Team Pending", Qualifier: "team-review-requested:@my-teams"},
		{Name: "Reviewed, Not Approved", Qualifier: "reviewed-by:@me -review:approved"},
		{Name: "Awaiting My Review", Qualifier: "-reviewed-by:@me -review:approved review:required"},
		{Name: "All Open", Qualifier: ""},
		{Name: "Authored by Me", Qualifier: "author:@me"},
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
