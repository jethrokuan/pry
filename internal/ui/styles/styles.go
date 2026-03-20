package styles

import "charm.land/lipgloss/v2"

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
