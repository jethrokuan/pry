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
	BgSearch     color.Color = lipgloss.Yellow
	// Subtle background tints — hardcoded hex since ANSI 16 has no in-between shades.
	// These are dark enough to work on any dark terminal theme.
	BgActiveHunk color.Color = lipgloss.Color("#1a1a2e")
	BgDiffAdd    color.Color = lipgloss.Color("#012800")
	BgDiffDelete color.Color = lipgloss.Color("#340001")
	BgSurface    color.Color = lipgloss.BrightBlack
	BgOverlay    color.Color = lipgloss.Black
	BgHunkHeader color.Color = lipgloss.BrightBlack
	DiffContext  color.Color = lipgloss.White
	LabelFg      color.Color = lipgloss.Black
)

// Apply sets up the composed Lipgloss styles from the ANSI palette.
func Apply() {
	Title = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Blue)
	Subtitle = lipgloss.NewStyle().Foreground(lipgloss.BrightBlack)
	Selected = lipgloss.NewStyle().Bold(true).
		Foreground(lipgloss.BrightWhite).Background(lipgloss.Blue)
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
	Apply()
}
