package prlist

import (
	"context"
	"fmt"
	"image/color"
	"log/slog"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jethrokuan/pry/internal/clipboard"
	"github.com/jethrokuan/pry/internal/ui/components/helppopup"
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

type flashExpiredMsg struct{}

// enrichState tracks per-PR background enrichment status.
type enrichState int

const (
	enrichNone    enrichState = iota // not yet fetched
	enrichPending                    // GetPR in flight
	enrichDone                       // full data merged
)

// prEnrichedMsg carries the result of a background GetPR call.
type prEnrichedMsg struct {
	PRNumber int
	FullPR   *review.PullRequest
	Err      error
}

// maxEnrichPRs is the number of PRs to background-enrich after list load.
const maxEnrichPRs = 10

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
	OpenInBrowser key.Binding
	CopyNumber   key.Binding
	CopyURL      key.Binding
	HalfPageDown key.Binding
	HalfPageUp   key.Binding
	Quit         key.Binding
	Help         key.Binding
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
	OpenInBrowser: key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "open in browser")),
	CopyNumber:   key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy PR number")),
	CopyURL:      key.NewBinding(key.WithKeys("Y"), key.WithHelp("Y", "copy PR URL")),
	HalfPageDown: key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "half page down")),
	HalfPageUp:   key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("ctrl+u", "half page up")),
	Quit:         key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
	Help:         key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
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

	// Help overlay
	showHelp bool

	// Flash message (ephemeral status)
	flashMsg string

	// Background enrichment: tracks GetPR fetch state per PR number
	enrichMap map[int]enrichState
}

// UserIdentityMsg carries the resolved user identity from the app layer.
type UserIdentityMsg struct {
	Identity *review.UserIdentity
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
		preview:     prpreview.New(),
		enrichMap:   make(map[int]enrichState),
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
// Used by Update() for sizing estimates. View() uses render-then-measure
// for the authoritative layout, so keep these values in sync.
func (m Model) layoutDimensions() (sidebarW, mainHeight int) {
	sidebarW = m.width * 45 / 100
	// Estimate: header 2 lines (tab bar + separator) + footer 2 lines (gap + help text).
	// View() computes this precisely via lipgloss.Height().
	mainHeight = m.height - 4
	if mainHeight < 3 {
		mainHeight = 3
	}
	return
}

func (m Model) halfPageSize() int {
	_, mainHeight := m.layoutDimensions()
	rowHeight := 4
	visibleRows := mainHeight / rowHeight
	half := visibleRows / 2
	if half < 1 {
		half = 1
	}
	return half
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
			m.enrichMap = make(map[int]enrichState)
			m.preview.ResetCache()
			m.tabBar.SetCount(m.filterIdx, len(m.prs))
			sidebarCmd := m.refreshSidebarPreview()
			enrichCmd := m.enrichVisible()
			return m, tea.Batch(sidebarCmd, enrichCmd)
		}

	case UserIdentityMsg:
		if msg.Identity != nil {
			m.preview.SetUserIdentity(msg.Identity)
		}

	case userTeamsLoadedMsg:
		m.userTeams = make(map[string]bool, len(msg.teams))
		for _, t := range msg.teams {
			m.userTeams[t] = true
		}

	case prEnrichedMsg:
		// Ignore stale messages from a previous filter/list load.
		if _, tracked := m.enrichMap[msg.PRNumber]; !tracked {
			return m, nil
		}
		if msg.Err != nil {
			slog.Warn("enrichment failed", "pr", msg.PRNumber, "error", msg.Err)
			delete(m.enrichMap, msg.PRNumber)
			return m, nil
		}
		m.enrichMap[msg.PRNumber] = enrichDone
		for i := range m.prs {
			if m.prs[i].Number == msg.PRNumber {
				m.prs[i].MergeEnriched(msg.FullPR)
				break
			}
		}
		// Update sidebar if this is the currently selected PR.
		if len(m.prs) > 0 && m.cursor < len(m.prs) && m.prs[m.cursor].Number == msg.PRNumber {
			m.preview.HandleBodyLoaded(prpreview.BodyLoadedMsg{
				PRNumber: msg.PRNumber,
				Body:     msg.FullPR.Body,
				FullPR:   msg.FullPR,
			}, &m.prs[m.cursor])
		}
		return m, nil

	case flashExpiredMsg:
		m.flashMsg = ""
		return m, nil

	case spinner.TickMsg:
		if m.loading || m.hasEnrichmentPending() {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}

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

		// Help overlay: any key dismisses
		if m.showHelp {
			m.showHelp = false
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
		case key.Matches(msg, keys.HalfPageDown):
			if len(m.prs) > 0 {
				m.cursor += m.halfPageSize()
				if m.cursor >= len(m.prs) {
					m.cursor = len(m.prs) - 1
				}
				return m, m.refreshSidebarPreview()
			}
		case key.Matches(msg, keys.HalfPageUp):
			if len(m.prs) > 0 {
				m.cursor -= m.halfPageSize()
				if m.cursor < 0 {
					m.cursor = 0
				}
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
		case key.Matches(msg, keys.OpenInBrowser):
			if len(m.prs) > 0 {
				return m, openBrowser(m.prs[m.cursor].URL)
			}
		case key.Matches(msg, keys.Refresh):
			if inv, ok := m.svc.(review.CacheInvalidator); ok {
				inv.InvalidateListPRs()
			}
			m.loading = true
			return m, tea.Batch(m.fetchPRs(), m.spinner.Tick)
		case key.Matches(msg, keys.CopyNumber):
			if len(m.prs) > 0 {
				pr := m.prs[m.cursor]
				text := fmt.Sprintf("%d", pr.Number)
				if err := clipboard.WriteText(text); err != nil {
					return m, m.setFlash("Copy failed: " + err.Error())
				}
				return m, m.setFlash(fmt.Sprintf("Copied #%d", pr.Number))
			}
		case key.Matches(msg, keys.CopyURL):
			if len(m.prs) > 0 {
				pr := m.prs[m.cursor]
				if err := clipboard.WriteText(pr.URL); err != nil {
					return m, m.setFlash("Copy failed: " + err.Error())
				}
				return m, m.setFlash(fmt.Sprintf("Copied %s", pr.URL))
			}
		case key.Matches(msg, keys.Help):
			m.showHelp = true
			return m, nil
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
// and ensures the selected PR is being enriched.
func (m *Model) refreshSidebarPreview() tea.Cmd {
	if len(m.prs) == 0 || m.cursor >= len(m.prs) {
		m.preview.SetNoSelection()
		return nil
	}
	pr := &m.prs[m.cursor]
	m.preview.Refresh(pr)

	// Ensure selected PR gets enriched even if outside the initial batch.
	num := pr.Number
	if m.enrichMap[num] == enrichNone {
		m.enrichMap[num] = enrichPending
		svc := m.svc
		return tea.Batch(func() tea.Msg {
			full, err := svc.GetPR(context.Background(), num)
			return prEnrichedMsg{PRNumber: num, FullPR: full, Err: err}
		}, m.spinner.Tick)
	}
	return nil
}

// enrichVisible kicks off background GetPR fetches for the first N PRs,
// prioritizing the cursor PR. Returns a batched command or nil.
func (m *Model) enrichVisible() tea.Cmd {
	if len(m.prs) == 0 {
		return nil
	}

	limit := maxEnrichPRs
	if limit > len(m.prs) {
		limit = len(m.prs)
	}

	// Build fetch list: cursor PR first, then others up to limit.
	var toFetch []int
	if m.cursor < len(m.prs) {
		num := m.prs[m.cursor].Number
		if m.enrichMap[num] == enrichNone {
			toFetch = append(toFetch, num)
			m.enrichMap[num] = enrichPending
		}
	}
	for i := 0; i < limit && len(toFetch) < limit; i++ {
		num := m.prs[i].Number
		if m.enrichMap[num] == enrichNone {
			toFetch = append(toFetch, num)
			m.enrichMap[num] = enrichPending
		}
	}

	if len(toFetch) == 0 {
		return nil
	}

	svc := m.svc
	cmds := make([]tea.Cmd, 0, len(toFetch)+1)
	for _, num := range toFetch {
		n := num
		cmds = append(cmds, func() tea.Msg {
			full, err := svc.GetPR(context.Background(), n)
			return prEnrichedMsg{PRNumber: n, FullPR: full, Err: err}
		})
	}
	cmds = append(cmds, m.spinner.Tick)
	return tea.Batch(cmds...)
}

// hasEnrichmentPending returns true if any PR enrichment is in flight.
func (m Model) hasEnrichmentPending() bool {
	for _, state := range m.enrichMap {
		if state == enrichPending {
			return true
		}
	}
	return false
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

	// Render fixed elements first, then measure to compute remaining space.
	// This avoids fragile hardcoded arithmetic (e.g. "m.height - 4").

	// Header: tab bar + separator
	m.tabBar.SetWidth(m.width)
	header := m.tabBar.View() + "\n" +
		lipgloss.NewStyle().Foreground(styles.Muted).Render(strings.Repeat("─", m.width)) + "\n"

	// Footer
	footer := "\n" + m.renderFooter()

	// Main content fills the remaining vertical space
	mainHeight := m.height - lipgloss.Height(header) - lipgloss.Height(footer)
	if mainHeight < 3 {
		mainHeight = 3
	}
	sidebarW := m.width * 45 / 100
	tableWidth := m.width - sidebarW

	m.preview.SetSize(sidebarW, mainHeight)
	content := lipgloss.JoinHorizontal(lipgloss.Top, m.renderLeftPane(tableWidth, mainHeight), m.preview.View())

	result := header + content + footer
	if m.showHelp {
		popup := helppopup.Render(helpSections(), m.width)
		result = helppopup.Overlay(result, popup, m.width, m.height)
	}
	return result
}

// renderLeftPane renders the search bar, PR table, and scrollbar.
func (m Model) renderLeftPane(tableWidth, mainHeight int) string {
	var leftPane strings.Builder

	// Search bar (scoped to left pane width)
	var searchBar string
	if m.editing {
		searchBorder := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(styles.Primary).
			Width(tableWidth - 4).
			Padding(0, 1)
		searchBar = searchBorder.Render(m.filterInput.View()) + "\n"
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
		searchBar = searchBorder.Render(
			lipgloss.NewStyle().Foreground(styles.Muted).Render(qualifier),
		) + "\n"
	}
	leftPane.WriteString(searchBar)

	// Measure the rendered search bar instead of hardcoding "- 3"
	tableHeight := mainHeight - lipgloss.Height(searchBar)

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
		sb.VisibleItems = tableHeight / 4
		sb.ThumbColor = styles.Primary
		start, _ := m.visibleRange(tableHeight)
		sb.Offset = start

		sbView := sb.View()
		if sbView != "" {
			leftPane.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, tableContent, sbView))
		} else {
			leftPane.WriteString(tableContent)
		}
	}

	return leftPane.String()
}

// renderFooter renders the bottom bar with help text and repo label.
func (m Model) renderFooter() string {
	repoLabel := fmt.Sprintf("%s/%s", m.svc.RepoOwner(), m.svc.RepoName())
	repo := lipgloss.NewStyle().Foreground(styles.Primary).Render(repoLabel)

	var footerLeft string
	if m.flashMsg != "" {
		footerLeft = lipgloss.NewStyle().Foreground(styles.Success).Render(m.flashMsg)
	} else {
		footerLeft = styles.HelpStyle.Render("↑/k up  ↓/j down  enter select  tab switch  / search  y copy #  Y copy URL  ? help")
	}
	gap := m.width - lipgloss.Width(footerLeft) - lipgloss.Width(repo)
	if gap < 1 {
		gap = 1
	}
	return footerLeft + strings.Repeat(" ", gap) + repo
}

// helpSections returns the keybinding sections for the help popup.
func helpSections() []helppopup.Section {
	return []helppopup.Section{
		helppopup.Bind("Navigation", keys.Up, keys.Down, keys.HalfPageDown, keys.HalfPageUp, keys.Select),
		helppopup.Bind("Tabs & Filters", keys.NextTab, keys.PrevTab, keys.EditFilter),
		helppopup.Bind("Preview", keys.SidebarDown, keys.SidebarUp),
		helppopup.Bind("Copy", keys.CopyNumber, keys.CopyURL),
		helppopup.Bind("Other", keys.OpenInBrowser, keys.Refresh, keys.Help, keys.Quit),
	}
}

// setFlash sets a flash message and returns a command that clears it after a delay.
func (m *Model) setFlash(msg string) tea.Cmd {
	m.flashMsg = msg
	return tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg {
		return flashExpiredMsg{}
	})
}

// visibleRange computes the start and end indices of visible PR rows given
// the available height in terminal lines. Each PR row is 4 lines tall.
func (m Model) visibleRange(height int) (start, end int) {
	rowHeight := 4
	visibleRows := height / rowHeight
	if visibleRows < 1 {
		visibleRows = 1
	}
	start = 0
	if m.cursor >= visibleRows {
		start = m.cursor - visibleRows + 1
	}
	end = start + visibleRows
	if end > len(m.prs) {
		end = len(m.prs)
	}
	return start, end
}

// renderTable renders the PR table with multi-line rows (gh-dash style).
// Each PR gets three lines:
//
//	Line 1: state_icon  #number by @author
//	Line 2:             bold title
//	Line 3:             +N -N  ·  N files  ·  review_status  CI  ·  comments  ·  updated  age
func (m Model) renderTable(width, height int) string {
	start, end := m.visibleRange(height)

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

		// Line 3: stats on left, merge status flushed right
		dot := muted.Render(" · ")
		statsContent := indent +
			s(styles.Success).Render(fmt.Sprintf("+%s", formatNum(pr.Additions))) +
			muted.Render(" ") +
			s(styles.Danger).Render(fmt.Sprintf("-%s", formatNum(pr.Deletions))) +
			dot +
			s(lipgloss.BrightCyan).Render(iconFiles) + muted.Render(fmt.Sprintf(" %d", pr.Files))
		if pr.CommentCount > 0 {
			statsContent += dot +
				s(lipgloss.BrightBlue).Render(iconCommentSingle) + muted.Render(fmt.Sprintf(" %d", pr.CommentCount))
		}
		statsContent += dot +
			s(lipgloss.BrightMagenta).Render(iconUpdated) + muted.Render(" "+shortTimeAgo(pr.UpdatedAt))
		if m.enrichMap[pr.Number] == enrichPending {
			statsContent += dot + muted.Render(m.spinner.View())
		}

		mergeIcon, mergeLabel, mergeColor := mergeStateLabel(pr)
		mergeTag := s(mergeColor).Render(mergeIcon+" "+mergeLabel) + " "
		leftPart := base.Render(statsContent)
		line3 := base.Width(width).Render(
			leftPart + lipgloss.PlaceHorizontal(width-lipgloss.Width(leftPart), lipgloss.Right, mergeTag, lipgloss.WithWhitespaceStyle(base)),
		)

		row := line1 + "\n" + line2 + "\n" + line3
		borderStyle := lipgloss.NewStyle().
			BorderBottom(true).
			BorderStyle(lipgloss.Border{Bottom: "─"}).
			BorderForeground(styles.Muted)
		b.WriteString(borderStyle.Render(row) + "\n")
	}

	return b.String()
}

// openBrowser opens the given URL in the user's default browser.
func openBrowser(url string) tea.Cmd {
	return func() tea.Msg {
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("open", url)
		case "windows":
			cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
		default:
			cmd = exec.Command("xdg-open", url)
		}
		if err := cmd.Start(); err != nil {
			slog.Warn("failed to open URL in browser", "url", url, "error", err)
		}
		return nil
	}
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
	iconCommentSingle    = "\uf27b"     // nf-fa-commenting

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

// mergeStateLabel returns the icon, label text, and color for a PR's merge state.
func mergeStateLabel(pr review.PullRequest) (string, string, color.Color) {
	switch pr.MergeState {
	case "CLEAN", "HAS_HOOKS":
		return iconApproved, "Ready", lipgloss.Green
	case "BLOCKED":
		return iconChangesRequested, "Blocked", lipgloss.Red
	case "UNSTABLE":
		return iconWaiting, "Unstable", lipgloss.BrightYellow
	case "DIRTY":
		return iconChangesRequested, "Conflicts", lipgloss.Red
	case "DRAFT":
		return iconWaiting, "Draft", lipgloss.BrightBlack
	default:
		return iconWaiting, "Unknown", lipgloss.BrightBlack
	}
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

