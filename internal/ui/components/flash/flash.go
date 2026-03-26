// Package flash provides a stackable flash message system for the root model.
// Screens add/remove flashes via typed messages; the root model owns the state.
package flash

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jethrokuan/pry/internal/ui/styles"
)

// Style controls how a flash is rendered.
type Style int

const (
	StyleSuccess Style = iota // green, auto-expires
	StyleInfo                 // muted, auto-expires
	StyleSpinner              // animated spinner prefix, no auto-expiry
	StyleDanger               // danger/red, auto-expires
)

// item is a single flash entry.
type item struct {
	ID    string
	Text  string
	Style Style
}

// ShowMsg asks the root model to display a flash. If an entry with the same
// ID already exists it is replaced (moved to top).
type ShowMsg struct {
	ID      string
	Text    string
	Style   Style
	Expires time.Duration // 0 = no auto-expiry (must be dismissed manually)
}

// Cmd returns a tea.Cmd that emits this ShowMsg.
func (s ShowMsg) Cmd() tea.Cmd {
	return func() tea.Msg { return s }
}

// DismissMsg removes the flash with the given ID.
type DismissMsg struct {
	ID string
}

// Cmd returns a tea.Cmd that emits this DismissMsg.
func (d DismissMsg) Cmd() tea.Cmd {
	return func() tea.Msg { return d }
}

// expiredMsg is internal — auto-fires when a timed flash expires.
type expiredMsg struct {
	ID string
}

// Model holds the flash stack. Newer items are at the end (rendered on top).
type Model struct {
	items   []item
	spinner spinner.Model
}

// New creates an empty flash model.
func New() Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{spinner: s}
}

// Update handles flash-related messages. Call this from the root Update.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ShowMsg:
		// Remove existing entry with same ID (if any).
		m.items = removeByID(m.items, msg.ID)
		// Append to end (newest = top when rendered).
		m.items = append(m.items, item{
			ID:    msg.ID,
			Text:  msg.Text,
			Style: msg.Style,
		})
		var cmds []tea.Cmd
		if msg.Expires > 0 {
			id := msg.ID
			cmds = append(cmds, tea.Tick(msg.Expires, func(time.Time) tea.Msg {
				return expiredMsg{ID: id}
			}))
		}
		if msg.Style == StyleSpinner {
			cmds = append(cmds, m.spinner.Tick)
		}
		return m, tea.Batch(cmds...)

	case DismissMsg:
		m.items = removeByID(m.items, msg.ID)
		return m, nil

	case expiredMsg:
		m.items = removeByID(m.items, msg.ID)
		return m, nil

	case spinner.TickMsg:
		if m.hasSpinner() {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

// View renders the flash stack as a styled overlay box.
// Returns empty string when there are no flashes.
func (m Model) View() string {
	if len(m.items) == 0 {
		return ""
	}

	lines := make([]string, len(m.items))
	// Reverse order: newest (last in slice) rendered first.
	for i, it := range m.items {
		ri := len(m.items) - 1 - i
		var rendered string
		switch it.Style {
		case StyleSpinner:
			rendered = m.spinner.View() + " " + it.Text
		case StyleSuccess:
			rendered = lipgloss.NewStyle().Foreground(styles.Success).Bold(true).Render(it.Text)
		case StyleDanger:
			rendered = lipgloss.NewStyle().Foreground(styles.Danger).Bold(true).Render(it.Text)
		default:
			rendered = it.Text
		}
		lines[ri] = rendered
	}

	content := strings.Join(lines, "\n")

	// Wrap in a styled overlay box.
	box := lipgloss.NewStyle().
		Background(styles.BgSelected).
		Foreground(lipgloss.BrightWhite).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.Info).
		BorderBackground(styles.BgSelected).
		Padding(0, 2).
		Bold(true).
		Render(content)

	return box
}

// Empty returns true when there are no flash messages.
func (m Model) Empty() bool {
	return len(m.items) == 0
}

func (m Model) hasSpinner() bool {
	for _, it := range m.items {
		if it.Style == StyleSpinner {
			return true
		}
	}
	return false
}

func removeByID(items []item, id string) []item {
	n := 0
	for _, it := range items {
		if it.ID != id {
			items[n] = it
			n++
		}
	}
	return items[:n]
}
