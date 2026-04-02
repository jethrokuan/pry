package styles

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Semantic color aliases — map roles to ANSI palette colors.
// These adapt to the user's terminal color scheme automatically.
var (
	Primary   color.Color = lipgloss.Blue
	Secondary color.Color = lipgloss.Magenta
	Success   color.Color = lipgloss.Green
	Warning   color.Color = lipgloss.Yellow
	Danger    color.Color = lipgloss.Red
	Muted     color.Color = lipgloss.BrightBlack
	Info      color.Color = lipgloss.Cyan
	Cyan      color.Color = lipgloss.Cyan // alias for backward compat
)

// Semantic background colors.
var (
	BgCursor     color.Color = lipgloss.BrightBlack
	BgSelected   color.Color
	BgSearch     color.Color = lipgloss.Yellow
	BgHunk       color.Color
	BgDiffAdd    color.Color
	BgDiffDelete color.Color
	BgSurface    color.Color = lipgloss.BrightBlack
	BgOverlay    color.Color = lipgloss.Black
	BgHunkHeader color.Color = lipgloss.BrightBlack
	DiffContext  color.Color = lipgloss.White
	LabelFg      color.Color = lipgloss.Black
)

// tint produces a subtle background tint from a base ANSI color.
// On dark themes it darkens; on light themes it lightens.
func tint(base color.Color, isDark bool, amount float64) color.Color {
	if isDark {
		return lipgloss.Darken(base, amount)
	}
	return lipgloss.Lighten(base, amount)
}

// Apply sets up the composed Lipgloss styles from the ANSI palette.
// isDark should be true for dark terminal backgrounds, false for light.
// bg is the detected terminal background color (may be nil).
func Apply(isDark bool, bg color.Color) {
	if bg != nil {
		BgSelected = tint(bg, !isDark, 0.10)
		BgHunk = tint(bg, !isDark, 0.01)
		BgOverlay = tint(bg, !isDark, 0.15)
	} else {
		BgSelected = tint(lipgloss.Blue, isDark, 0.20)
		BgHunk = tint(lipgloss.BrightBlack, isDark, 0.20)
	}
	BgDiffAdd = tint(lipgloss.Green, isDark, 0.50)
	BgDiffDelete = tint(lipgloss.Red, isDark, 0.50)

	Title = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Blue)
	Subtitle = lipgloss.NewStyle().Foreground(lipgloss.BrightBlack)
	StatusBar = lipgloss.NewStyle().
		Foreground(lipgloss.BrightWhite).Background(lipgloss.BrightBlack).Padding(0, 1)
	HelpStyle = lipgloss.NewStyle().Foreground(lipgloss.BrightBlack)

	BorderStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.BrightBlack)
	ActiveBorder = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Blue)

	PRNumber = lipgloss.NewStyle().Foreground(lipgloss.Cyan).Bold(true)
	PRAuthor = lipgloss.NewStyle().Foreground(lipgloss.Magenta)
	PRDraft = lipgloss.NewStyle().Foreground(lipgloss.BrightBlack).Italic(true)
	LabelStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Black).Background(lipgloss.Cyan).Padding(0, 1)

	Addition = lipgloss.NewStyle().Foreground(lipgloss.Green).Background(BgDiffAdd)
	Deletion = lipgloss.NewStyle().Foreground(lipgloss.Red).Background(BgDiffDelete)
	Context = lipgloss.NewStyle().Foreground(lipgloss.White)
	HunkHeader = lipgloss.NewStyle().Foreground(lipgloss.Cyan).Background(lipgloss.BrightBlack)

	CommentBorder = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Yellow).Padding(0, 1)
	CommentAuthor = lipgloss.NewStyle().Foreground(lipgloss.Magenta).Bold(true)
}

func init() {
	Apply(true, nil) // default to dark; re-applied when terminal background is detected
}
