package tabbar

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/jethrokuan/pry/internal/ui/styles"
)

// Tab represents a single tab in the bar.
type Tab struct {
	Label string
	Count int // -1 means don't show count
}

// Model is a horizontal tab bar with overflow indicators.
type Model struct {
	tabs   []Tab
	active int
	width  int
}

// New creates a new tab bar.
func New(tabs []Tab) Model {
	return Model{tabs: tabs}
}

// Active returns the active tab index.
func (m Model) Active() int {
	return m.active
}

// SetActive sets the active tab.
func (m *Model) SetActive(i int) {
	if i >= 0 && i < len(m.tabs) {
		m.active = i
	}
}

// SetWidth sets the available width for rendering.
func (m *Model) SetWidth(w int) {
	m.width = w
}

// SetCount updates the count for a specific tab.
func (m *Model) SetCount(i, count int) {
	if i >= 0 && i < len(m.tabs) {
		m.tabs[i].Count = count
	}
}

// SetTabs replaces all tabs.
func (m *Model) SetTabs(tabs []Tab) {
	m.tabs = tabs
	if m.active >= len(tabs) {
		m.active = 0
	}
}

// Len returns the number of tabs.
func (m Model) Len() int {
	return len(m.tabs)
}

// Next moves to the next tab. Returns true if the tab changed.
func (m *Model) Next() bool {
	if m.active < len(m.tabs)-1 {
		m.active++
		return true
	}
	return false
}

// Prev moves to the previous tab. Returns true if the tab changed.
func (m *Model) Prev() bool {
	if m.active > 0 {
		m.active--
		return true
	}
	return false
}

var (
	activeStyle = lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1)

	inactiveStyle = lipgloss.NewStyle().
			Foreground(lipgloss.BrightBlack).
			Padding(0, 1)

	separatorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.BrightBlack)

	overflowStyle = lipgloss.NewStyle().
			Foreground(lipgloss.BrightBlack)
)

// View renders the tab bar.
func (m Model) View() string {
	if len(m.tabs) == 0 {
		return ""
	}

	_ = styles.Primary // ensure styles package is referenced

	// Render all tab labels
	var rendered []string
	for i, tab := range m.tabs {
		label := tab.Label
		if tab.Count >= 0 {
			label = fmt.Sprintf("%s (%d)", label, tab.Count)
		}
		if i == m.active {
			rendered = append(rendered, activeStyle.Render(label))
		} else {
			rendered = append(rendered, inactiveStyle.Render(label))
		}
	}

	sep := separatorStyle.Render(" │ ")

	// Join all tabs with separators
	full := strings.Join(rendered, sep)
	fullWidth := lipgloss.Width(full)

	// If it fits, return directly
	if m.width <= 0 || fullWidth <= m.width {
		return full
	}

	// Overflow: render from the active tab outward until width is exhausted
	maxWidth := m.width - 4 // reserve space for overflow indicators
	maxWidth = max(maxWidth, 10)

	// Start with the active tab and expand outward
	left := m.active
	right := m.active
	result := rendered[m.active]
	currentWidth := lipgloss.Width(result)

	for {
		expanded := false

		// Try adding left
		if left > 0 {
			candidate := rendered[left-1] + sep + result
			if lipgloss.Width(candidate) <= maxWidth {
				result = candidate
				currentWidth = lipgloss.Width(result)
				left--
				expanded = true
			}
		}

		// Try adding right
		if right < len(rendered)-1 {
			candidate := result + sep + rendered[right+1]
			if lipgloss.Width(candidate) <= maxWidth {
				result = candidate
				currentWidth = lipgloss.Width(result)
				right++
				expanded = true
			}
		}

		if !expanded {
			break
		}
	}

	_ = currentWidth

	// Add overflow indicators
	var prefix, suffix string
	if left > 0 {
		prefix = overflowStyle.Render("← ")
	}
	if right < len(rendered)-1 {
		suffix = overflowStyle.Render(" →")
	}

	return prefix + result + suffix
}
