package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/jethrokuan/pry/internal/review"
)

// FilterConfig defines a PR list filter in the config file.
type FilterConfig struct {
	Name      string `toml:"name"`
	Qualifier string `toml:"qualifier"`
}

// FileTreeConfig holds file tree display settings.
type FileTreeConfig struct {
	// OwnerFilter controls whether the CODEOWNERS-based owner filter is enabled
	// by default. nil = default (on when identity is available), true = on, false = off.
	OwnerFilter *bool `toml:"owner_filter"`
}

// PRListConfig holds PR list layout settings.
type PRListConfig struct {
	SidebarWidth   int  `toml:"sidebar_width"`   // width of sidebar in columns (default 50)
	SidebarVisible bool `toml:"sidebar_visible"` // whether sidebar starts visible
}

// Config holds user configuration for the tool.
type Config struct {
	Editor   string         `toml:"editor"`
	UseDelta bool           `toml:"use_delta"`
	PageSize int            `toml:"page_size"`
	Filters  []FilterConfig `toml:"filters"`
	Columns  []string       `toml:"columns"`
	FileTree FileTreeConfig `toml:"file_tree"`
	PRList   PRListConfig   `toml:"pr_list"`
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

// Default returns the default configuration.
func Default() Config {
	return Config{
		Editor:   "",
		UseDelta: true,
		PageSize: 50,
		PRList: PRListConfig{
			SidebarWidth: 50,
		},
	}
}

// LoadFrom reads config from the given path.
// Falls back to defaults for missing values.
func LoadFrom(path string) Config {
	cfg := Default()
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not read config %s: %v\n", path, err)
		return Default()
	}
	return cfg
}

// Load reads config from ~/.config/pry/config.toml.
// Falls back to defaults for missing values.
func Load() Config {
	cfg := Default()

	home, err := os.UserHomeDir()
	if err != nil {
		return cfg
	}

	path := filepath.Join(home, ".config", "pry", "config.toml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cfg
	}

	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return Default()
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

