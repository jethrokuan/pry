package sidebar

import (
	"fmt"

	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"

	"github.com/jethrokuan/pry/internal/ui/styles"
)

// Model is a sidebar panel with a scrollable viewport.
type Model struct {
	viewport       viewport.Model
	width          int
	height         int
	ready          bool
	pendingContent string // buffered content set before viewport is initialized
}

// New creates a new sidebar model.
func New() Model {
	return Model{}
}

// SetSize updates the sidebar dimensions and initializes the viewport.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
	contentWidth := max(width-3, 1) // left border + padding
	if !m.ready {
		m.viewport = viewport.New(
			viewport.WithWidth(contentWidth),
			viewport.WithHeight(height),
		)
		m.ready = true
		if m.pendingContent != "" {
			m.viewport.SetContent(m.pendingContent)
			m.pendingContent = ""
		}
	} else {
		m.viewport.SetWidth(contentWidth)
		m.viewport.SetHeight(height)
	}
}

// SetContent sets the sidebar content.
func (m *Model) SetContent(content string) {
	if !m.ready {
		m.pendingContent = content
		return
	}
	m.viewport.SetContent(content)
}

// ScrollDown scrolls down by n lines.
func (m *Model) ScrollDown(n int) {
	m.viewport.ScrollDown(n)
}

// ScrollUp scrolls up by n lines.
func (m *Model) ScrollUp(n int) {
	m.viewport.ScrollUp(n)
}

// View renders the sidebar.
func (m Model) View() string {
	if m.width == 0 {
		return ""
	}

	borderStyle := lipgloss.NewStyle().
		BorderLeft(true).
		BorderStyle(lipgloss.Border{Left: "│"}).
		BorderForeground(styles.Muted).
		Width(m.width).
		Height(m.height)

	// Show scroll percentage
	pct := int(m.viewport.ScrollPercent() * 100)
	pager := lipgloss.NewStyle().
		Foreground(styles.Muted).
		Bold(true).
		Render(fmt.Sprintf(" %d%%", pct))

	content := m.viewport.View()
	if m.height > 1 {
		return borderStyle.Height(m.height - 1).Render(content) + "\n" + pager
	}
	return borderStyle.Render(content)
}
