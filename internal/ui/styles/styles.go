package styles

import "github.com/charmbracelet/lipgloss"

// Color aliases — set by Apply().
var (
	Primary   lipgloss.Color
	Secondary lipgloss.Color
	Success   lipgloss.Color
	Warning   lipgloss.Color
	Danger    lipgloss.Color
	Muted     lipgloss.Color
	Cyan      lipgloss.Color
)

// Composed styles — set by Apply().
var (
	Title    lipgloss.Style
	Subtitle lipgloss.Style
	Selected lipgloss.Style
	StatusBar lipgloss.Style
	HelpStyle lipgloss.Style

	BorderStyle  lipgloss.Style
	ActiveBorder lipgloss.Style

	PRNumber   lipgloss.Style
	PRAuthor   lipgloss.Style
	PRDraft    lipgloss.Style
	LabelStyle lipgloss.Style

	Addition   lipgloss.Style
	Deletion   lipgloss.Style
	Context    lipgloss.Style
	HunkHeader lipgloss.Style

	CommentBorder lipgloss.Style
	CommentAuthor lipgloss.Style
)
