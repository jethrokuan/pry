package prpreview

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"

	"github.com/jethrokuan/pry/internal/review"
	"github.com/jethrokuan/pry/internal/ui/components/sidebar"
	"github.com/jethrokuan/pry/internal/ui/components/tabbar"
	"github.com/jethrokuan/pry/internal/ui/icons"
	"github.com/jethrokuan/pry/internal/ui/mdutil"
	"github.com/jethrokuan/pry/internal/ui/styles"
)

// Sidebar tab indices.
const (
	tabOverview = iota
	tabCommits
	tabChecks
)

// CommitsLoadedMsg carries the result of an async commits fetch.
type CommitsLoadedMsg struct {
	PRNumber int
	Commits  []review.Commit
	Err      error
}

// Model manages the PR preview sidebar content and async body fetching.
type Model struct {
	userIdentity *review.UserIdentity
	sidebar      sidebar.Model
	tabs         tabbar.Model
	sidebarWidth int
	previewPRNum int
	loading      bool

	// Cached state for re-rendering when switching tabs.
	cachedPR      *review.PullRequest
	cachedBody    string
	cachedCommits []review.Commit
	commitsLoaded bool // true once commits have been fetched for current PR
}

// New creates a new PR preview model.
func New() Model {
	return Model{
		sidebar:      sidebar.New(),
		sidebarWidth: 50,
		tabs: tabbar.New([]tabbar.Tab{
			{Label: icons.Overview + " Overview", Count: -1},
			{Label: icons.GitCommit + " Commits", Count: -1},
			{Label: icons.Checklist + " Checks", Count: -1},
		}),
	}
}

// SetUserIdentity updates the user identity used for review status rendering.
func (m *Model) SetUserIdentity(id *review.UserIdentity) {
	m.userIdentity = id
}

// isMyTeam reports whether the given team slug belongs to the current user.
func (m *Model) isMyTeam(slug string) bool {
	if m.userIdentity == nil {
		return false
	}
	for _, t := range m.userIdentity.Teams {
		// Teams are "org/slug"; match against the slug suffix.
		if strings.HasSuffix(t, "/"+slug) {
			return true
		}
	}
	return false
}

// SetSize updates the sidebar dimensions.
func (m *Model) SetSize(width, height int) {
	m.sidebarWidth = width
	m.tabs.SetWidth(width - 4) // account for sidebar border + padding
	m.sidebar.SetSize(width, height)
}

// ScrollDown scrolls the sidebar viewport down by n lines.
func (m *Model) ScrollDown(n int) {
	m.sidebar.ScrollDown(n)
}

// ScrollUp scrolls the sidebar viewport up by n lines.
func (m *Model) ScrollUp(n int) {
	m.sidebar.ScrollUp(n)
}

// NextSection moves to the next sidebar tab.
func (m *Model) NextSection() {
	if m.tabs.Next() {
		m.rerender()
	}
}

// PrevSection moves to the previous sidebar tab.
func (m *Model) PrevSection() {
	if m.tabs.Prev() {
		m.rerender()
	}
}

// rerender re-renders using cached PR data for the current tab.
func (m *Model) rerender() {
	if m.cachedPR != nil {
		m.renderContent(m.cachedPR, m.cachedBody)
	}
}

// View renders the sidebar.
func (m Model) View() string {
	return m.sidebar.View()
}

// BodyLoadedMsg carries the result of an async PR detail fetch.
// The full PR is included so the caller can merge detailed fields
// (check runs, reviewers, etc.) back into the list-level PR data.
type BodyLoadedMsg struct {
	PRNumber int
	Body     string
	FullPR   *review.PullRequest // full PR from GetPR (nil on error)
	Err      error
}

// Refresh updates the sidebar preview for the given PR.
// It renders with whatever data is currently available on the PR.
// Background enrichment (via prEnrichedMsg) will call HandleBodyLoaded
// when full data arrives.
func (m *Model) Refresh(pr *review.PullRequest) {
	if m.previewPRNum != pr.Number {
		// New PR — reset lazy-loaded data
		m.cachedCommits = nil
		m.commitsLoaded = false
	}
	m.previewPRNum = pr.Number
	m.loading = !pr.Enriched
	m.cachedPR = pr
	m.cachedBody = pr.Body
	m.renderContent(pr, pr.Body)
}

// HandleCommitsLoaded processes a CommitsLoadedMsg.
func (m *Model) HandleCommitsLoaded(msg CommitsLoadedMsg) {
	if msg.PRNumber != m.previewPRNum {
		return
	}
	m.commitsLoaded = true
	if msg.Err == nil {
		m.cachedCommits = msg.Commits
	}
	if m.tabs.Active() == tabCommits {
		m.rerender()
	}
}

// NeedsCommits returns true if the Commits tab is active but data hasn't been fetched yet.
func (m *Model) NeedsCommits() (int, bool) {
	if m.tabs.Active() == tabCommits && !m.commitsLoaded && m.previewPRNum > 0 {
		return m.previewPRNum, true
	}
	return 0, false
}

// HandleBodyLoaded processes a BodyLoadedMsg, updating the sidebar if it matches the current PR.
func (m *Model) HandleBodyLoaded(msg BodyLoadedMsg, pr *review.PullRequest) {
	if pr != nil && pr.Number == msg.PRNumber {
		m.loading = false
		m.cachedPR = pr
		if msg.Err == nil {
			m.cachedBody = msg.Body
			m.renderContent(pr, msg.Body)
		} else {
			m.cachedBody = ""
			m.renderContent(pr, "")
		}
	}
}

// ResetCache clears the cached PR number so the next Refresh will re-fetch.
func (m *Model) ResetCache() {
	m.previewPRNum = 0
}

// SetNoSelection shows a placeholder when no PR is selected.
func (m *Model) SetNoSelection() {
	m.sidebar.SetContent("Nothing selected yet")
}

func (m *Model) renderContent(pr *review.PullRequest, body string) {
	var b strings.Builder
	muted := lipgloss.NewStyle().Foreground(styles.Muted)

	// --- Persistent header (always visible, above tabs) ---

	// Title
	prLabel := lipgloss.NewStyle().Foreground(lipgloss.BrightYellow).Bold(true).Render(fmt.Sprintf("#%d", pr.Number))
	prTitle := lipgloss.NewStyle().Bold(true).Render(pr.Title)
	b.WriteString(prLabel + " " + prTitle + "\n")

	// State badge + branch info
	var stateBadge string
	switch {
	case pr.Draft:
		stateBadge = lipgloss.NewStyle().Foreground(styles.Muted).Render("◌ Draft")
	case pr.State == "MERGED":
		stateBadge = lipgloss.NewStyle().Foreground(styles.Secondary).Render("● Merged")
	case pr.State == "CLOSED":
		stateBadge = lipgloss.NewStyle().Foreground(styles.Danger).Render("● Closed")
	default:
		stateBadge = lipgloss.NewStyle().Foreground(styles.Success).Render("● Open")
	}
	pill := lipgloss.NewStyle().
		Foreground(lipgloss.BrightWhite).
		Background(styles.BgSelected).
		Padding(0, 1)
	arrow := muted.Render(" ← ")
	branchInfo := pill.Render(pr.Base) + arrow + pill.Render(pr.Branch)
	b.WriteString(stateBadge + "  " + branchInfo + "\n")

	// Author + time
	b.WriteString(fmt.Sprintf("by %s · %s",
		lipgloss.NewStyle().Foreground(styles.Cyan).Render("@"+pr.Author),
		timeAgo(pr.UpdatedAt)) + "\n")

	// Tab bar with bottom border
	contentWidth := m.sidebarWidth - 6
	if contentWidth < 10 {
		contentWidth = 10
	}
	b.WriteString("\n" + m.tabs.View() + "\n")
	b.WriteString(muted.Render(strings.Repeat("─", contentWidth)) + "\n\n")

	// --- Tab-specific content ---
	switch m.tabs.Active() {
	case tabOverview:
		m.renderOverview(&b, pr, body)
	case tabCommits:
		m.renderCommitsTab(&b, pr)
	case tabChecks:
		m.renderChecksTab(&b, pr)
	}

	m.sidebar.SetContent(b.String())
}

func (m *Model) renderOverview(b *strings.Builder, pr *review.PullRequest, body string) {
	sectionHeader := lipgloss.NewStyle().Bold(true)
	muted := lipgloss.NewStyle().Foreground(styles.Muted)

	// Changes summary
	add := lipgloss.NewStyle().Foreground(styles.Success).Render(fmt.Sprintf("+%d", pr.Additions))
	del := lipgloss.NewStyle().Foreground(styles.Danger).Render(fmt.Sprintf("-%d", pr.Deletions))
	commitLabel := "commits"
	if pr.Commits == 1 {
		commitLabel = "commit"
	}
	b.WriteString(muted.Render(fmt.Sprintf("%d %s · %d files  ", pr.Commits, commitLabel, pr.Files)) + add + " " + del + "\n\n")

	// Labels
	if len(pr.Labels) > 0 {
		b.WriteString(sectionHeader.Render("◫ Labels") + "\n")
		for _, l := range pr.Labels {
			b.WriteString(styles.LabelStyle.Render(l) + " ")
		}
		b.WriteString("\n\n")
	}

	// Summary (markdown body)
	if body != "" {
		b.WriteString(sectionHeader.Render("≡ Summary") + "\n")
		sidebarContentWidth := m.sidebarWidth - 5
		if sidebarContentWidth < 20 {
			sidebarContentWidth = 20
		}
		renderer, err := glamour.NewTermRenderer(
			glamour.WithStylePath("dark"),
			glamour.WithWordWrap(sidebarContentWidth),
		)
		if err == nil {
			rendered, err := renderer.Render(mdutil.ReplaceImages(body))
			if err == nil {
				b.WriteString(truncateLines(rendered, 15))
			} else {
				b.WriteString(truncateLines(body, 15))
			}
		} else {
			b.WriteString(truncateLines(body, 15))
		}
	} else if m.loading {
		b.WriteString(sectionHeader.Render("≡ Summary") + "\n")
		b.WriteString(muted.Render("Loading...") + "\n")
	}

	// Checks summary box
	boxWidth := m.sidebarWidth - 6
	if boxWidth < 20 {
		boxWidth = 20
	}
	b.WriteString(sectionHeader.Render("☐ Checks") + "\n")
	b.WriteString(m.renderChecksBox(pr, boxWidth) + "\n")
}

func (m *Model) renderCommitsTab(b *strings.Builder, pr *review.PullRequest) {
	muted := lipgloss.NewStyle().Foreground(styles.Muted)
	sectionHeader := lipgloss.NewStyle().Bold(true)

	commitLabel := "commits"
	if pr.Commits == 1 {
		commitLabel = "commit"
	}
	b.WriteString(sectionHeader.Render(fmt.Sprintf("%s %d %s", icons.GitCommit, pr.Commits, commitLabel)) + "\n\n")

	if !m.commitsLoaded {
		b.WriteString(muted.Render("Loading commits...") + "\n")
		return
	}
	if len(m.cachedCommits) == 0 {
		b.WriteString(muted.Render("No commits") + "\n")
		return
	}

	contentWidth := m.sidebarWidth - 6
	if contentWidth < 20 {
		contentWidth = 20
	}

	for _, c := range m.cachedCommits {
		// Check status icon
		var checkIcon string
		switch {
		case c.ChecksPass == nil:
			checkIcon = lipgloss.NewStyle().Foreground(styles.Muted).Render("○")
		case *c.ChecksPass:
			checkIcon = lipgloss.NewStyle().Foreground(styles.Success).Render("✓")
		default:
			checkIcon = lipgloss.NewStyle().Foreground(styles.Danger).Render("✗")
		}

		// First line: "✓ <message>              <sha>"
		// Use lipgloss to truncate message and right-align SHA.
		sha := muted.Render(c.ShortSHA)
		shaWidth := lipgloss.Width(sha)
		iconWidth := lipgloss.Width(checkIcon) + 1 // icon + space
		msgWidth := contentWidth - iconWidth - shaWidth - 2
		if msgWidth < 10 {
			msgWidth = 10
		}
		msg := lipgloss.NewStyle().
			Width(msgWidth).MaxWidth(msgWidth).
			Height(1).MaxHeight(1).
			Render(c.Message)
		row := checkIcon + " " + msg + "  " + sha
		b.WriteString(lipgloss.PlaceHorizontal(contentWidth, lipgloss.Left, row) + "\n")

		// Second line: "  @author 2h ago · ✓ 451/22"
		add := lipgloss.NewStyle().Foreground(styles.Success).Render(fmt.Sprintf("%d", c.Additions))
		del := lipgloss.NewStyle().Foreground(styles.Danger).Render(fmt.Sprintf("%d", c.Deletions))
		b.WriteString(muted.Render(fmt.Sprintf("  @%s %s · ", c.Author, timeAgo(c.CommittedAt))) + checkIcon + " " + add + "/" + del + "\n\n")
	}
}

func (m *Model) renderChecksTab(b *strings.Builder, pr *review.PullRequest) {
	sectionHeader := lipgloss.NewStyle().Bold(true)
	muted := lipgloss.NewStyle().Foreground(styles.Muted)

	boxWidth := m.sidebarWidth - 6
	if boxWidth < 20 {
		boxWidth = 20
	}
	b.WriteString(m.renderChecksBox(pr, boxWidth) + "\n\n")

	// Reviewers section
	if len(pr.Reviewers) > 0 {
		b.WriteString(sectionHeader.Render("Reviewers") + "\n\n")
		for _, r := range pr.Reviewers {
			if r.State == "" {
				continue
			}
			var stateIcon string
			var style lipgloss.Style
			switch r.State {
			case "APPROVED":
				stateIcon = "✓"
				style = lipgloss.NewStyle().Foreground(styles.Success)
			case "CHANGES_REQUESTED":
				stateIcon = "●"
				style = lipgloss.NewStyle().Foreground(styles.Danger)
			case "COMMENTED", "DISMISSED":
				stateIcon = icons.Comment
				style = lipgloss.NewStyle().Foreground(styles.Muted)
			default: // PENDING
				stateIcon = "●"
				style = lipgloss.NewStyle().Foreground(styles.Warning)
			}
			name := r.Login
			if r.IsTeam && m.isMyTeam(r.Login) {
				name = lipgloss.NewStyle().Bold(true).Render(name)
			}
			b.WriteString("  " + style.Render(stateIcon) + " " + name + "\n")
		}
		b.WriteString("\n")
	}

	// All Checks list (sorted: failures first, then pending, then skipped, then success)
	if len(pr.CheckRuns) > 0 {
		sorted := make([]review.CheckRun, len(pr.CheckRuns))
		copy(sorted, pr.CheckRuns)
		sort.Slice(sorted, func(i, j int) bool {
			return checkRunSortOrder(sorted[i]) < checkRunSortOrder(sorted[j])
		})

		b.WriteString(sectionHeader.Render(icons.Checklist+" All Checks") + "\n\n")
		for _, cr := range sorted {
			var checkIcon string
			switch {
			case cr.Status == review.CheckRunCompleted && cr.Conclusion == review.ConclusionSuccess:
				checkIcon = lipgloss.NewStyle().Foreground(styles.Success).Render("✓")
			case cr.Status == review.CheckRunCompleted && (cr.Conclusion == review.ConclusionFailure || cr.Conclusion == review.ConclusionTimedOut || cr.Conclusion == review.ConclusionStartupFailure || cr.Conclusion == review.ConclusionActionRequired):
				checkIcon = lipgloss.NewStyle().Foreground(styles.Danger).Render("✗")
			case cr.Status == review.CheckRunCompleted && (cr.Conclusion == review.ConclusionSkipped || cr.Conclusion == review.ConclusionNeutral || cr.Conclusion == review.ConclusionCancelled):
				checkIcon = muted.Render("○")
			default:
				checkIcon = lipgloss.NewStyle().Foreground(styles.Warning).Render("◑")
			}
			b.WriteString("  " + checkIcon + " " + cr.Name + "\n")
		}
	} else if m.loading {
		b.WriteString(muted.Render("Loading checks...") + "\n")
	}
}

// renderChecksBox renders a bordered box with review, CI checks, and merge status subsections.
func (m *Model) renderChecksBox(pr *review.PullRequest, boxWidth int) string {
	muted := lipgloss.NewStyle().Foreground(styles.Muted)
	innerWidth := boxWidth - 4

	var sections []string

	// 1. Review status subsection
	if sec := reviewSubsection(pr, m.userIdentity); sec != "" {
		sections = append(sections, sec)
	}

	// 2. CI checks subsection
	if sec := checksSubsection(pr, innerWidth); sec != "" {
		sections = append(sections, sec)
	}

	// 3. Merge/draft status subsection
	if sec := mergeSubsection(pr); sec != "" {
		sections = append(sections, sec)
	}

	if m.loading && len(sections) == 0 {
		sections = append(sections, muted.Render("Loading..."))
	}

	if len(sections) == 0 {
		sections = append(sections, lipgloss.NewStyle().Foreground(styles.Success).Render("✓ All checks passed"))
	}

	// Determine border color from worst status
	borderColor := styles.Success
	switch {
	case pr.MergeState == "BLOCKED" || pr.MergeState == "DIRTY":
		borderColor = styles.Danger
	case pr.CheckCounts.Failing > 0:
		borderColor = styles.Danger
	case pr.ChecksPass != nil && !*pr.ChecksPass:
		borderColor = styles.Danger
	case pr.MergeState == "UNSTABLE" || pr.MergeState == "DRAFT" || pr.Draft:
		borderColor = styles.Warning
	case pr.CheckCounts.Pending > 0:
		borderColor = styles.Warning
	case pr.MergeState == "":
		borderColor = styles.Muted
	}

	rule := muted.Render(strings.Repeat("─", innerWidth))
	content := strings.Join(sections, "\n"+rule+"\n")

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		Width(boxWidth).
		Render(content)
}

// reviewSubsection renders the review status part of the Checks box.
func reviewSubsection(pr *review.PullRequest, _ *review.UserIdentity) string {
	bold := lipgloss.NewStyle().Bold(true)
	muted := lipgloss.NewStyle().Foreground(styles.Muted)

	switch pr.ReviewDecision {
	case "APPROVED":
		var count int
		for _, r := range pr.Reviewers {
			if r.State == "APPROVED" {
				count++
			}
		}
		subtitle := fmt.Sprintf("%d approved", count)
		return lipgloss.NewStyle().Foreground(styles.Success).Render("✓ ") +
			bold.Render("Approved") + "\n" + muted.Render("  "+subtitle)

	case "CHANGES_REQUESTED":
		var changesCount, approvedCount int
		for _, r := range pr.Reviewers {
			switch r.State {
			case "CHANGES_REQUESTED":
				changesCount++
			case "APPROVED":
				approvedCount++
			}
		}
		// latestReviews may not reflect "changes requested" if the reviewer
		// commented afterward — GitHub tracks it as sticky until approval.
		// Fall back to a descriptive message when count is 0.
		var subtitle string
		if changesCount > 0 {
			parts := []string{fmt.Sprintf("%d requesting changes", changesCount)}
			if approvedCount > 0 {
				parts = append(parts, fmt.Sprintf("%d approved", approvedCount))
			}
			subtitle = strings.Join(parts, ", ")
		} else {
			subtitle = "Changes must be addressed to merge"
		}
		return lipgloss.NewStyle().Foreground(styles.Danger).Render("✗ ") +
			bold.Render("Changes requested") + "\n" + muted.Render("  "+subtitle)

	case "REVIEW_REQUIRED":
		var commented int
		for _, r := range pr.Reviewers {
			if r.State == "COMMENTED" || r.State == "DISMISSED" {
				commented++
			}
		}
		var subtitle string
		if commented > 0 {
			label := "reviewer left comments"
			if commented > 1 {
				label = "reviewers left comments"
			}
			subtitle = fmt.Sprintf("%d %s", commented, label)
		} else {
			subtitle = "Awaiting required reviews"
		}
		return lipgloss.NewStyle().Foreground(styles.Warning).Render("◑ ") +
			bold.Render("Review Required") + "\n" + muted.Render("  "+subtitle)
	}
	return ""
}

// checksSubsection renders the CI checks part of the Checks box.
func checksSubsection(pr *review.PullRequest, barWidth int) string {
	cc := pr.CheckCounts
	if cc.Total == 0 && len(pr.CheckRuns) > 0 {
		cc = countsFromCheckRuns(pr.CheckRuns, pr.ChecksTotal)
	}
	if cc.Total == 0 {
		return ""
	}

	bold := lipgloss.NewStyle().Bold(true)
	muted := lipgloss.NewStyle().Foreground(styles.Muted)

	allPassed := cc.Failing == 0 && cc.Pending == 0
	if allPassed {
		title := lipgloss.NewStyle().Foreground(styles.Success).Render("✓ ") +
			bold.Render("All checks passed")
		return title + "\n" + muted.Render(fmt.Sprintf("  %d successful", cc.Passing))
	}

	// Title
	var icon, titleText string
	if cc.Failing > 0 {
		icon = lipgloss.NewStyle().Foreground(styles.Danger).Render("✗ ")
		titleText = "Some checks were not successful"
	} else {
		icon = lipgloss.NewStyle().Foreground(styles.Warning).Render("◑ ")
		titleText = "Some checks are pending"
	}
	title := icon + bold.Render(titleText)

	// Stats line: "1 failing, 60 skipped, 48 successful"
	var statParts []string
	if cc.Failing > 0 {
		statParts = append(statParts, fmt.Sprintf("%d failing", cc.Failing))
	}
	if cc.Pending > 0 {
		statParts = append(statParts, fmt.Sprintf("%d pending", cc.Pending))
	}
	if cc.Skipped > 0 {
		statParts = append(statParts, fmt.Sprintf("%d skipped", cc.Skipped))
	}
	if cc.Passing > 0 {
		statParts = append(statParts, fmt.Sprintf("%d successful", cc.Passing))
	}
	subtitle := muted.Render("  " + strings.Join(statParts, ", "))

	// Progress bar
	bar := renderChecksProgressBar(cc, barWidth)

	result := title + "\n" + subtitle
	if bar != "" {
		result += "\n" + bar
	}
	return result
}

// mergeSubsection renders the merge/draft status part of the Checks box.
func mergeSubsection(pr *review.PullRequest) string {
	bold := lipgloss.NewStyle().Bold(true)
	muted := lipgloss.NewStyle().Foreground(styles.Muted)

	if pr.Draft {
		return lipgloss.NewStyle().Foreground(styles.Muted).Render("◌ ") +
			bold.Render("This pull request is still a work in progress") + "\n" +
			muted.Render("  Draft pull requests cannot be merged")
	}
	if pr.Mergeable == "CONFLICTING" {
		title := lipgloss.NewStyle().Foreground(styles.Danger).Render("✗ ") +
			bold.Render("Merge conflicts")
		if len(pr.ConflictFiles) > 0 {
			const maxShow = 3
			show := pr.ConflictFiles
			if len(show) > maxShow {
				show = show[:maxShow]
			}
			title += "\n" + muted.Render("  "+strings.Join(show, ", "))
			if len(pr.ConflictFiles) > maxShow {
				title += muted.Render(fmt.Sprintf(", +%d more", len(pr.ConflictFiles)-maxShow))
			}
		}
		return title
	}
	// Don't show a merge subsection if everything is clean
	return ""
}

func truncateLines(s string, max int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= max {
		return s
	}
	remaining := len(lines) - max
	label := fmt.Sprintf("  ⋯ %d more lines", remaining)
	return strings.Join(lines[:max], "\n") + "\n" + lipgloss.NewStyle().Foreground(styles.Muted).Italic(true).Render(label) + "\n"
}

// renderChecksProgressBar renders a proportional progress bar using ▃ characters.
func renderChecksProgressBar(cc review.CheckCounts, width int) string {
	if cc.Total == 0 {
		return ""
	}

	// Calculate proportional widths, ensuring at least 1 char for non-zero segments.
	type segment struct {
		count int
		color lipgloss.Style
	}
	segments := []segment{
		{cc.Failing, lipgloss.NewStyle().Foreground(styles.Danger)},
		{cc.Pending, lipgloss.NewStyle().Foreground(styles.Warning)},
		{cc.Skipped, lipgloss.NewStyle().Foreground(styles.Muted)},
		{cc.Passing, lipgloss.NewStyle().Foreground(styles.Success)},
	}

	// Distribute width proportionally
	widths := make([]int, len(segments))
	remaining := width
	for i, s := range segments {
		if s.count > 0 {
			w := s.count * width / cc.Total
			if w == 0 {
				w = 1
			}
			widths[i] = w
			remaining -= w
		}
	}
	// Distribute leftover to largest segment
	if remaining > 0 {
		maxIdx := 0
		for i, s := range segments {
			if s.count > segments[maxIdx].count {
				maxIdx = i
			}
		}
		widths[maxIdx] += remaining
	}

	var bar strings.Builder
	for i, s := range segments {
		if widths[i] > 0 {
			bar.WriteString(s.color.Render(strings.Repeat("▃", widths[i])))
		}
	}
	return bar.String()
}

// countsFromCheckRuns computes CheckCounts by iterating individual CheckRuns.
// Used as fallback when pre-aggregated API counts aren't available.
func countsFromCheckRuns(runs []review.CheckRun, apiTotal int) review.CheckCounts {
	var cc review.CheckCounts
	for _, cr := range runs {
		switch {
		case cr.Status == review.CheckRunCompleted && (cr.Conclusion == review.ConclusionFailure || cr.Conclusion == review.ConclusionTimedOut || cr.Conclusion == review.ConclusionStartupFailure || cr.Conclusion == review.ConclusionActionRequired || cr.Conclusion == review.ConclusionCancelled):
			cc.Failing++
		case cr.Status == review.CheckRunInProgress || cr.Status == review.CheckRunQueued:
			cc.Pending++
		case cr.Status == review.CheckRunCompleted && (cr.Conclusion == review.ConclusionSkipped || cr.Conclusion == review.ConclusionNeutral):
			cc.Skipped++
		case cr.Status == review.CheckRunCompleted && cr.Conclusion == review.ConclusionSuccess:
			cc.Passing++
		}
	}
	cc.Total = cc.Failing + cc.Passing + cc.Skipped + cc.Pending
	if apiTotal > cc.Total {
		cc.Total = apiTotal
	}
	return cc
}

// checkRunSortOrder returns a sort key: failures first, then pending, skipped, success.
func checkRunSortOrder(cr review.CheckRun) int {
	switch {
	case cr.Status == review.CheckRunCompleted && (cr.Conclusion == review.ConclusionFailure || cr.Conclusion == review.ConclusionTimedOut || cr.Conclusion == review.ConclusionStartupFailure || cr.Conclusion == review.ConclusionActionRequired):
		return 0
	case cr.Status == review.CheckRunInProgress || cr.Status == review.CheckRunQueued:
		return 1
	case cr.Status == review.CheckRunCompleted && (cr.Conclusion == review.ConclusionSkipped || cr.Conclusion == review.ConclusionNeutral || cr.Conclusion == review.ConclusionCancelled):
		return 2
	default: // SUCCESS
		return 3
	}
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
