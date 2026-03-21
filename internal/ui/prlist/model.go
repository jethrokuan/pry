package prlist

import (
	"context"
	"fmt"
	"image/color"
	"log/slog"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jethrokuan/pry/internal/review"
	"github.com/jethrokuan/pry/internal/ui/components/scrollbar"
	"github.com/jethrokuan/pry/internal/ui/components/tabbar"
	"github.com/jethrokuan/pry/internal/ui/prpreview"
	"github.com/jethrokuan/pry/internal/ui/styles"
)

// PRSelectedMsg is sent when the user selects a PR.
type PRSelectedMsg struct {
	PR *review.PullRequest
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
	Up          key.Binding
	Down        key.Binding
	Select      key.Binding
	NextTab     key.Binding
	PrevTab     key.Binding
	EditFilter  key.Binding
	Refresh     key.Binding
	SidebarDown key.Binding
	SidebarUp   key.Binding
	Quit        key.Binding
	Help        key.Binding
}

var keys = KeyMap{
	Up:          key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:        key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	Select:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
	NextTab:     key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next tab")),
	PrevTab:     key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev tab")),
	EditFilter:  key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "edit filter")),
	Refresh:     key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
	SidebarDown: key.NewBinding(key.WithKeys("J"), key.WithHelp("J", "scroll preview down")),
	SidebarUp:   key.NewBinding(key.WithKeys("K"), key.WithHelp("K", "scroll preview up")),
	Quit:        key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
	Help:        key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
}

// Model is the Bubble Tea model for the PR list screen.
type Model struct {
	svc              review.Service
	prs              []review.PullRequest
	cursor           int
	filters          []review.PRFilter
	filterIdx        int
	userTeams        map[string]bool // cached user team membership ("org/slug" → true)
	loading          bool
	err              error
	width            int
	height           int
	spinner          spinner.Model

	// Tab bar (replaces filter picker overlay)
	tabBar tabbar.Model

	// Sidebar preview
	preview prpreview.Model

	// Filter editing
	editing      bool            // true when the qualifier text input is active
	filterInput  textinput.Model // text input for editing the qualifier
	customFilter *review.PRFilter // non-nil when a user-edited filter is active
}

// New creates a new PR list model.
func New(svc review.Service, filters []review.PRFilter) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	ti := textinput.New()
	ti.Placeholder = "e.g. review-requested:@me label:bug author:octocat"
	ti.CharLimit = 256

	tabs := make([]tabbar.Tab, len(filters))
	for i, f := range filters {
		tabs[i] = tabbar.Tab{Label: f.Name, Count: -1}
	}

	return Model{
		svc:          svc,
		filters:      filters,
		loading:      true,
		spinner:      s,
		filterInput: ti,
		tabBar:      tabbar.New(tabs),
		preview:     prpreview.New(svc),
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

// layoutDimensions returns sidebar width and main content height.
func (m Model) layoutDimensions() (sidebarW, mainHeight int) {
	sidebarW = m.width * 45 / 100
	// tab bar (1) + separator (1) + footer (2) = 4
	mainHeight = m.height - 4
	if mainHeight < 3 {
		mainHeight = 3
	}
	return
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
		// Pre-size sidebar so viewport is initialized before content is set
		sw, mh := m.layoutDimensions()
		m.preview.SetSize(sw, mh)

	case prsLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.prs = msg.prs
			m.cursor = 0
			m.preview.ResetCache()
			m.tabBar.SetCount(m.filterIdx, len(m.prs))
			return m, m.refreshSidebarPreview()
		}

	case userTeamsLoadedMsg:
		m.userTeams = make(map[string]bool, len(msg.teams))
		for _, t := range msg.teams {
			m.userTeams[t] = true
		}

	case prpreview.BodyLoadedMsg:
		if len(m.prs) > 0 && m.cursor < len(m.prs) {
			m.preview.HandleBodyLoaded(msg, &m.prs[m.cursor])
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyPressMsg:
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
			case "esc", "ctrl+c":
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

		// Normal mode
		switch {
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, keys.Up):
			if m.cursor > 0 {
				m.cursor--
				return m, m.refreshSidebarPreview()
			}
		case key.Matches(msg, keys.Down):
			if m.cursor < len(m.prs)-1 {
				m.cursor++
				return m, m.refreshSidebarPreview()
			}
		case key.Matches(msg, keys.Select):
			if len(m.prs) > 0 {
				pr := m.prs[m.cursor]
				return m, func() tea.Msg {
					return PRSelectedMsg{PR: &pr}
				}
			}
		case key.Matches(msg, keys.NextTab):
			if m.tabBar.Next() {
				m.filterIdx = m.tabBar.Active()
				m.customFilter = nil
				m.loading = true
				return m, tea.Batch(m.fetchPRs(), m.spinner.Tick)
			}
		case key.Matches(msg, keys.PrevTab):
			if m.tabBar.Prev() {
				m.filterIdx = m.tabBar.Active()
				m.customFilter = nil
				m.loading = true
				return m, tea.Batch(m.fetchPRs(), m.spinner.Tick)
			}
		case key.Matches(msg, keys.EditFilter):
			m.editing = true
			m.customFilter = nil // clear custom filter when entering edit mode
			m.filterInput.SetValue(m.activeFilter().Qualifier)
			m.filterInput.Focus()
			return m, nil
		case key.Matches(msg, keys.SidebarDown):
			m.preview.ScrollDown(3)
		case key.Matches(msg, keys.SidebarUp):
			m.preview.ScrollUp(3)
		case key.Matches(msg, keys.Refresh):
			m.loading = true
			return m, tea.Batch(m.fetchPRs(), m.spinner.Tick)
		}

		// Number keys 1-9 for direct tab selection
		if num := msg.String(); len(num) == 1 && num[0] >= '1' && num[0] <= '9' {
			idx := int(num[0]-'1')
			if idx < len(m.filters) && idx != m.filterIdx {
				m.tabBar.SetActive(idx)
				m.filterIdx = idx
				m.customFilter = nil
				m.loading = true
				return m, tea.Batch(m.fetchPRs(), m.spinner.Tick)
			}
		}
	}

	return m, nil
}

// refreshSidebarPreview updates the sidebar with the current PR's metadata
// and triggers an async body fetch if needed.
func (m *Model) refreshSidebarPreview() tea.Cmd {
	if len(m.prs) == 0 || m.cursor >= len(m.prs) {
		m.preview.SetNoSelection()
		return nil
	}
	fn := m.preview.Refresh(&m.prs[m.cursor])
	if fn == nil {
		return nil
	}
	return func() tea.Msg { return fn() }
}

// renderCtx holds extra data needed at render time.
type renderCtx struct {
	userTeams map[string]bool // "org/team-slug" → true
}

// View renders the PR list.
func (m Model) View() string {
	if m.width == 0 {
		return m.spinner.View() + " Loading..."
	}

	var b strings.Builder

	// Tab bar (full width)
	m.tabBar.SetWidth(m.width)
	b.WriteString(m.tabBar.View() + "\n")

	// Horizontal separator between tab bar and panes
	b.WriteString(lipgloss.NewStyle().Foreground(styles.Muted).Render(strings.Repeat("─", m.width)) + "\n")

	// Compute layout
	sidebarW, mainHeight := m.layoutDimensions()
	tableWidth := m.width - sidebarW

	// Left pane: search bar + PR table
	var leftPane strings.Builder

	// Search bar (scoped to left pane width)
	if m.editing {
		searchBorder := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(styles.Primary).
			Width(tableWidth - 4).
			Padding(0, 1)
		leftPane.WriteString(searchBorder.Render(m.filterInput.View()) + "\n")
	} else {
		af := m.activeFilter()
		qualifier := af.Qualifier
		if qualifier == "" {
			qualifier = "(all open)"
		}
		searchBorder := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(styles.Muted).
			Width(tableWidth - 4).
			Padding(0, 1)
		leftPane.WriteString(searchBorder.Render(
			lipgloss.NewStyle().Foreground(styles.Muted).Render(qualifier),
		) + "\n")
	}

	// Search bar takes 3 lines (border top + content + border bottom)
	tableHeight := mainHeight - 3

	if m.loading {
		loadingText := m.spinner.View() + " Loading PRs..."
		leftPane.WriteString(lipgloss.Place(tableWidth, tableHeight, lipgloss.Center, lipgloss.Center, loadingText))
	} else if m.err != nil {
		leftPane.WriteString(lipgloss.NewStyle().Foreground(styles.Danger).Render(fmt.Sprintf("Error: %v", m.err)))
		leftPane.WriteString("\n\nPress 'r' to retry or ctrl+c to quit")
	} else if len(m.prs) == 0 {
		leftPane.WriteString("No PRs found for this filter.\n")
		leftPane.WriteString("\nPress tab to switch filters or ctrl+c to quit")
	} else {
		scrollbarWidth := 1
		tableContent := m.renderTable(tableWidth-scrollbarWidth, tableHeight)

		sb := scrollbar.New()
		sb.Height = tableHeight
		sb.TotalItems = len(m.prs)
		rowHeight := 4
		sb.VisibleItems = tableHeight / rowHeight
		sb.ThumbColor = styles.Primary
		// Compute offset (same logic as renderTable)
		visibleRows := tableHeight / rowHeight
		if visibleRows < 1 {
			visibleRows = 1
		}
		start := 0
		if m.cursor >= visibleRows {
			start = m.cursor - visibleRows + 1
		}
		sb.Offset = start

		sbView := sb.View()
		if sbView != "" {
			leftPane.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, tableContent, sbView))
		} else {
			leftPane.WriteString(tableContent)
		}
	}

	m.preview.SetSize(sidebarW, mainHeight)
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, leftPane.String(), m.preview.View()))

	// Footer
	b.WriteString("\n")
	repoLabel := fmt.Sprintf("%s/%s", m.svc.RepoOwner(), m.svc.RepoName())
	help := styles.HelpStyle.Render("↑/k up  ↓/j down  enter select  tab switch  / search  J/K scroll preview  r refresh  ctrl+c quit")
	repo := lipgloss.NewStyle().Foreground(styles.Primary).Render(repoLabel)
	footerWidth := lipgloss.Width(help) + lipgloss.Width(repo)
	gap := m.width - footerWidth
	if gap < 1 {
		gap = 1
	}
	b.WriteString(help + strings.Repeat(" ", gap) + repo)

	return b.String()
}

// renderTable renders the PR table with multi-line rows (gh-dash style).
// Each PR gets three lines:
//
//	Line 1: state_icon  #number by @author
//	Line 2:             bold title
//	Line 3:             +N -N  ·  N files  ·  review_status  CI  ·  comments  ·  updated  age
func (m Model) renderTable(width, height int) string {
	// Each PR row = 3 lines content + 1 border = 4 lines visual height
	rowHeight := 4
	visibleRows := height / rowHeight
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

	stateWidth := 4 // icon + padding

	var b strings.Builder

	visiblePRs := m.prs[start:end]
	cursorInView := m.cursor - start

	for i, pr := range visiblePRs {
		isSelected := i == cursorInView

		// Base style carries background for selected rows.
		// All fragment styles use .Inherit(base) to pick it up.
		base := lipgloss.NewStyle()
		if isSelected {
			base = base.Background(styles.BgSelected)
		}
		s := func(c color.Color) lipgloss.Style {
			return lipgloss.NewStyle().Foreground(c).Inherit(base)
		}
		muted := s(styles.Muted)

		stateIcon, stateColor := renderStateIcon(pr)

		// Line 1: state_icon  #number by @author
		line1Content := s(stateColor).Width(stateWidth).Render(stateIcon) +
			s(lipgloss.BrightYellow).Render(fmt.Sprintf("#%d", pr.Number)) +
			muted.Render(" by ") +
			s(lipgloss.BrightYellow).Render("@"+pr.Author)
		line1 := base.Width(width).Render(line1Content)

		// Line 2: bold title
		indent := lipgloss.NewStyle().Inherit(base).Width(stateWidth).Render("")
		var titleRendered string
		if pr.Draft {
			titleRendered = lipgloss.NewStyle().Foreground(styles.Muted).Italic(true).Inherit(base).Render(pr.Title)
		} else {
			titleRendered = lipgloss.NewStyle().Bold(true).Inherit(base).Render(pr.Title)
		}
		line2 := base.Width(width).Render(indent + titleRendered)

		// Line 3: stats with colored icons
		dot := muted.Render(" · ")
		statsContent := indent +
			s(styles.Success).Render(fmt.Sprintf("+%s", formatNum(pr.Additions))) +
			muted.Render(" ") +
			s(styles.Danger).Render(fmt.Sprintf("-%s", formatNum(pr.Deletions))) +
			dot +
			s(lipgloss.BrightCyan).Render(iconFiles) + muted.Render(fmt.Sprintf(" %d", pr.Files)) +
			dot +
			renderReviewStatusInherited(pr, base) +
			muted.Render(" ") +
			renderCIIconInherited(pr, base)
		if pr.CommentCount > 0 {
			statsContent += dot +
				s(lipgloss.BrightBlue).Render(iconCommentSingle) + muted.Render(fmt.Sprintf(" %d", pr.CommentCount))
		}
		statsContent += dot +
			s(lipgloss.BrightMagenta).Render(iconUpdated) + muted.Render(" "+shortTimeAgo(pr.UpdatedAt)) +
			dot +
			s(lipgloss.BrightCyan).Render(iconCreated) + muted.Render(" "+shortTimeAgo(pr.CreatedAt))
		line3 := base.Width(width).Render(statsContent)

		row := line1 + "\n" + line2 + "\n" + line3
		borderStyle := lipgloss.NewStyle().
			BorderBottom(true).
			BorderStyle(lipgloss.Border{Bottom: "─"}).
			BorderForeground(styles.Muted)
		b.WriteString(borderStyle.Render(row) + "\n")
	}

	return b.String()
}

// Nerd Font icons — exact codepoints from gh-dash constants.go.
var (
	iconOpen   = "\uf407" // nf-oct-git_pull_request
	iconDraft  = "\uebdb" // nf-cod-git_pull_request_draft
	iconMerged = "\uf4c9" // nf-oct-git_merge
	iconClosed = "\uf4dc" // nf-oct-git_pull_request_closed

	iconApproved         = "\U000f012c" // nf-md-check_circle
	iconChangesRequested = "\ueb43"     // nf-cod-request_changes
	iconWaiting          = "\ue641"     // nf-seti-clock
	iconComment          = "\uf0e6"     // nf-fa-comments
	iconCommentSingle    = "\uf27b"     // nf-fa-commenting

	iconCISuccess = "\uf058"     // nf-fa-check_circle
	iconCIFailure = "\U000f0159" // nf-md-close_circle
	iconCIPending = "\ue641"     // nf-seti-clock
	iconEmpty     = "\ueabd"     // nf-cod-circle_slash

	iconFiles   = "\uf440" // nf-oct-diff
	iconUpdated = "\uf472" // nf-oct-history
	iconCreated = "\uf455" // nf-oct-calendar
)

// renderStateIcon returns the icon and color for a PR's state.
func renderStateIcon(pr review.PullRequest) (string, color.Color) {
	if pr.Draft {
		return iconDraft, lipgloss.BrightBlack
	}
	switch pr.State {
	case "MERGED":
		return iconMerged, lipgloss.Magenta
	case "CLOSED":
		return iconClosed, lipgloss.Red
	default:
		return iconOpen, lipgloss.Green
	}
}

// renderReviewStatusInherited renders the review porcelain with colors, inheriting base for background.
func renderReviewStatusInherited(pr review.PullRequest, base lipgloss.Style) string {
	s := func(c color.Color) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(c).Inherit(base)
	}

	if len(pr.Reviewers) == 0 {
		switch pr.ReviewDecision {
		case "APPROVED":
			return s(lipgloss.Green).Render(iconApproved)
		case "CHANGES_REQUESTED":
			return s(lipgloss.Red).Render(iconChangesRequested)
		default:
			return s(lipgloss.BrightBlack).Render(iconWaiting)
		}
	}

	var approved, pending, changesReq, commented int
	for _, r := range pr.Reviewers {
		switch r.State {
		case "APPROVED":
			approved++
		case "CHANGES_REQUESTED":
			changesReq++
		case "COMMENTED":
			commented++
		default:
			pending++
		}
	}

	var parts []string
	if approved > 0 {
		parts = append(parts, s(lipgloss.Green).Render(fmt.Sprintf("%s %d", iconApproved, approved)))
	}
	if changesReq > 0 {
		parts = append(parts, s(lipgloss.Red).Render(fmt.Sprintf("%s %d", iconChangesRequested, changesReq)))
	}
	if commented > 0 {
		parts = append(parts, s(lipgloss.Cyan).Render(fmt.Sprintf("%s %d", iconComment, commented)))
	}
	if pending > 0 {
		parts = append(parts, s(lipgloss.BrightBlack).Render(fmt.Sprintf("%s %d", iconWaiting, pending)))
	}
	if len(parts) == 0 {
		return s(lipgloss.BrightBlack).Render(iconWaiting)
	}
	sp := lipgloss.NewStyle().Inherit(base).Render(" ")
	return strings.Join(parts, sp)
}

// renderCIIconInherited renders the CI icon with color, inheriting base for background.
func renderCIIconInherited(pr review.PullRequest, base lipgloss.Style) string {
	s := func(c color.Color) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(c).Inherit(base)
	}
	if pr.ChecksPass == nil {
		return s(lipgloss.BrightBlack).Render(iconEmpty)
	}
	if *pr.ChecksPass {
		return s(lipgloss.Green).Render(iconCISuccess)
	}
	return s(lipgloss.Red).Render(iconCIFailure)
}

// renderStatsPlain returns the stats line as plain text (no ANSI colors)
// so it can be rendered as a single cell with uniform background.
func renderStatsPlain(pr review.PullRequest) string {
	parts := []string{
		fmt.Sprintf("+%s -%s", formatNum(pr.Additions), formatNum(pr.Deletions)),
		fmt.Sprintf("%s %d", iconFiles, pr.Files),
		renderReviewPlain(pr),
		renderCIPlain(pr),
	}
	if pr.CommentCount > 0 {
		parts = append(parts, fmt.Sprintf("%s %d", iconCommentSingle, pr.CommentCount))
	}
	parts = append(parts,
		fmt.Sprintf("%s %s", iconUpdated, shortTimeAgo(pr.UpdatedAt)),
		fmt.Sprintf("%s %s", iconCreated, shortTimeAgo(pr.CreatedAt)),
	)
	return strings.Join(parts, " · ")
}

// renderReviewPlain returns the review porcelain as plain text.
func renderReviewPlain(pr review.PullRequest) string {
	if len(pr.Reviewers) == 0 {
		switch pr.ReviewDecision {
		case "APPROVED":
			return iconApproved
		case "CHANGES_REQUESTED":
			return iconChangesRequested
		default:
			return iconWaiting
		}
	}

	var approved, pending, changesReq, commented int
	for _, r := range pr.Reviewers {
		switch r.State {
		case "APPROVED":
			approved++
		case "CHANGES_REQUESTED":
			changesReq++
		case "COMMENTED":
			commented++
		default:
			pending++
		}
	}

	var parts []string
	if approved > 0 {
		parts = append(parts, fmt.Sprintf("%s %d", iconApproved, approved))
	}
	if changesReq > 0 {
		parts = append(parts, fmt.Sprintf("%s %d", iconChangesRequested, changesReq))
	}
	if commented > 0 {
		parts = append(parts, fmt.Sprintf("%s %d", iconComment, commented))
	}
	if pending > 0 {
		parts = append(parts, fmt.Sprintf("%s %d", iconWaiting, pending))
	}
	if len(parts) == 0 {
		return iconWaiting
	}
	return strings.Join(parts, " ")
}

// renderCIPlain returns the CI status as plain text.
func renderCIPlain(pr review.PullRequest) string {
	if pr.ChecksPass == nil {
		return iconEmpty
	}
	if *pr.ChecksPass {
		return iconCISuccess
	}
	return iconCIFailure
}

// renderReviewStatus returns a colored porcelain review summary (used by sidebar).
func renderReviewStatus(pr review.PullRequest) string {
	fg := func(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }

	if len(pr.Reviewers) == 0 {
		switch pr.ReviewDecision {
		case "APPROVED":
			return fg(lipgloss.Green).Render(iconApproved)
		case "CHANGES_REQUESTED":
			return fg(lipgloss.Red).Render(iconChangesRequested)
		default:
			return fg(lipgloss.BrightBlack).Render(iconWaiting)
		}
	}

	var approved, pending, changesReq, commented int
	for _, r := range pr.Reviewers {
		switch r.State {
		case "APPROVED":
			approved++
		case "CHANGES_REQUESTED":
			changesReq++
		case "COMMENTED":
			commented++
		default:
			pending++
		}
	}

	var parts []string
	if approved > 0 {
		parts = append(parts, fg(lipgloss.Green).Render(fmt.Sprintf("%s %d", iconApproved, approved)))
	}
	if changesReq > 0 {
		parts = append(parts, fg(lipgloss.Red).Render(fmt.Sprintf("%s %d", iconChangesRequested, changesReq)))
	}
	if commented > 0 {
		parts = append(parts, fg(lipgloss.Cyan).Render(fmt.Sprintf("%s %d", iconComment, commented)))
	}
	if pending > 0 {
		parts = append(parts, fg(lipgloss.BrightBlack).Render(fmt.Sprintf("%s %d", iconWaiting, pending)))
	}

	if len(parts) == 0 {
		return fg(lipgloss.BrightBlack).Render(iconWaiting)
	}
	return strings.Join(parts, " ")
}

// renderCIIcon returns the icon and color for CI status.
func renderCIIcon(pr review.PullRequest) (string, color.Color) {
	if pr.ChecksPass == nil {
		return iconEmpty, lipgloss.BrightBlack
	}
	if *pr.ChecksPass {
		return iconCISuccess, lipgloss.Green
	}
	return iconCIFailure, lipgloss.Red
}

// formatNum formats a number with k suffix for large values.
func formatNum(n int) string {
	if n >= 10000 {
		return fmt.Sprintf("%.0fk", float64(n)/1000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

// shortTimeAgo returns a compact relative time string.
func shortTimeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dw", int(d.Hours()/(24*7)))
	default:
		return fmt.Sprintf("%dmo", int(d.Hours()/(24*30)))
	}
}

