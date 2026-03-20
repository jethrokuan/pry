package prlist

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	ltable "github.com/charmbracelet/lipgloss/table"

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

type userTeamsLoadedMsg struct {
	teams []string
}

// KeyMap defines the key bindings for the PR list.
type KeyMap struct {
	Up         key.Binding
	Down       key.Binding
	Select     key.Binding
	Filter     key.Binding
	EditFilter key.Binding
	Refresh    key.Binding
	Quit       key.Binding
	Help       key.Binding
}

var keys = KeyMap{
	Up:         key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:       key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	Select:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
	Filter:     key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "filter")),
	EditFilter: key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "edit filter")),
	Refresh:    key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
	Quit:       key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	Help:       key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
}

// Model is the Bubble Tea model for the PR list screen.
type Model struct {
	svc              review.Service
	prs              []review.PullRequest
	cursor           int
	filters          []review.PRFilter
	filterIdx        int
	columns          []string
	userTeams        map[string]bool // cached user team membership ("org/slug" → true)
	loading          bool
	err              error
	width            int
	height           int
	spinner          spinner.Model
	showFilterPicker bool
	filterCursor     int

	// Filter editing
	editing      bool            // true when the qualifier text input is active
	filterInput  textinput.Model // text input for editing the qualifier
	customFilter *review.PRFilter // non-nil when a user-edited filter is active
}

// New creates a new PR list model.
func New(svc review.Service, filters []review.PRFilter, columns []string) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	ti := textinput.New()
	ti.Placeholder = "e.g. review-requested:@me label:bug author:octocat"
	ti.CharLimit = 256

	return Model{
		svc:         svc,
		filters:     filters,
		columns:     columns,
		loading:     true,
		spinner:     s,
		filterInput: ti,
	}
}

// Init starts the initial PR fetch.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.fetchPRs(),
		m.fetchUserTeams(),
		m.spinner.Tick,
	)
}

func (m Model) fetchUserTeams() tea.Cmd {
	return func() tea.Msg {
		teams, err := m.svc.UserTeams(context.Background())
		if err != nil {
			slog.Warn("failed to fetch user teams", "error", err)
		}
		return userTeamsLoadedMsg{teams: teams}
	}
}

// activeFilter returns the currently active filter — either a custom
// user-edited filter or the selected preset filter.
func (m Model) activeFilter() review.PRFilter {
	if m.customFilter != nil {
		return *m.customFilter
	}
	return m.filters[m.filterIdx]
}

func (m Model) fetchPRs() tea.Cmd {
	filter := m.activeFilter()
	return func() tea.Msg {
		prs, err := m.svc.ListPRs(context.Background(), filter)
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

	case userTeamsLoadedMsg:
		m.userTeams = make(map[string]bool, len(msg.teams))
		for _, t := range msg.teams {
			m.userTeams[t] = true
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		// Filter editing mode: forward keys to the text input
		if m.editing {
			switch msg.String() {
			case "enter":
				qualifier := strings.TrimSpace(m.filterInput.Value())
				m.editing = false
				m.filterInput.Blur()
				m.customFilter = &review.PRFilter{
					Name:      "Custom",
					Qualifier: qualifier,
				}
				m.loading = true
				return m, tea.Batch(m.fetchPRs(), m.spinner.Tick)
			case "esc":
				m.editing = false
				m.filterInput.Blur()
				return m, nil
			default:
				var cmd tea.Cmd
				m.filterInput, cmd = m.filterInput.Update(msg)
				return m, cmd
			}
		}

		if m.loading {
			return m, nil
		}

		// Filter picker mode
		if m.showFilterPicker {
			switch {
			case key.Matches(msg, keys.Up):
				if m.filterCursor > 0 {
					m.filterCursor--
				}
			case key.Matches(msg, keys.Down):
				if m.filterCursor < len(m.filters)-1 {
					m.filterCursor++
				}
			case key.Matches(msg, keys.Select):
				m.showFilterPicker = false
				if m.filterCursor != m.filterIdx || m.customFilter != nil {
					m.filterIdx = m.filterCursor
					m.customFilter = nil
					m.loading = true
					return m, tea.Batch(m.fetchPRs(), m.spinner.Tick)
				}
			case key.Matches(msg, keys.Quit), key.Matches(msg, key.NewBinding(key.WithKeys("esc", "f"))):
				m.showFilterPicker = false
			}
			return m, nil
		}

		// Normal mode
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
			m.showFilterPicker = true
			m.filterCursor = m.filterIdx
		case key.Matches(msg, keys.EditFilter):
			m.editing = true
			m.customFilter = nil // clear custom filter when entering edit mode
			m.filterInput.SetValue(m.activeFilter().Qualifier)
			m.filterInput.Focus()
			return m, m.filterInput.Cursor.BlinkCmd()
		case key.Matches(msg, keys.Refresh):
			m.loading = true
			return m, tea.Batch(m.fetchPRs(), m.spinner.Tick)
		}
	}

	return m, nil
}

// renderCtx holds extra data needed by some columns at render time.
type renderCtx struct {
	userTeams map[string]bool // "org/team-slug" → true
}

// columnDef describes how to render a single column.
type columnDef struct {
	id     string
	header string
	width  int // fixed width; 0 means flexible (takes remaining space)
	render func(pr review.PullRequest, ctx renderCtx) string
	// style returns a lipgloss.Style for the cell. If nil, no extra styling.
	style func(pr review.PullRequest, ctx renderCtx) lipgloss.Style
}

// knownColumns maps column IDs to their definitions.
// Width 0 means the column is flexible and fills remaining space.
var knownColumns = map[string]columnDef{
	"number": {
		id:     "number",
		header: "#",
		width:  7,
		render: func(pr review.PullRequest, _ renderCtx) string {
			return fmt.Sprintf("#%d", pr.Number)
		},
		style: func(_ review.PullRequest, _ renderCtx) lipgloss.Style {
			return lipgloss.NewStyle().Foreground(styles.Primary)
		},
	},
	"title": {
		id:     "title",
		header: "Title",
		width:  0, // flexible
		render: func(pr review.PullRequest, _ renderCtx) string {
			return pr.Title
		},
		style: func(pr review.PullRequest, _ renderCtx) lipgloss.Style {
			if pr.Draft {
				return lipgloss.NewStyle().Italic(true).Foreground(styles.Muted)
			}
			return lipgloss.NewStyle()
		},
	},
	"author": {
		id:     "author",
		header: "Author",
		width:  14,
		render: func(pr review.PullRequest, _ renderCtx) string {
			return "@" + pr.Author
		},
		style: func(_ review.PullRequest, _ renderCtx) lipgloss.Style {
			return lipgloss.NewStyle().Foreground(styles.Cyan)
		},
	},
	"changes": {
		id:     "changes",
		header: "+/-",
		width:  12,
		render: func(pr review.PullRequest, _ renderCtx) string {
			return fmt.Sprintf("+%d/-%d", pr.Additions, pr.Deletions)
		},
	},
	"updated": {
		id:     "updated",
		header: "Updated",
		width:  10,
		render: func(pr review.PullRequest, _ renderCtx) string {
			return timeAgo(pr.UpdatedAt)
		},
	},
	"pending_teams": {
		id:     "pending_teams",
		header: "Pending Teams",
		width:  20,
		render: func(pr review.PullRequest, _ renderCtx) string {
			if len(pr.PendingTeams) == 0 {
				return ""
			}
			return strings.Join(stripOrgPrefixes(pr.PendingTeams), ", ")
		},
	},
	"my_teams": {
		id:     "my_teams",
		header: "My Teams",
		width:  20,
		render: func(pr review.PullRequest, ctx renderCtx) string {
			if len(pr.PendingTeams) == 0 || len(ctx.userTeams) == 0 {
				return ""
			}
			var mine []string
			for _, t := range pr.PendingTeams {
				if ctx.userTeams[t] {
					mine = append(mine, stripOrgPrefix(t))
				}
			}
			return strings.Join(mine, ", ")
		},
	},
	"my_review": {
		id:     "my_review",
		header: "Review",
		width:  10,
		render: func(pr review.PullRequest, _ renderCtx) string {
			switch pr.MyReviewState {
			case "APPROVED":
				return "Approved"
			case "CHANGES_REQUESTED":
				return "Changes"
			case "COMMENTED":
				return "Commented"
			case "DISMISSED":
				return "Dismissed"
			default:
				return ""
			}
		},
		style: func(pr review.PullRequest, _ renderCtx) lipgloss.Style {
			switch pr.MyReviewState {
			case "APPROVED":
				return lipgloss.NewStyle().Foreground(styles.Success)
			case "CHANGES_REQUESTED":
				return lipgloss.NewStyle().Foreground(styles.Warning)
			case "COMMENTED":
				return lipgloss.NewStyle().Foreground(styles.Cyan)
			case "DISMISSED":
				return lipgloss.NewStyle().Foreground(styles.Muted)
			default:
				return lipgloss.NewStyle()
			}
		},
	},
}

// resolveColumns returns ordered column defs for the configured column IDs.
func resolveColumns(ids []string) []columnDef {
	var cols []columnDef
	for _, id := range ids {
		if c, ok := knownColumns[id]; ok {
			cols = append(cols, c)
		}
	}
	return cols
}

// View renders the PR list.
func (m Model) View() string {
	if m.width == 0 {
		return ""
	}

	var b strings.Builder

	// Header
	header := styles.Title.Render("PR Review")
	af := m.activeFilter()
	filterLabel := fmt.Sprintf("  Filter: [%s]", af.Name)
	repoLabel := fmt.Sprintf("  %s/%s", m.svc.RepoOwner(), m.svc.RepoName())
	b.WriteString(header + filterLabel + repoLabel + "\n")

	if m.editing {
		prompt := lipgloss.NewStyle().Foreground(styles.Primary).Bold(true).Render("  Qualifier: ")
		b.WriteString(prompt + m.filterInput.View() + "\n")
		hint := lipgloss.NewStyle().Foreground(styles.Muted)
		b.WriteString(hint.Render("  enter apply  esc cancel  Examples: author:X label:Y review-requested:@me team-review-requested:@my-teams") + "\n\n")
		// Still show the table below the editor
	} else {
		qualifier := af.Qualifier
		if qualifier == "" {
			qualifier = "(none)"
		}
		qualifierStyle := lipgloss.NewStyle().Foreground(styles.Muted)
		b.WriteString(qualifierStyle.Render("  "+qualifier) + "\n\n")
	}

	// Filter picker overlay
	if m.showFilterPicker {
		b.WriteString("Select a filter:\n\n")
		for i, f := range m.filters {
			cursor := "  "
			if i == m.filterCursor {
				cursor = "> "
			}
			name := f.Name
			if i == m.filterIdx {
				name += " (current)"
			}
			if i == m.filterCursor {
				b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(styles.Primary).Render(cursor+name) + "\n")
			} else {
				b.WriteString(cursor + name + "\n")
			}
		}
		b.WriteString("\n")
		b.WriteString(styles.HelpStyle.Render("↑/k up  ↓/j down  enter select  esc/f cancel"))
		return b.String()
	}

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

	cols := resolveColumns(m.columns)
	rctx := renderCtx{userTeams: m.userTeams}

	// Compute visible window (account for qualifier line from PR #6)
	tableHeight := m.height - 7 // header + qualifier + footer + padding
	if tableHeight < 3 {
		tableHeight = 3
	}
	visibleRows := tableHeight - 2 // header row + border
	if visibleRows < 1 {
		visibleRows = 1
	}
	start := 0
	if m.cursor >= visibleRows {
		start = m.cursor - visibleRows + 1
	}
	end := start + visibleRows
	if end > len(m.prs) {
		end = len(m.prs)
	}

	// Build headers
	headers := make([]string, len(cols))
	for i, c := range cols {
		headers[i] = c.header
	}

	// Build rows for visible window
	visiblePRs := m.prs[start:end]
	rows := make([][]string, len(visiblePRs))
	for i, pr := range visiblePRs {
		row := make([]string, len(cols))
		for j, c := range cols {
			row[j] = c.render(pr, rctx)
		}
		rows[i] = row
	}

	// Cursor-relative index within the visible window
	cursorInView := m.cursor - start

	// Build lipgloss table
	t := ltable.New().
		Headers(headers...).
		Rows(rows...).
		Width(m.width).
		BorderTop(false).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderColumn(false).
		BorderHeader(true).
		StyleFunc(func(row, col int) lipgloss.Style {
			s := lipgloss.NewStyle().PaddingRight(1)

			// Header row
			if row == ltable.HeaderRow {
				return s.Bold(true).Foreground(styles.Secondary)
			}

			// Per-column styling
			if col >= 0 && col < len(cols) {
				pr := visiblePRs[row]
				if cols[col].style != nil {
					colStyle := cols[col].style(pr, rctx)
					fg, _ := colStyle.GetForeground().(lipgloss.Color)
					if fg != "" {
						s = s.Foreground(fg)
					}
					if colStyle.GetItalic() {
						s = s.Italic(true)
					}
				}
			}

			// Selected row
			if row == cursorInView {
				s = s.Bold(true)
			}

			return s
		})

	b.WriteString(t.Render())

	// Footer
	b.WriteString("\n")
	help := "↑/k up  ↓/j down  enter select  f filter  / edit filter  r refresh  q quit"
	b.WriteString(styles.HelpStyle.Render(help))

	return b.String()
}

// stripOrgPrefix removes the "org/" prefix from a team slug like "org/team-name".
func stripOrgPrefix(slug string) string {
	if i := strings.Index(slug, "/"); i >= 0 {
		return slug[i+1:]
	}
	return slug
}

func stripOrgPrefixes(slugs []string) []string {
	out := make([]string, len(slugs))
	for i, s := range slugs {
		out[i] = stripOrgPrefix(s)
	}
	return out
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
