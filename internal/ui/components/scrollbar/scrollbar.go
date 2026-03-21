// Package scrollbar provides a generic vertical scrollbar component
// for Bubble Tea TUI applications.
package scrollbar

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// Model is a generic vertical scrollbar that can be used with any
// scrollable list or viewport. It renders a single-column track with
// a thumb indicating the current scroll position.
type Model struct {
	// Height is the total height (in terminal rows) available for the scrollbar.
	Height int

	// TotalItems is the total number of items in the list.
	TotalItems int

	// VisibleItems is the number of items visible at once.
	VisibleItems int

	// Offset is the index of the first visible item.
	Offset int

	// TrackColor is the color of the scrollbar track (background).
	TrackColor color.Color

	// ThumbColor is the color of the scrollbar thumb (indicator).
	ThumbColor color.Color

	// TrackChar is the character used for the track. Defaults to " ".
	TrackChar string

	// ThumbChar is the character used for the thumb. Defaults to "┃".
	ThumbChar string
}

// New creates a scrollbar with sensible defaults.
func New() Model {
	return Model{
		TrackColor: lipgloss.BrightBlack,
		ThumbColor: lipgloss.White,
		TrackChar:  " ",
		ThumbChar:  "┃",
	}
}

// View renders the scrollbar as a single-column string with newlines.
// Returns an empty string if all items fit on screen (no scrolling needed).
func (m Model) View() string {
	if m.Height <= 0 || m.TotalItems <= 0 || m.TotalItems <= m.VisibleItems {
		return ""
	}

	trackStyle := lipgloss.NewStyle().Foreground(m.TrackColor)
	thumbStyle := lipgloss.NewStyle().Foreground(m.ThumbColor)

	// Calculate thumb size: proportional to visible/total ratio, minimum 1 row.
	thumbSize := m.Height * m.VisibleItems / m.TotalItems
	thumbSize = max(thumbSize, 1)
	if thumbSize >= m.Height {
		thumbSize = m.Height - 1
	}

	// Calculate thumb position.
	scrollRange := m.TotalItems - m.VisibleItems
	trackRange := m.Height - thumbSize
	thumbStart := 0
	if scrollRange > 0 {
		thumbStart = m.Offset * trackRange / scrollRange
	}
	if thumbStart+thumbSize > m.Height {
		thumbStart = m.Height - thumbSize
	}

	lines := make([]string, m.Height)
	for i := range m.Height {
		if i >= thumbStart && i < thumbStart+thumbSize {
			lines[i] = thumbStyle.Render(m.ThumbChar)
		} else {
			lines[i] = trackStyle.Render(m.TrackChar)
		}
	}
	return strings.Join(lines, "\n")
}
