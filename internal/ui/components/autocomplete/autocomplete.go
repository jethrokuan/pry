// Package autocomplete provides a reusable dropdown suggestion component
// for use with textinput.Model or textarea.Model in Bubble Tea applications.
package autocomplete

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jethrokuan/pry/internal/ui/components/scrollbar"
	"github.com/jethrokuan/pry/internal/ui/styles"
)

// Suggestion represents a single autocomplete entry.
type Suggestion struct {
	Value string // text to insert on completion
	Label string // display text in dropdown (if empty, uses Value)
}

func (s Suggestion) label() string {
	if s.Label != "" {
		return s.Label
	}
	return s.Value
}

// Model manages autocomplete dropdown state.
type Model struct {
	suggestions []Suggestion
	filtered    []Suggestion
	cursor      int
	offset      int // index of the first visible item
	active      bool
	maxVisible  int
}

// New creates a new autocomplete model.
func New() Model {
	return Model{maxVisible: 5}
}

// SetSuggestions replaces the full suggestion list.
func (m *Model) SetSuggestions(s []Suggestion) {
	m.suggestions = s
}

// IsActive returns true if the dropdown is showing with matches.
func (m *Model) IsActive() bool {
	return m.active && len(m.filtered) > 0
}

// Selected returns the currently highlighted suggestion.
// Only valid when IsActive() is true.
func (m *Model) Selected() Suggestion {
	if m.cursor < len(m.filtered) {
		return m.filtered[m.cursor]
	}
	return Suggestion{}
}

// Show filters suggestions by prefix and activates the dropdown.
// An empty prefix shows all suggestions. Matching is case-insensitive
// and checks both Value and Label using substring matching.
func (m *Model) Show(prefix string) {
	prefix = strings.ToLower(prefix)
	m.filtered = m.filtered[:0]
	for _, s := range m.suggestions {
		if prefix == "" ||
			strings.Contains(strings.ToLower(s.Value), prefix) ||
			(s.Label != "" && strings.Contains(strings.ToLower(s.Label), prefix)) {
			m.filtered = append(m.filtered, s)
		}
	}
	m.active = len(m.filtered) > 0
	if m.cursor >= len(m.filtered) {
		m.cursor = 0
	}
	m.ensureCursorVisible()
}

// Hide dismisses the dropdown and resets cursor.
func (m *Model) Hide() {
	m.active = false
	m.cursor = 0
	m.offset = 0
}

// HandleKey processes a key event. Returns true if the key was consumed.
// When tab/enter is pressed and the dropdown is active, the caller should
// read Selected() and perform the text insertion.
func (m *Model) HandleKey(keyStr string) (consumed bool, selected bool) {
	if !m.IsActive() {
		return false, false
	}
	switch keyStr {
	case "down", "ctrl+n":
		m.cursor = (m.cursor + 1) % len(m.filtered)
		m.ensureCursorVisible()
		return true, false
	case "up", "ctrl+p":
		m.cursor--
		if m.cursor < 0 {
			m.cursor = len(m.filtered) - 1
		}
		m.ensureCursorVisible()
		return true, false
	case "tab", "enter":
		return true, true
	case "esc":
		m.Hide()
		return true, false
	}
	return false, false
}

// ensureCursorVisible adjusts the scroll offset so the cursor is in the visible window.
func (m *Model) ensureCursorVisible() {
	if m.cursor < m.offset {
		m.offset = m.cursor
	} else if m.cursor >= m.offset+m.maxVisible {
		m.offset = m.cursor - m.maxVisible + 1
	}
}

// View renders the autocomplete dropdown rows (without border).
func (m Model) View() string {
	if !m.active || len(m.filtered) == 0 {
		return ""
	}

	total := len(m.filtered)
	numVisible := total
	if numVisible > m.maxVisible {
		numVisible = m.maxVisible
	}

	// Measure the widest label across ALL filtered items, capped at maxRowWidth.
	// Add padding (1 each side) to the width since lipgloss Width includes padding.
	const maxRowWidth = 50
	contentWidth := 0
	for _, s := range m.filtered {
		if w := lipgloss.Width(s.label()); w > contentWidth {
			contentWidth = w
		}
	}
	if contentWidth > maxRowWidth {
		contentWidth = maxRowWidth
	}
	rowWidth := contentWidth + 2 // +2 for horizontal padding

	// Use BgSelected for highlighted row (matches PR list cursor)
	selectedRow := lipgloss.NewStyle().
		Background(styles.BgSelected).
		Bold(true).
		Width(rowWidth).
		Padding(0, 1)
	normalRow := lipgloss.NewStyle().
		Width(rowWidth).
		Padding(0, 1)

	// Render the visible window
	var rows []string
	end := m.offset + numVisible
	if end > total {
		end = total
	}
	for i := m.offset; i < end; i++ {
		label := m.filtered[i].label()
		if i == m.cursor {
			rows = append(rows, selectedRow.Render(label))
		} else {
			rows = append(rows, normalRow.Render(label))
		}
	}

	content := strings.Join(rows, "\n")

	// Add scrollbar if there are more items than visible
	if total > m.maxVisible {
		sb := scrollbar.New()
		sb.Height = numVisible
		sb.TotalItems = total
		sb.VisibleItems = numVisible
		sb.Offset = m.offset
		sb.ThumbColor = styles.Primary
		content = lipgloss.JoinHorizontal(lipgloss.Top, content, sb.View())
	}

	return styles.BorderStyle.Render(content)
}

// Overlay composites the autocomplete dropdown on top of base at the given
// position using lipgloss layer compositing. Returns base unchanged if inactive.
func (m Model) Overlay(base string, pos tea.Position) string {
	content := m.View()
	if content == "" {
		return base
	}

	baseLayer := lipgloss.NewLayer(base)
	popupLayer := lipgloss.NewLayer(content).X(pos.X).Y(pos.Y).Z(1)
	return lipgloss.NewCompositor(baseLayer, popupLayer).Render()
}

