package prpreview

import (
	"context"
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
	svc          review.Service
	sidebar      sidebar.Model
	sidebarWidth int
	previewPRNum int
	loading      bool
}

// New creates a new PR preview model.
func New(svc review.Service) Model {
	return Model{
		svc:          svc,
		sidebar:      sidebar.New(),
		sidebarWidth: 50,
	}
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

// BodyLoadedMsg carries the result of an async PR body fetch.
type BodyLoadedMsg struct {
	PRNumber int
	Body     string
	Err      error
}

// Refresh updates the sidebar preview for the given PR.
// It renders metadata immediately and returns a command to fetch the body if needed.
func (m *Model) Refresh(pr *review.PullRequest) func() BodyLoadedMsg {
	m.renderContent(pr, "")

	if pr.Number != m.previewPRNum {
		m.loading = true
		m.previewPRNum = pr.Number
		prNumber := pr.Number
		return func() BodyLoadedMsg {
			full, err := m.svc.GetPR(context.Background(), prNumber)
			if err != nil {
				return BodyLoadedMsg{PRNumber: prNumber, Err: err}
			}
			return BodyLoadedMsg{PRNumber: prNumber, Body: full.Body}
		}
	}
	return nil
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
	m.sidebar.SetContent("No PR selected")
}

func (m *Model) renderContent(pr *review.PullRequest, body string) {
	var b strings.Builder

	// Header: number + title on one line
	prLabel := lipgloss.NewStyle().Foreground(lipgloss.BrightYellow).Bold(true).Render(fmt.Sprintf("#%d", pr.Number))
	prTitle := lipgloss.NewStyle().Bold(true).Render(pr.Title)
	b.WriteString(prLabel + " " + prTitle + "\n\n")

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

	// Review status
	sectionHeader := lipgloss.NewStyle().Bold(true)
	b.WriteString(sectionHeader.Render("☷ Reviewers") + "\n")
	var reviewStatus string
	switch pr.ReviewDecision {
	case "APPROVED":
		reviewStatus = lipgloss.NewStyle().Foreground(styles.Success).Render("✓ Approved")
	case "CHANGES_REQUESTED":
		reviewStatus = lipgloss.NewStyle().Foreground(styles.Danger).Render("✗ Changes requested")
	case "REVIEW_REQUIRED":
		reviewStatus = lipgloss.NewStyle().Foreground(styles.Warning).Render("○ Review required")
	default:
		reviewStatus = lipgloss.NewStyle().Foreground(styles.Muted).Render("○ Pending")
	}
	b.WriteString(reviewStatus + "\n")
	if len(pr.PendingTeams) > 0 {
		teams := strings.Join(stripOrgPrefixes(pr.PendingTeams), ", ")
		b.WriteString(lipgloss.NewStyle().Foreground(styles.Warning).Render("  Waiting: "+teams) + "\n")
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

	// Changes
	b.WriteString(sectionHeader.Render("△ Changes") + "\n")
	add := lipgloss.NewStyle().Foreground(styles.Success).Render(fmt.Sprintf("+%d", pr.Additions))
	del := lipgloss.NewStyle().Foreground(styles.Danger).Render(fmt.Sprintf("-%d", pr.Deletions))
	b.WriteString(fmt.Sprintf("%d files changed  %s %s", pr.Files, add, del) + "\n\n")

	// CI status
	b.WriteString(sectionHeader.Render("◈ Checks") + "\n")
	if pr.ChecksPass == nil {
		b.WriteString(lipgloss.NewStyle().Foreground(styles.Muted).Render("○ No checks") + "\n")
	} else if *pr.ChecksPass {
		b.WriteString(lipgloss.NewStyle().Foreground(styles.Success).Render("✓ Checks passing") + "\n")
	} else {
		b.WriteString(lipgloss.NewStyle().Foreground(styles.Danger).Render("✗ Checks failing") + "\n")
	}
	if pr.ChecksSummary != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(styles.Muted).Render("  "+pr.ChecksSummary) + "\n")
	}
	b.WriteString("\n")

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
	return strings.Join(lines[:max], "\n") + "\n" + lipgloss.NewStyle().Foreground(styles.Muted).Render("  ⋯") + "\n"
}

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
