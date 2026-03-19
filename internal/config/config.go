package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/jkuan/pr-review/internal/ui/styles"
)

// Config holds user configuration for the tool.
type Config struct {
	Editor   string       `toml:"editor"`
	UseDelta bool         `toml:"use_delta"`
	PageSize int          `toml:"page_size"`
	Theme    string       `toml:"theme"`
	Colors   styles.Theme `toml:"colors"`
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
