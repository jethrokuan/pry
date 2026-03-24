package prpreview

import (
	"fmt"
	"strings"
	"time"

	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"

	"github.com/jethrokuan/pry/internal/review"
	"github.com/jethrokuan/pry/internal/ui/components/sidebar"
	"github.com/jethrokuan/pry/internal/ui/mdutil"
	"github.com/jethrokuan/pry/internal/ui/styles"
)

// Model manages the PR preview sidebar content and async body fetching.
type Model struct {
	userIdentity *review.UserIdentity
	sidebar      sidebar.Model
	sidebarWidth int
	previewPRNum int
	loading      bool
}

// New creates a new PR preview model.
func New() Model {
	return Model{
		sidebar:      sidebar.New(),
		sidebarWidth: 50,
	}
}

// SetUserIdentity updates the user identity used for review status rendering.
func (m *Model) SetUserIdentity(id *review.UserIdentity) {
	m.userIdentity = id
}

// SetSize updates the sidebar dimensions.
func (m *Model) SetSize(width, height int) {
	m.sidebarWidth = width
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
	m.previewPRNum = pr.Number
	// If check runs are loaded, the PR has been enriched.
	m.loading = pr.CheckRuns == nil
	m.renderContent(pr, pr.Body)
}

// HandleBodyLoaded processes a BodyLoadedMsg, updating the sidebar if it matches the current PR.
func (m *Model) HandleBodyLoaded(msg BodyLoadedMsg, pr *review.PullRequest) {
	if pr != nil && pr.Number == msg.PRNumber {
		m.loading = false
		if msg.Err == nil {
			m.renderContent(pr, msg.Body)
		} else {
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

	// Header: number + title on one line
	prLabel := lipgloss.NewStyle().Foreground(lipgloss.BrightYellow).Bold(true).Render(fmt.Sprintf("#%d", pr.Number))
	prTitle := lipgloss.NewStyle().Bold(true).Render(pr.Title)
	b.WriteString(prLabel + " " + prTitle + "\n")

	// Changes summary (compact, right below title)
	add := lipgloss.NewStyle().Foreground(styles.Success).Render(fmt.Sprintf("+%d", pr.Additions))
	del := lipgloss.NewStyle().Foreground(styles.Danger).Render(fmt.Sprintf("-%d", pr.Deletions))
	muted := lipgloss.NewStyle().Foreground(styles.Muted)
	commitLabel := "commits"
	if pr.Commits == 1 {
		commitLabel = "commit"
	}
	b.WriteString(muted.Render(fmt.Sprintf("%d %s · %d files changed  ", pr.Commits, commitLabel, pr.Files)) + add + " " + del + "\n\n")

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
	renderPill := func(text string) string {
		return pill.Render(text)
	}
	arrow := lipgloss.NewStyle().Foreground(styles.Muted).Render(" ← ")
	branchInfo := renderPill(pr.Base) + arrow + renderPill(pr.Branch)
	b.WriteString(stateBadge + "  " + branchInfo + "\n")

	// Author + time
	authorLine := fmt.Sprintf("by %s · %s",
		lipgloss.NewStyle().Foreground(styles.Cyan).Render("@"+pr.Author),
		timeAgo(pr.UpdatedAt))
	b.WriteString(authorLine + "\n\n")

	// Merge status
	sectionHeader := lipgloss.NewStyle().Bold(true)
	b.WriteString(sectionHeader.Render("☷ Merge Status") + "\n")
	var mergeStatus string
	switch pr.MergeState {
	case "CLEAN":
		mergeStatus = lipgloss.NewStyle().Foreground(styles.Success).Render("✓ Ready to merge")
	case "HAS_HOOKS":
		mergeStatus = lipgloss.NewStyle().Foreground(styles.Success).Render("✓ Ready (has hooks)")
	case "DRAFT":
		mergeStatus = lipgloss.NewStyle().Foreground(styles.Muted).Render("◌ Draft")
	case "BLOCKED":
		mergeStatus = lipgloss.NewStyle().Foreground(styles.Danger).Render("✗ Blocked")
	case "UNSTABLE":
		mergeStatus = lipgloss.NewStyle().Foreground(styles.Warning).Render("○ Unstable")
	case "DIRTY":
		mergeStatus = lipgloss.NewStyle().Foreground(styles.Danger).Render("✗ Merge conflicts")
	default:
		mergeStatus = lipgloss.NewStyle().Foreground(styles.Muted).Render("○ Unknown")
	}
	b.WriteString(mergeStatus + "\n")

	// Blocking reasons / clean status detail.
	// These sections depend on check runs and reviewers which are only
	// available after the full PR detail (GetPR) loads. Show a loading
	// indicator until then to avoid misleading empty data.
	if m.loading {
		b.WriteString(lipgloss.NewStyle().Foreground(styles.Muted).Render("  Loading details...") + "\n")
	} else if pr.MergeState == "BLOCKED" || pr.MergeState == "UNSTABLE" || pr.MergeState == "DIRTY" {
		if pr.Mergeable == "CONFLICTING" {
			b.WriteString(lipgloss.NewStyle().Foreground(styles.Danger).Render("  ✗ Merge conflicts") + "\n")
			renderConflictFiles(&b, pr.ConflictFiles)
		}
		renderReviewStatus(&b, pr, m.userIdentity)
		renderCheckRunsDetail(&b, pr)
	} else if pr.MergeState == "CLEAN" || pr.MergeState == "HAS_HOOKS" {
		renderCleanStatus(&b, pr)
	}
	b.WriteString("\n")

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
		b.WriteString(lipgloss.NewStyle().Foreground(styles.Muted).Render("Loading...") + "\n")
	}

	m.sidebar.SetContent(b.String())
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

// renderReviewStatus renders the review/approval status line with pending reviewer names.
func renderReviewStatus(b *strings.Builder, pr *review.PullRequest, userIdentity *review.UserIdentity) {
	if pr.ReviewDecision == "CHANGES_REQUESTED" {
		// Collect reviewers who requested changes
		var requesters []string
		for _, r := range pr.Reviewers {
			if r.State == "CHANGES_REQUESTED" {
				requesters = append(requesters, r.Login)
			}
		}

		if len(requesters) == 0 {
			b.WriteString(lipgloss.NewStyle().Foreground(styles.Danger).Render("  ✗ Changes requested") + "\n")
		} else {
			const maxVisible = 4
			display := requesters
			var overflow int
			if len(display) > maxVisible {
				overflow = len(display) - maxVisible
				display = display[:maxVisible]
			}
			dangerStyle := lipgloss.NewStyle().Foreground(styles.Danger)
			names := strings.Join(display, dangerStyle.Render(", "))
			suffix := ""
			if overflow > 0 {
				suffix = lipgloss.NewStyle().Foreground(styles.Muted).Render(fmt.Sprintf(", +%d more", overflow))
			}
			b.WriteString(dangerStyle.Render("  ✗ Changes requested by: ") + names + suffix + "\n")
		}
		return
	}
	if pr.ReviewDecision != "REVIEW_REQUIRED" {
		return
	}

	// Collect pending reviewer names (individuals + teams)
	var pending []string
	userTeams := make(map[string]bool)
	if userIdentity != nil {
		for _, t := range userIdentity.Teams {
			userTeams[t] = true
		}
	}

	for _, r := range pr.Reviewers {
		if r.State == "PENDING" && !r.IsTeam {
			pending = append(pending, r.Login)
		}
	}
	for _, t := range pr.PendingTeams {
		pending = append(pending, stripOrgPrefix(t))
	}

	// If no explicit pending reviewers, show reviewers who interacted but haven't approved
	// (e.g., COMMENTED or DISMISSED — they're still required to approve).
	// This handles cases where a team reviewer commented (removing the team from
	// reviewRequests) but hasn't approved yet.
	if len(pending) == 0 {
		for _, r := range pr.Reviewers {
			if r.State != "" && r.State != "APPROVED" && r.State != "PENDING" {
				pending = append(pending, r.Login)
			}
		}
	}

	if len(pending) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(styles.Warning).Render("  ○ Awaiting required reviews") + "\n")
		return
	}

	const maxVisible = 4
	display := pending
	var overflow int
	if len(display) > maxVisible {
		overflow = len(display) - maxVisible
		display = display[:maxVisible]
	}

	highlight := lipgloss.NewStyle().Foreground(styles.Primary).Bold(true).Underline(true)
	normal := lipgloss.NewStyle().Foreground(styles.Warning)

	var parts []string
	for _, name := range display {
		// Highlight if name matches a user's team
		isUserTeam := false
		for _, t := range pr.PendingTeams {
			if stripOrgPrefix(t) == name && userTeams[t] {
				isUserTeam = true
				break
			}
		}
		if isUserTeam {
			parts = append(parts, highlight.Render(name))
		} else {
			parts = append(parts, normal.Render(name))
		}
	}

	prefix := lipgloss.NewStyle().Foreground(styles.Warning).Render("  ○ Awaiting reviews: ")
	names := strings.Join(parts, lipgloss.NewStyle().Foreground(styles.Warning).Render(", "))
	if overflow > 0 {
		names += lipgloss.NewStyle().Foreground(styles.Muted).Render(fmt.Sprintf(", +%d more", overflow))
	}
	b.WriteString(prefix + names + "\n")
}

// renderConflictFiles renders the list of conflicted file names.
func renderConflictFiles(b *strings.Builder, files []string) {
	if len(files) == 0 {
		return
	}
	fileStyle := lipgloss.NewStyle().Foreground(styles.Muted)
	const maxVisible = 5
	show := files
	if len(show) > maxVisible {
		show = show[:maxVisible]
	}
	for _, f := range show {
		b.WriteString(fileStyle.Render("    "+f) + "\n")
	}
	if len(files) > maxVisible {
		b.WriteString(fileStyle.Italic(true).Render(fmt.Sprintf("    +%d more", len(files)-maxVisible)) + "\n")
	}
}

// renderCheckRunsDetail renders the check runs summary and detail lines.
func renderCheckRunsDetail(b *strings.Builder, pr *review.PullRequest) {
	if len(pr.CheckRuns) == 0 {
		// No detailed check runs yet — don't render from the summary boolean
		// as it may be stale. Enrichment will provide real data.
		return
	}

	var failed, running, passed, skipped, other []review.CheckRun
	for _, cr := range pr.CheckRuns {
		switch {
		case cr.Status == review.CheckRunCompleted && (cr.Conclusion == review.ConclusionFailure || cr.Conclusion == review.ConclusionTimedOut || cr.Conclusion == review.ConclusionStartupFailure || cr.Conclusion == review.ConclusionActionRequired):
			failed = append(failed, cr)
		case cr.Status == review.CheckRunInProgress || cr.Status == review.CheckRunQueued:
			running = append(running, cr)
		case cr.Status == review.CheckRunCompleted && (cr.Conclusion == review.ConclusionSkipped || cr.Conclusion == review.ConclusionCancelled):
			skipped = append(skipped, cr)
		case cr.Status == review.CheckRunCompleted && (cr.Conclusion == review.ConclusionSuccess || cr.Conclusion == review.ConclusionNeutral):
			passed = append(passed, cr)
		default:
			other = append(other, cr)
		}
	}

	allPassed := len(failed) == 0 && len(running) == 0
	if allPassed {
		total := len(passed) + len(skipped) + len(other)
		b.WriteString(lipgloss.NewStyle().Foreground(styles.Success).Render(fmt.Sprintf("  ✓ %d/%d checks passed", total, total)) + "\n")
		return
	}

	// Summary line: "Checks: 2 failing, 1 running · ✓ 5 passed"
	var summaryParts []string
	if len(failed) > 0 {
		summaryParts = append(summaryParts, lipgloss.NewStyle().Foreground(styles.Danger).Render(fmt.Sprintf("%d failing", len(failed))))
	}
	if len(running) > 0 {
		summaryParts = append(summaryParts, lipgloss.NewStyle().Foreground(styles.Warning).Render(fmt.Sprintf("%d running", len(running))))
	}

	prefix := lipgloss.NewStyle().Foreground(styles.Danger).Render("  ✗ Checks: ")
	rest := strings.Join(summaryParts, lipgloss.NewStyle().Foreground(styles.Muted).Render(", "))
	if len(passed) > 0 {
		rest += lipgloss.NewStyle().Foreground(styles.Muted).Render(" · ") +
			lipgloss.NewStyle().Foreground(styles.Success).Render(fmt.Sprintf("✓ %d passed", len(passed)))
	}
	b.WriteString(prefix + rest + "\n")

	// Detail lines: show failed and running checks, one per line
	var detailChecks []review.CheckRun
	detailChecks = append(detailChecks, failed...)
	detailChecks = append(detailChecks, running...)

	const maxDetailChecks = 6
	showChecks := detailChecks
	if len(showChecks) > maxDetailChecks {
		showChecks = showChecks[:maxDetailChecks]
	}

	for _, cr := range showChecks {
		var icon string
		var style lipgloss.Style
		switch {
		case cr.Status == review.CheckRunCompleted && cr.Conclusion != review.ConclusionSuccess:
			icon = "✗"
			style = lipgloss.NewStyle().Foreground(styles.Danger)
		case cr.Status == review.CheckRunInProgress:
			icon = "◑"
			style = lipgloss.NewStyle().Foreground(styles.Warning)
		case cr.Status == review.CheckRunQueued:
			icon = "○"
			style = lipgloss.NewStyle().Foreground(styles.Muted)
		default:
			icon = "○"
			style = lipgloss.NewStyle().Foreground(styles.Muted)
		}
		entry := icon + " " + cr.Name
		dur := formatCheckDuration(cr)
		if dur != "" {
			entry += " (" + dur + ")"
		}
		b.WriteString("    " + style.Render(entry) + "\n")
	}

	if len(detailChecks) > maxDetailChecks {
		b.WriteString("    " + lipgloss.NewStyle().Foreground(styles.Muted).Render(fmt.Sprintf("+%d more", len(detailChecks)-maxDetailChecks)) + "\n")
	}
}

// renderCleanStatus renders the status line when PR is ready to merge.
func renderCleanStatus(b *strings.Builder, pr *review.PullRequest) {
	var parts []string
	if len(pr.CheckRuns) > 0 {
		total := len(pr.CheckRuns)
		parts = append(parts, lipgloss.NewStyle().Foreground(styles.Success).Render(fmt.Sprintf("✓ %d/%d checks passed", total, total)))
	} else if pr.ChecksPass != nil && *pr.ChecksPass {
		parts = append(parts, lipgloss.NewStyle().Foreground(styles.Success).Render("✓ Checks passing"))
	}
	if pr.ReviewDecision == "APPROVED" {
		parts = append(parts, lipgloss.NewStyle().Foreground(styles.Success).Render("✓ Approved"))
	}
	if len(parts) > 0 {
		b.WriteString("  " + strings.Join(parts, lipgloss.NewStyle().Foreground(styles.Muted).Render(" · ")) + "\n")
	}
}

// formatCheckDuration formats the duration of a check run.
func formatCheckDuration(cr review.CheckRun) string {
	if cr.StartedAt.IsZero() {
		return ""
	}
	var d time.Duration
	if cr.Status == review.CheckRunInProgress || cr.CompletedAt.IsZero() {
		d = time.Since(cr.StartedAt)
	} else {
		d = cr.CompletedAt.Sub(cr.StartedAt)
	}

	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	suffix := ""
	if cr.Status == review.CheckRunInProgress {
		suffix = "↑"
	}

	if minutes > 0 {
		return fmt.Sprintf("%dm %02ds%s", minutes, seconds, suffix)
	}
	return fmt.Sprintf("%ds%s", seconds, suffix)
}

func stripOrgPrefix(slug string) string {
	if i := strings.Index(slug, "/"); i >= 0 {
		return slug[i+1:]
	}
	return slug
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
