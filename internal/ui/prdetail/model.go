package prdetail

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/jkuan/pr-review/internal/review"
	"github.com/jkuan/pr-review/internal/ui/mdutil"
	"github.com/jkuan/pr-review/internal/ui/styles"
)

// StartReviewMsg signals to transition to the diff view.
type StartReviewMsg struct {
	PR review.PullRequest
}

// CheckoutMsg signals to checkout the PR.
type CheckoutMsg struct {
	PR review.PullRequest
}

// BackMsg signals to go back to the PR list.
type BackMsg struct{}

type KeyMap struct {
	Review   key.Binding
	Checkout key.Binding
	Web      key.Binding
	Back     key.Binding
	Quit     key.Binding
	Up       key.Binding
	Down     key.Binding
}

var keys = KeyMap{
	Review:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "review diffs")),
	Checkout: key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "checkout")),
	Web:      key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "open in web")),
	Back:     key.NewBinding(key.WithKeys("esc", "backspace"), key.WithHelp("esc", "back")),
	Quit:     key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
	Up:       key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "scroll up")),
	Down:     key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "scroll down")),
}

// Model is the PR detail screen model.
type Model struct {
	pr              review.PullRequest
	viewport        viewport.Model
	ready           bool
	bodyLoaded      bool
	width           int
	height          int
	checkoutErr     error
	checkoutSuccess bool
}

// SetCheckoutErr sets the checkout error to display.
func (m *Model) SetCheckoutErr(err error) {
	m.checkoutErr = err
	m.checkoutSuccess = false
}

// SetCheckoutSuccess marks the checkout as successful.
func (m *Model) SetCheckoutSuccess() {
	m.checkoutErr = nil
	m.checkoutSuccess = true
}

// SetPR updates the PR data and marks the body as loaded.
func (m *Model) SetPR(pr review.PullRequest) {
	m.pr = pr
	m.bodyLoaded = true
	if m.ready {
		m.viewport.SetContent(m.renderBody())
	}
}

// New creates a new PR detail model.
func New(pr review.PullRequest) Model {
	return Model{
		pr: pr,
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

		headerHeight := 8 // lines for header info
		footerHeight := 3

		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-headerHeight-footerHeight)
			if m.bodyLoaded {
				m.viewport.SetContent(m.renderBody())
			}
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - headerHeight - footerHeight
		}

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Back):
			return m, func() tea.Msg { return BackMsg{} }
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, keys.Review):
			return m, func() tea.Msg { return StartReviewMsg{PR: m.pr} }
		case key.Matches(msg, keys.Checkout):
			return m, func() tea.Msg { return CheckoutMsg{PR: m.pr} }
		case key.Matches(msg, keys.Web):
			return m, tea.ExecProcess(exec.Command("open", m.pr.URL), nil)
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m Model) renderBody() string {
	if m.pr.Body == "" {
		return "No description provided."
	}
	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(m.width-4),
	)
	if err != nil {
		return m.pr.Body
	}
	rendered, err := renderer.Render(mdutil.ReplaceImages(m.pr.Body))
	if err != nil {
		return m.pr.Body
	}
	return rendered
}

// View renders the PR detail.
func (m Model) View() string {
	if m.width == 0 {
		return "Loading PR details...\n"
	}

	var b strings.Builder

	// Header
	title := styles.Title.Render(fmt.Sprintf("PR #%d: %s", m.pr.Number, m.pr.Title))
	b.WriteString(title + "\n")

	meta := fmt.Sprintf("%s → %s  |  +%d/-%d  |  %d files",
		styles.PRAuthor.Render("@"+m.pr.Author),
		m.pr.Base,
		m.pr.Additions,
		m.pr.Deletions,
		m.pr.Files,
	)
	b.WriteString(meta + "\n")

	// Labels
	if len(m.pr.Labels) > 0 {
		var labels []string
		for _, l := range m.pr.Labels {
			labels = append(labels, styles.LabelStyle.Render(l))
		}
		b.WriteString("Labels: " + strings.Join(labels, " ") + "\n")
	}

	// Review status
	reviewStatus := "pending"
	reviewStyle := lipgloss.NewStyle().Foreground(styles.Warning)
	switch m.pr.ReviewDecision {
	case "APPROVED":
		reviewStatus = "approved"
		reviewStyle = lipgloss.NewStyle().Foreground(styles.Success)
	case "CHANGES_REQUESTED":
		reviewStatus = "changes requested"
		reviewStyle = lipgloss.NewStyle().Foreground(styles.Danger)
	}
	b.WriteString("Review: " + reviewStyle.Render(reviewStatus) + "\n")
	b.WriteString(strings.Repeat("─", m.width) + "\n")

	// Body
	if !m.bodyLoaded {
		b.WriteString(styles.HelpStyle.Render("Loading PR details..."))
	} else if m.ready {
		b.WriteString(m.viewport.View())
	}

	// Checkout status
	if m.checkoutErr != nil {
		b.WriteString(lipgloss.NewStyle().Foreground(styles.Danger).
			Render(fmt.Sprintf("Checkout failed: %v", m.checkoutErr)) + "\n")
	} else if m.checkoutSuccess {
		b.WriteString(lipgloss.NewStyle().Foreground(styles.Success).
			Render("Checked out successfully") + "\n")
	}

	// Footer
	b.WriteString("\n")
	help := "enter review  c checkout  w web  esc back  ctrl+c quit"
	b.WriteString(styles.HelpStyle.Render(help))

	return b.String()
}
