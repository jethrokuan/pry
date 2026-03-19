package prlist

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jkuan/pr-review/internal/review"
	"github.com/jkuan/pr-review/internal/ui/styles"
)

// PRSelectedMsg is sent when the user selects a PR.
type PRSelectedMsg struct {
	PR review.PullRequest
}

// FilterChangedMsg is sent when the filter changes.
type FilterChangedMsg struct{}

type prsLoadedMsg struct {
	prs []review.PullRequest
	err error
}

// KeyMap defines the key bindings for the PR list.
type KeyMap struct {
	Up      key.Binding
	Down    key.Binding
	Select  key.Binding
	Filter  key.Binding
	Refresh key.Binding
	Quit key.Binding
	Help    key.Binding
}

var keys = KeyMap{
	Up:      key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:    key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	Select:  key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
	Filter:  key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "filter")),
	Refresh: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
	Quit:    key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	Help:    key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
}

// Model is the Bubble Tea model for the PR list screen.
type Model struct {
	svc       review.Service
	prs       []review.PullRequest
	cursor    int
	filters   []review.PRFilter
	filterIdx int
	loading   bool
	err       error
	width     int
	height    int
	spinner   spinner.Model
}

// New creates a new PR list model.
func New(svc review.Service, filters []review.PRFilter) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		svc:     svc,
		filters: filters,
		loading: true,
		spinner: s,
	}
}

// Init starts the initial PR fetch.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.fetchPRs(),
		m.spinner.Tick,
	)
}

func (m Model) fetchPRs() tea.Cmd {
	return func() tea.Msg {
		prs, err := m.svc.ListPRs(context.Background(), m.filters[m.filterIdx])
		return prsLoadedMsg{prs: prs, err: err}
	}
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case prsLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.prs = msg.prs
			m.cursor = 0
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		if m.loading {
			return m, nil
		}
		switch {
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, keys.Down):
			if m.cursor < len(m.prs)-1 {
				m.cursor++
			}
		case key.Matches(msg, keys.Select):
			if len(m.prs) > 0 {
				return m, func() tea.Msg {
					return PRSelectedMsg{PR: m.prs[m.cursor]}
				}
			}
		case key.Matches(msg, keys.Filter):
			m.filterIdx = (m.filterIdx + 1) % len(m.filters)
			m.loading = true
			return m, tea.Batch(m.fetchPRs(), m.spinner.Tick)
		case key.Matches(msg, keys.Refresh):
			m.loading = true
			return m, tea.Batch(m.fetchPRs(), m.spinner.Tick)
		}
	}

	return m, nil
}

// View renders the PR list.
func (m Model) View() string {
	if m.width == 0 {
		return ""
	}

	var b strings.Builder

	// Header
	header := styles.Title.Render("PR Review")
	filterLabel := fmt.Sprintf("  Filter: [%s]", m.filters[m.filterIdx].Name)
	repoLabel := fmt.Sprintf("  %s/%s", m.svc.RepoOwner(), m.svc.RepoName())
	b.WriteString(header + filterLabel + repoLabel + "\n\n")

	if m.loading {
		b.WriteString(m.spinner.View() + " Loading PRs...\n")
		return b.String()
	}

	if m.err != nil {
		b.WriteString(lipgloss.NewStyle().Foreground(styles.Danger).Render(fmt.Sprintf("Error: %v", m.err)))
		b.WriteString("\n\nPress 'r' to retry or 'q' to quit")
		return b.String()
	}

	if len(m.prs) == 0 {
		b.WriteString("No PRs found for this filter.\n")
		b.WriteString("\nPress 'f' to change filter or 'q' to quit")
		return b.String()
	}

	// Table header
	headerFmt := fmt.Sprintf("  %-6s %-*s %-12s %-10s %s",
		"#", m.width-50, "Title", "Author", "+/-", "Updated")
	b.WriteString(styles.Subtitle.Render(headerFmt) + "\n")
	b.WriteString(strings.Repeat("─", m.width) + "\n")

	// PR rows
	maxVisible := m.height - 8 // account for header, footer, padding
	if maxVisible < 1 {
		maxVisible = 1
	}
	start := 0
	if m.cursor >= maxVisible {
		start = m.cursor - maxVisible + 1
	}
	end := start + maxVisible
	if end > len(m.prs) {
		end = len(m.prs)
	}

	for i := start; i < end; i++ {
		pr := m.prs[i]
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}

		titleWidth := m.width - 50
		if titleWidth < 20 {
			titleWidth = 20
		}
		title := truncate(pr.Title, titleWidth)
		if pr.Draft {
			title = styles.PRDraft.Render(title)
		}

		changes := fmt.Sprintf("+%d/-%d", pr.Additions, pr.Deletions)
		updated := timeAgo(pr.UpdatedAt)

		line := fmt.Sprintf("%s%-6s %-*s %-12s %-10s %s",
			cursor,
			styles.PRNumber.Render(fmt.Sprintf("#%d", pr.Number)),
			titleWidth, title,
			styles.PRAuthor.Render("@"+pr.Author),
			changes,
			updated,
		)

		if i == m.cursor {
			line = lipgloss.NewStyle().Bold(true).Render(line)
		}

		b.WriteString(line + "\n")
	}

	// Footer
	b.WriteString("\n")
	help := "↑/k up  ↓/j down  enter select  f filter  r refresh  q quit"
	b.WriteString(styles.HelpStyle.Render(help))

	return b.String()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
