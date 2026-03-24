package sidebar

import (
	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"

	"github.com/jethrokuan/pry/internal/ui/components/scrollbar"
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
	contentWidth := max(width-6, 1)  // left border(1) + left pad(1) + right pad(1) + border width(2) + scrollbar(1)
	contentHeight := max(height-2, 1) // top pad(1) + bottom pad(1)
	if !m.ready {
		m.viewport = viewport.New(
			viewport.WithWidth(contentWidth),
			viewport.WithHeight(contentHeight),
		)
		m.ready = true
		if m.pendingContent != "" {
			m.viewport.SetContent(m.pendingContent)
			m.pendingContent = ""
		}
	} else {
		m.viewport.SetWidth(contentWidth)
		m.viewport.SetHeight(contentHeight)
	}
}

// SetContent sets the sidebar content.
func (m *Model) SetContent(content string) {
	if !m.ready {
		m.pendingContent = content
		return
	}
	m.viewport.SetContent(content)
	m.viewport.GotoTop()
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
		Padding(1).
		Width(m.width).
		Height(m.height)

	content := m.viewport.View()

	// Render scrollbar alongside content
	sb := scrollbar.New()
	sb.Height = m.viewport.Height()
	sb.TotalItems = m.viewport.TotalLineCount()
	sb.VisibleItems = m.viewport.Height()
	sb.Offset = m.viewport.YOffset()
	sb.ThumbColor = styles.Primary

	sbView := sb.View()
	if sbView != "" {
		content = lipgloss.JoinHorizontal(lipgloss.Top, content, sbView)
	}

	return borderStyle.Render(content)
}
