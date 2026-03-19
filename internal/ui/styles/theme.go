package styles

import "github.com/charmbracelet/lipgloss"

// Theme holds every semantic color used by the application.
// Values are lipgloss-compatible: ANSI number ("4"), 256-color ("237"), or hex ("#282828").
type Theme struct {
	Fg         string `toml:"fg"`
	FgMuted    string `toml:"fg_muted"`
	FgEmphasis string `toml:"fg_emphasis"`

	Bg        string `toml:"bg"`
	BgSurface string `toml:"bg_surface"`
	BgOverlay string `toml:"bg_overlay"`
	BgCursor  string `toml:"bg_cursor"`
	BgSearch  string `toml:"bg_search"`

	AccentPrimary   string `toml:"accent_primary"`
	AccentSecondary string `toml:"accent_secondary"`
	AccentInfo      string `toml:"accent_info"`
	AccentSuccess   string `toml:"accent_success"`
	AccentWarning   string `toml:"accent_warning"`
	AccentDanger    string `toml:"accent_danger"`

	DiffContext  string `toml:"diff_context"`
	BgDiffAdd    string `toml:"bg_diff_add"`
	BgDiffDelete string `toml:"bg_diff_delete"`
	BgHunkHeader  string `toml:"bg_hunk_header"`
	BgActiveHunk  string `toml:"bg_active_hunk"`
	LabelFg       string `toml:"label_fg"`
}

// Current is the active theme.
var Current Theme

// DefaultTheme uses ANSI 0-15 colors so it inherits the user's terminal palette.
func DefaultTheme() Theme {
	return Theme{
		FgMuted:    "8",
		FgEmphasis: "15",

		BgSurface: "236",
		BgOverlay: "235",
		BgCursor:  "237",
		BgSearch:  "226",

		AccentPrimary:   "4",
		AccentSecondary: "5",
		AccentInfo:      "6",
		AccentSuccess:   "2",
		AccentWarning:   "3",
		AccentDanger:    "1",

		DiffContext:  "7",
		BgDiffAdd:    "22",
		BgDiffDelete: "52",
		BgHunkHeader: "238",
		BgActiveHunk: "234",
		LabelFg:      "0",
	}
}

func SolarizedDark() Theme {
	return Theme{
		Fg:         "#839496",
		FgMuted:    "#586e75",
		FgEmphasis: "#eee8d5",
		Bg:         "#002b36",
		BgSurface:  "#073642",
		BgOverlay:  "#073642",
		BgCursor:   "#073642",
		BgSearch:   "#b58900",
		AccentPrimary:   "#268bd2",
		AccentSecondary: "#d33682",
		AccentInfo:      "#2aa198",
		AccentSuccess:   "#859900",
		AccentWarning:   "#b58900",
		AccentDanger:    "#dc322f",
		DiffContext:     "#839496",
		BgDiffAdd:       "#003520",
		BgDiffDelete:    "#3a0c0c",
		BgHunkHeader:    "#073642",
		BgActiveHunk:    "#052f3a",
		LabelFg:        "#002b36",
	}
}

func SolarizedLight() Theme {
	return Theme{
		Fg:         "#657b83",
		FgMuted:    "#93a1a1",
		FgEmphasis: "#073642",
		Bg:         "#fdf6e3",
		BgSurface:  "#eee8d5",
		BgOverlay:  "#eee8d5",
		BgCursor:   "#eee8d5",
		BgSearch:   "#b58900",
		AccentPrimary:   "#268bd2",
		AccentSecondary: "#d33682",
		AccentInfo:      "#2aa198",
		AccentSuccess:   "#859900",
		AccentWarning:   "#b58900",
		AccentDanger:    "#dc322f",
		DiffContext:     "#657b83",
		BgDiffAdd:       "#e6f2d5",
		BgDiffDelete:    "#f8d7d5",
		BgHunkHeader:    "#eee8d5",
		BgActiveHunk:    "#f0ead8",
		LabelFg:        "#fdf6e3",
	}
}

func CatppuccinMocha() Theme {
	return Theme{
		Fg:         "#cdd6f4",
		FgMuted:    "#6c7086",
		FgEmphasis: "#cdd6f4",
		Bg:         "#1e1e2e",
		BgSurface:  "#313244",
		BgOverlay:  "#181825",
		BgCursor:   "#45475a",
		BgSearch:   "#f9e2af",
		AccentPrimary:   "#89b4fa",
		AccentSecondary: "#cba6f7",
		AccentInfo:      "#94e2d5",
		AccentSuccess:   "#a6e3a1",
		AccentWarning:   "#f9e2af",
		AccentDanger:    "#f38ba8",
		DiffContext:     "#bac2de",
		BgDiffAdd:       "#1a3a2a",
		BgDiffDelete:    "#3a1a2a",
		BgHunkHeader:    "#313244",
		BgActiveHunk:    "#242436",
		LabelFg:        "#1e1e2e",
	}
}

func CatppuccinLatte() Theme {
	return Theme{
		Fg:         "#4c4f69",
		FgMuted:    "#9ca0b0",
		FgEmphasis: "#4c4f69",
		Bg:         "#eff1f5",
		BgSurface:  "#ccd0da",
		BgOverlay:  "#e6e9ef",
		BgCursor:   "#bcc0cc",
		BgSearch:   "#df8e1d",
		AccentPrimary:   "#1e66f5",
		AccentSecondary: "#8839ef",
		AccentInfo:      "#179299",
		AccentSuccess:   "#40a02b",
		AccentWarning:   "#df8e1d",
		AccentDanger:    "#d20f39",
		DiffContext:     "#5c5f77",
		BgDiffAdd:       "#d9f0d0",
		BgDiffDelete:    "#f5d0d0",
		BgHunkHeader:    "#ccd0da",
		BgActiveHunk:    "#e2e5ed",
		LabelFg:        "#eff1f5",
	}
}

func GruvboxDark() Theme {
	return Theme{
		Fg:         "#ebdbb2",
		FgMuted:    "#a89984",
		FgEmphasis: "#fbf1c7",
		Bg:         "#282828",
		BgSurface:  "#3c3836",
		BgOverlay:  "#3c3836",
		BgCursor:   "#504945",
		BgSearch:   "#fabd2f",
		AccentPrimary:   "#83a598",
		AccentSecondary: "#d3869b",
		AccentInfo:      "#8ec07c",
		AccentSuccess:   "#b8bb26",
		AccentWarning:   "#fabd2f",
		AccentDanger:    "#fb4934",
		DiffContext:     "#d5c4a1",
		BgDiffAdd:       "#2e3b1f",
		BgDiffDelete:    "#3c1f1f",
		BgHunkHeader:    "#3c3836",
		BgActiveHunk:    "#32302f",
		LabelFg:        "#282828",
	}
}

func GruvboxLight() Theme {
	return Theme{
		Fg:         "#3c3836",
		FgMuted:    "#928374",
		FgEmphasis: "#282828",
		Bg:         "#fbf1c7",
		BgSurface:  "#ebdbb2",
		BgOverlay:  "#ebdbb2",
		BgCursor:   "#d5c4a1",
		BgSearch:   "#d79921",
		AccentPrimary:   "#458588",
		AccentSecondary: "#b16286",
		AccentInfo:      "#689d6a",
		AccentSuccess:   "#79740e",
		AccentWarning:   "#d79921",
		AccentDanger:    "#cc241d",
		DiffContext:     "#504945",
		BgDiffAdd:       "#e2edb8",
		BgDiffDelete:    "#f0c8c0",
		BgHunkHeader:    "#ebdbb2",
		BgActiveHunk:    "#f5ead0",
		LabelFg:        "#fbf1c7",
	}
}

func Dracula() Theme {
	return Theme{
		Fg:         "#f8f8f2",
		FgMuted:    "#6272a4",
		FgEmphasis: "#f8f8f2",
		Bg:         "#282a36",
		BgSurface:  "#44475a",
		BgOverlay:  "#21222c",
		BgCursor:   "#44475a",
		BgSearch:   "#f1fa8c",
		AccentPrimary:   "#8be9fd",
		AccentSecondary: "#ff79c6",
		AccentInfo:      "#8be9fd",
		AccentSuccess:   "#50fa7b",
		AccentWarning:   "#f1fa8c",
		AccentDanger:    "#ff5555",
		DiffContext:     "#f8f8f2",
		BgDiffAdd:       "#1a3a1a",
		BgDiffDelete:    "#3a1a1a",
		BgHunkHeader:    "#44475a",
		BgActiveHunk:    "#2e3044",
		LabelFg:        "#282a36",
	}
}

func Nord() Theme {
	return Theme{
		Fg:         "#d8dee9",
		FgMuted:    "#4c566a",
		FgEmphasis: "#eceff4",
		Bg:         "#2e3440",
		BgSurface:  "#3b4252",
		BgOverlay:  "#3b4252",
		BgCursor:   "#434c5e",
		BgSearch:   "#ebcb8b",
		AccentPrimary:   "#81a1c1",
		AccentSecondary: "#b48ead",
		AccentInfo:      "#88c0d0",
		AccentSuccess:   "#a3be8c",
		AccentWarning:   "#ebcb8b",
		AccentDanger:    "#bf616a",
		DiffContext:     "#d8dee9",
		BgDiffAdd:       "#2b3d2e",
		BgDiffDelete:    "#3d2b2e",
		BgHunkHeader:    "#3b4252",
		BgActiveHunk:    "#333a48",
		LabelFg:        "#2e3440",
	}
}

func TokyoNight() Theme {
	return Theme{
		Fg:         "#c0caf5",
		FgMuted:    "#565f89",
		FgEmphasis: "#c0caf5",
		Bg:         "#1a1b26",
		BgSurface:  "#292e42",
		BgOverlay:  "#16161e",
		BgCursor:   "#292e42",
		BgSearch:   "#e0af68",
		AccentPrimary:   "#7aa2f7",
		AccentSecondary: "#bb9af7",
		AccentInfo:      "#7dcfff",
		AccentSuccess:   "#9ece6a",
		AccentWarning:   "#e0af68",
		AccentDanger:    "#f7768e",
		DiffContext:     "#a9b1d6",
		BgDiffAdd:       "#1a2e1a",
		BgDiffDelete:    "#2e1a1e",
		BgHunkHeader:    "#292e42",
		BgActiveHunk:    "#1f2030",
		LabelFg:        "#1a1b26",
	}
}

// BuiltinThemes maps theme names to constructors.
var BuiltinThemes = map[string]func() Theme{
	"default":          DefaultTheme,
	"solarized-dark":   SolarizedDark,
	"solarized-light":  SolarizedLight,
	"catppuccin-mocha": CatppuccinMocha,
	"catppuccin-latte": CatppuccinLatte,
	"gruvbox-dark":     GruvboxDark,
	"gruvbox-light":    GruvboxLight,
	"dracula":          Dracula,
	"nord":             Nord,
	"tokyo-night":      TokyoNight,
}

// color converts a theme color string to a lipgloss.Color.
// Returns lipgloss.NoColor{} for empty strings (terminal default).
func color(s string) lipgloss.TerminalColor {
	if s == "" {
		return lipgloss.NoColor{}
	}
	return lipgloss.Color(s)
}

// Apply sets the active theme and re-derives all package-level styles.
func Apply(t Theme) {
	Current = t

	Primary = lipgloss.Color(t.AccentPrimary)
	Secondary = lipgloss.Color(t.AccentSecondary)
	Success = lipgloss.Color(t.AccentSuccess)
	Warning = lipgloss.Color(t.AccentWarning)
	Danger = lipgloss.Color(t.AccentDanger)
	Muted = lipgloss.Color(t.FgMuted)
	Cyan = lipgloss.Color(t.AccentInfo)

	Title = lipgloss.NewStyle().Bold(true).Foreground(color(t.AccentPrimary))
	Subtitle = lipgloss.NewStyle().Foreground(color(t.FgMuted))
	Selected = lipgloss.NewStyle().Bold(true).
		Foreground(color(t.FgEmphasis)).
		Background(color(t.AccentPrimary))
	StatusBar = lipgloss.NewStyle().
		Foreground(color(t.FgEmphasis)).
		Background(color(t.FgMuted)).
		Padding(0, 1)
	HelpStyle = lipgloss.NewStyle().Foreground(color(t.FgMuted))

	BorderStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(color(t.FgMuted))
	ActiveBorder = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(color(t.AccentPrimary))

	PRNumber = lipgloss.NewStyle().Foreground(color(t.AccentInfo)).Bold(true)
	PRAuthor = lipgloss.NewStyle().Foreground(color(t.AccentSecondary))
	PRDraft = lipgloss.NewStyle().Foreground(color(t.FgMuted)).Italic(true)
	LabelStyle = lipgloss.NewStyle().
		Foreground(color(t.LabelFg)).
		Background(color(t.AccentInfo)).
		Padding(0, 1)

	Addition = lipgloss.NewStyle().Foreground(color(t.AccentSuccess)).Background(color(t.BgDiffAdd))
	Deletion = lipgloss.NewStyle().Foreground(color(t.AccentDanger)).Background(color(t.BgDiffDelete))
	Context = lipgloss.NewStyle().Foreground(color(t.DiffContext))
	HunkHeader = lipgloss.NewStyle().Foreground(color(t.AccentInfo)).Background(color(t.BgHunkHeader))

	CommentBorder = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(color(t.AccentWarning)).
		Padding(0, 1)
	CommentAuthor = lipgloss.NewStyle().
		Foreground(color(t.AccentSecondary)).
		Bold(true)
}

// OverlayColors merges non-empty fields from a Colors override onto a base theme.
func OverlayColors(base Theme, overrides Theme) Theme {
	if overrides.Fg != "" {
		base.Fg = overrides.Fg
	}
	if overrides.FgMuted != "" {
		base.FgMuted = overrides.FgMuted
	}
	if overrides.FgEmphasis != "" {
		base.FgEmphasis = overrides.FgEmphasis
	}
	if overrides.Bg != "" {
		base.Bg = overrides.Bg
	}
	if overrides.BgSurface != "" {
		base.BgSurface = overrides.BgSurface
	}
	if overrides.BgOverlay != "" {
		base.BgOverlay = overrides.BgOverlay
	}
	if overrides.BgCursor != "" {
		base.BgCursor = overrides.BgCursor
	}
	if overrides.BgSearch != "" {
		base.BgSearch = overrides.BgSearch
	}
	if overrides.AccentPrimary != "" {
		base.AccentPrimary = overrides.AccentPrimary
	}
	if overrides.AccentSecondary != "" {
		base.AccentSecondary = overrides.AccentSecondary
	}
	if overrides.AccentInfo != "" {
		base.AccentInfo = overrides.AccentInfo
	}
	if overrides.AccentSuccess != "" {
		base.AccentSuccess = overrides.AccentSuccess
	}
	if overrides.AccentWarning != "" {
		base.AccentWarning = overrides.AccentWarning
	}
	if overrides.AccentDanger != "" {
		base.AccentDanger = overrides.AccentDanger
	}
	if overrides.DiffContext != "" {
		base.DiffContext = overrides.DiffContext
	}
	if overrides.BgDiffAdd != "" {
		base.BgDiffAdd = overrides.BgDiffAdd
	}
	if overrides.BgDiffDelete != "" {
		base.BgDiffDelete = overrides.BgDiffDelete
	}
	if overrides.BgHunkHeader != "" {
		base.BgHunkHeader = overrides.BgHunkHeader
	}
	if overrides.BgActiveHunk != "" {
		base.BgActiveHunk = overrides.BgActiveHunk
	}
	if overrides.LabelFg != "" {
		base.LabelFg = overrides.LabelFg
	}
	return base
}

func init() {
	Apply(DefaultTheme())
}
