package submit

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jethrokuan/pry/internal/appctx"
	"github.com/jethrokuan/pry/internal/review"
	"github.com/jethrokuan/pry/internal/ui/styles"
)

type waitForSyncMsg struct{}

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
	Submit:  key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "submit")),
	Cancel:  key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	Action1: key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "comment")),
	Action2: key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "approve")),
	Action3: key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "request changes")),
}

// Model is the submit review screen.
type Model struct {
	ctx *appctx.Context
	pr  *review.PullRequest
	textarea       textarea.Model
	submitting     bool
	submitted      bool
	waitingForSync bool
	err            error
	width          int
	height         int
	spinner        spinner.Model
	focusBody      bool
}

// New creates a submit model.
func New(ctx *appctx.Context, pr *review.PullRequest) Model {
	ta := textarea.New()
	ta.Placeholder = "Review summary (optional)..."
	ta.CharLimit = 0

	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		ctx:      ctx,
		pr:       pr,
		textarea: ta,
		spinner:  s,
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

	case waitForSyncMsg:
		if m.pr.PendingReview.InFlightCount() == 0 {
			// All syncs complete, now submit
			return m, m.submitReview()
		}
		// Still waiting, check again
		return m, tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg {
			return waitForSyncMsg{}
		})

	case submitResultMsg:
		m.submitting = false
		m.waitingForSync = false
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
			m.pr.PendingReview.Event = review.ReviewEventComment
		case key.Matches(msg, keys.Action2):
			m.pr.PendingReview.Event = review.ReviewEventApprove
		case key.Matches(msg, keys.Action3):
			m.pr.PendingReview.Event = review.ReviewEventRequestChanges
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
	m.pr.PendingReview.Body = m.textarea.Value()

	// Wait for any in-flight syncs to complete before submitting
	if m.pr.PendingReview.InFlightCount() > 0 {
		m.waitingForSync = true
		return tea.Batch(
			m.spinner.Tick,
			tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg {
				return waitForSyncMsg{}
			}),
		)
	}

	m.submitting = true
	m.waitingForSync = false
	return tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			err := m.ctx.Svc.SubmitReview(context.Background(), m.pr, m.pr.PendingReview)
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

	// Comment count
	b.WriteString(fmt.Sprintf("%d inline comments pending\n\n",
		len(m.pr.PendingReview.Comments)))

	// Show comment summaries with sync status
	for i, c := range m.pr.PendingReview.Comments {
		syncIcon := " "
		switch c.SyncStatus {
		case review.SyncComplete:
			syncIcon = "✓"
		case review.SyncInFlight:
			syncIcon = "..."
		case review.SyncFailed:
			syncIcon = "✗"
		case review.SyncPending:
			syncIcon = "○"
		}
		summary := truncate(c.Body, m.width-25)
		b.WriteString(fmt.Sprintf("  %s %d. %s:%d - %s\n", syncIcon, i+1, c.Path, c.Line, summary))
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
		if m.pr.PendingReview.Event == a.event {
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

	// Show sync failure warnings
	failedComments := 0
	for _, c := range m.pr.PendingReview.Comments {
		if c.SyncStatus == review.SyncFailed {
			failedComments++
		}
	}
	if failedComments > 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(styles.Danger).
			Render(fmt.Sprintf("Warning: %d comment(s) failed to sync to GitHub", failedComments)) + "\n")
		b.WriteString(lipgloss.NewStyle().Foreground(styles.Muted).
			Render("These comments will be included in the batch submit as fallback.") + "\n\n")
	}

	if m.waitingForSync {
		b.WriteString(m.spinner.View() + " Waiting for comments to sync...\n")
	} else if m.submitting {
		b.WriteString(m.spinner.View() + " Submitting review...\n")
	} else if m.err != nil {
		b.WriteString(lipgloss.NewStyle().Foreground(styles.Danger).
			Render(fmt.Sprintf("Error: %v", m.err)) + "\n")
		b.WriteString("Press ctrl+s to retry or esc to cancel\n")
	}

	// Footer
	help := "1/2/3 action  b edit body  ctrl+s submit  esc cancel"
	b.WriteString(styles.HelpStyle.Render(help))

	return b.String()
}

func truncate(s string, max int) string {
	lines := strings.SplitN(s, "\n", 2)
	s = lines[0]
	if len(s) > max && max > 3 {
		return s[:max-3] + "..."
	}
	return s
}
