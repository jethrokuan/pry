package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/jkuan/pr-review/internal/review"
	"github.com/jkuan/pr-review/internal/ui/styles"
)

// FilterConfig defines a PR list filter in the config file.
type FilterConfig struct {
	Name      string `toml:"name"`
	Qualifier string `toml:"qualifier"`
}

// Config holds user configuration for the tool.
type Config struct {
	Editor   string         `toml:"editor"`
	UseDelta bool           `toml:"use_delta"`
	PageSize int            `toml:"page_size"`
	Theme    string         `toml:"theme"`
	Colors   styles.Theme   `toml:"colors"`
	Filters  []FilterConfig `toml:"filters"`
	Columns  []string       `toml:"columns"`
}

// DefaultColumns returns the default PR list columns.
func DefaultColumns() []string {
	return []string{"number", "title", "author", "changes", "my_review", "my_teams", "updated"}
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
		Theme:    "default",
	}
}

// Load reads config from ~/.config/pr-review/config.toml.
// Falls back to defaults for missing values.
func Load() Config {
	cfg := Default()

	home, err := os.UserHomeDir()
	if err != nil {
		return cfg
	}

	path := filepath.Join(home, ".config", "pr-review", "config.toml")
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

// ResolveTheme returns a Theme based on the config: picks a built-in theme
// and overlays any per-color overrides from [colors].
func ResolveTheme(cfg Config) styles.Theme {
	themeFn, ok := styles.BuiltinThemes[cfg.Theme]
	if !ok {
		themeFn = styles.DefaultTheme
	}
	theme := themeFn()
	return styles.OverlayColors(theme, cfg.Colors)
}
