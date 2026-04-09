package submit

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jethrokuan/pry/internal/data"
	"github.com/jethrokuan/pry/internal/review"
	"github.com/jethrokuan/pry/internal/ui/styles"
)

// SubmittedMsg is sent when the review is successfully submitted.
type SubmittedMsg struct{}

// CancelledMsg is sent when submission is cancelled.
type CancelledMsg struct{}

type submitResultMsg struct {
	err error
}

type KeyMap struct {
	Submit  key.Binding
	Cancel  key.Binding
	Action1 key.Binding
	Action2 key.Binding
	Action3 key.Binding
}

var keys = KeyMap{
	Submit:  key.NewBinding(key.WithKeys("ctrl+enter"), key.WithHelp("ctrl+enter", "submit")),
	Cancel:  key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	Action1: key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "comment")),
	Action2: key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "approve")),
	Action3: key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "request changes")),
}

// Model is the submit review screen.
type Model struct {
	pr            *review.PullRequest
	pendingReview *review.PendingReview
	currentUser   string
	textarea      textarea.Model
	submitting    bool
	submitted     bool
	err           error
	width         int
	height        int
	spinner       spinner.Model
	focusBody     bool
	active        bool
}

// Active returns whether the submit modal is open.
func (m Model) Active() bool { return m.active }

// Open opens the submit modal and sizes it for the given terminal dimensions.
func (m *Model) Open(width, height int) {
	m.active = true
	m.width = width
	m.height = height
	popupW := min(width-4, 80)
	m.textarea.SetWidth(popupW - 6)
	m.textarea.SetHeight(5)
}

// Close closes the submit modal and resets transient state.
func (m *Model) Close() {
	m.active = false
	m.focusBody = false
	m.submitting = false
	m.submitted = false
	m.err = nil
	m.textarea.Blur()
}

// New creates a submit model.
func New(pr *review.PullRequest, currentUser string) Model {
	ta := textarea.New()
	ta.Placeholder = "Review summary (optional)..."
	ta.CharLimit = 0

	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		pr:            pr,
		pendingReview: pr.PendingReview,
		currentUser:   currentUser,
		textarea:      ta,
		spinner:       s,
	}
}

// Init initializes the model.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.textarea.SetWidth(msg.Width - 6)
		m.textarea.SetHeight(5)

	case submitResultMsg:
		m.submitting = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.submitted = true
			return m, func() tea.Msg { return SubmittedMsg{} }
		}

	case spinner.TickMsg:
		if m.submitting {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}

	case tea.KeyPressMsg:
		if m.submitting {
			return m, nil
		}

		if m.focusBody {
			switch {
			case key.Matches(msg, keys.Cancel):
				m.focusBody = false
				m.textarea.Blur()
				return m, nil
			case key.Matches(msg, keys.Submit):
				m.focusBody = false
				m.textarea.Blur()
				return m, nil
			}
			var cmd tea.Cmd
			m.textarea, cmd = m.textarea.Update(msg)
			return m, cmd
		}

		switch {
		case key.Matches(msg, keys.Cancel):
			return m, func() tea.Msg { return CancelledMsg{} }
		case key.Matches(msg, keys.Action1):
			m.pendingReview.Event = review.ReviewEventComment
		case key.Matches(msg, keys.Action2):
			m.pendingReview.Event = review.ReviewEventApprove
		case key.Matches(msg, keys.Action3):
			m.pendingReview.Event = review.ReviewEventRequestChanges
		case key.Matches(msg, key.NewBinding(key.WithKeys("b"))):
			m.focusBody = true
			m.textarea.Focus()
			return m, textarea.Blink
		case key.Matches(msg, keys.Submit):
			return m, m.submitReview()
		}
	}

	return m, nil
}

func (m *Model) submitReview() tea.Cmd {
	m.pendingReview.Body = m.textarea.Value()
	m.submitting = true
	pr := m.pr
	rev := m.pendingReview
	return tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			err := data.SubmitReview(pr, rev)
			return submitResultMsg{err: err}
		},
	)
}

// View renders the submit screen.
func (m Model) View() string {
	if m.width == 0 {
		return ""
	}

	var b strings.Builder

	b.WriteString(styles.Title.Render("Submit Review") + "\n\n")

	// Collect pending comments with their thread positions
	var pendingCount int
	var pendingLines []string
	for _, t := range m.pr.Threads {
		for _, c := range t.Comments {
			if c.IsPending && c.Author == m.currentUser {
				pendingCount++
				summary := truncate(c.Body, m.width-25)
				pendingLines = append(pendingLines, fmt.Sprintf("  %d. %s:%d - %s", pendingCount, t.Path, t.Line, summary))
			}
		}
	}

	b.WriteString(fmt.Sprintf("%d inline comments pending\n\n", pendingCount))
	for _, line := range pendingLines {
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")

	// Review body
	b.WriteString("Review body (press 'b' to edit):\n")
	bodyStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.Muted).
		Padding(0, 1)
	if m.focusBody {
		bodyStyle = bodyStyle.BorderForeground(styles.Primary)
	}
	b.WriteString(bodyStyle.Render(m.textarea.View()) + "\n\n")

	// Action selector
	b.WriteString("Action:\n")
	actions := []struct {
		event review.ReviewEvent
		label string
	}{
		{review.ReviewEventComment, "Comment"},
		{review.ReviewEventApprove, "Approve"},
		{review.ReviewEventRequestChanges, "Request Changes"},
	}

	for i, a := range actions {
		radio := "( )"
		if m.pendingReview.Event == a.event {
			radio = "(x)"
		}
		actionStyle := lipgloss.NewStyle()
		switch a.event {
		case review.ReviewEventApprove:
			actionStyle = lipgloss.NewStyle().Foreground(styles.Success)
		case review.ReviewEventRequestChanges:
			actionStyle = lipgloss.NewStyle().Foreground(styles.Danger)
		}
		b.WriteString(fmt.Sprintf("  %d. %s %s\n", i+1, radio, actionStyle.Render(a.label)))
	}

	b.WriteString("\n")

	if m.submitting {
		b.WriteString(m.spinner.View() + " Submitting review...\n")
	} else if m.err != nil {
		b.WriteString(lipgloss.NewStyle().Foreground(styles.Danger).
			Render(fmt.Sprintf("Error: %v", m.err)) + "\n")
		b.WriteString("Press ctrl+enter to retry or esc to cancel\n")
	}

	// Footer
	help := "1/2/3 action  b edit body  ctrl+enter submit  esc cancel"
	b.WriteString(styles.HelpStyle.Render(help))

	return b.String()
}

// RenderPopup renders the submit form as a bordered popup suitable for overlay.
func (m Model) RenderPopup() string {
	popupW := min(m.width-4, 80)

	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(styles.Primary).Render("Submit Review") + "\n\n")

	// Pending comments
	var pendingCount int
	for _, t := range m.pr.Threads {
		for _, c := range t.Comments {
			if c.IsPending && c.Author == m.currentUser {
				pendingCount++
			}
		}
	}
	b.WriteString(fmt.Sprintf("%d inline comments pending\n\n", pendingCount))

	// Review body
	b.WriteString("Review body (press 'b' to edit):\n")
	bodyStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.Muted).
		Padding(0, 1)
	if m.focusBody {
		bodyStyle = bodyStyle.BorderForeground(styles.Primary)
	}
	b.WriteString(bodyStyle.Render(m.textarea.View()) + "\n\n")

	// Action selector
	b.WriteString("Action:\n")
	actions := []struct {
		event review.ReviewEvent
		label string
	}{
		{review.ReviewEventComment, "Comment"},
		{review.ReviewEventApprove, "Approve"},
		{review.ReviewEventRequestChanges, "Request Changes"},
	}
	for i, a := range actions {
		radio := "( )"
		if m.pendingReview.Event == a.event {
			radio = "(x)"
		}
		actionStyle := lipgloss.NewStyle()
		switch a.event {
		case review.ReviewEventApprove:
			actionStyle = lipgloss.NewStyle().Foreground(styles.Success)
		case review.ReviewEventRequestChanges:
			actionStyle = lipgloss.NewStyle().Foreground(styles.Danger)
		}
		b.WriteString(fmt.Sprintf("  %d. %s %s\n", i+1, radio, actionStyle.Render(a.label)))
	}
	b.WriteString("\n")

	if m.submitting {
		b.WriteString(m.spinner.View() + " Submitting review...\n")
	} else if m.err != nil {
		b.WriteString(lipgloss.NewStyle().Foreground(styles.Danger).
			Render(fmt.Sprintf("Error: %v", m.err)) + "\n")
		b.WriteString("Press ctrl+enter to retry or esc to cancel\n")
	}

	help := "1/2/3 action  b edit body  ctrl+enter submit  esc cancel"
	b.WriteString(styles.HelpStyle.Render(help))

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.Primary).
		Padding(1, 2).
		Width(popupW)

	return boxStyle.Render(b.String())
}

func truncate(s string, max int) string {
	lines := strings.SplitN(s, "\n", 2)
	s = lines[0]
	if len(s) > max && max > 3 {
		return s[:max-3] + "..."
	}
	return s
}
