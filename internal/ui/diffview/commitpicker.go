package diffview

import (
	"fmt"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jethrokuan/pry/internal/data"
	"github.com/jethrokuan/pry/internal/ui/components/flash"
	"github.com/jethrokuan/pry/internal/ui/styles"
)

// openCommitPicker opens the commit picker overlay, loading commits if needed.
func (m Model) openCommitPicker() (Model, tea.Cmd) {
	m.commitPickerActive = true
	m.commitPickerAnchor = -1
	if !m.commitsLoaded {
		return m, m.fetchCommitsCmd()
	}
	return m, nil
}

func (m Model) fetchCommitsCmd() tea.Cmd {
	prNumber := m.pr.Number
	return func() tea.Msg {
		commits, err := data.FetchCommits(prNumber)
		return commitsLoadedMsg{commits: commits, err: err}
	}
}

// commitBaseSHA returns the parent SHA for the commit at index idx.
// For the first commit in the PR, returns the PR base SHA.
func (m Model) commitBaseSHA(idx int) string {
	if idx > 0 {
		return m.commits[idx-1].SHA
	}
	return m.pr.BaseSHA
}

// fetchRangeDiffCmd fetches the diff for commits[startIdx..endIdx] (inclusive).
func (m Model) fetchRangeDiffCmd(startIdx, endIdx int) tea.Cmd {
	baseSHA := m.commitBaseSHA(startIdx)
	headSHA := m.commits[endIdx].SHA

	if baseSHA == "" {
		return func() tea.Msg {
			return commitDiffLoadedMsg{err: fmt.Errorf("base SHA not available yet, try again")}
		}
	}

	return func() tea.Msg {
		files, err := data.FetchCommitDiff(baseSHA, headSHA)
		return commitDiffLoadedMsg{files: files, err: err}
	}
}

func (m Model) fetchFullDiffCmd() tea.Cmd {
	prNumber := m.pr.Number
	return func() tea.Msg {
		files, err := data.FetchDiffFiles(prNumber)
		return commitDiffLoadedMsg{files: files, err: err}
	}
}

// selectCommitRange sets the commit range and triggers a diff load.
func (m Model) selectCommitRange(startIdx, endIdx int) (Model, tea.Cmd) {
	// Normalize order
	if startIdx > endIdx {
		startIdx, endIdx = endIdx, startIdx
	}

	// Toggle off if selecting the exact same range
	if m.commitStart == startIdx && m.commitEnd == endIdx {
		m.commitStart = -1
		m.commitEnd = -1
		m.commitPickerActive = false
		m.loading = true
		return m, tea.Batch(
			flash.ShowMsg{ID: "diffview", Text: "Loading full PR diff…", Style: flash.StyleSpinner}.Cmd(),
			m.fetchFullDiffCmd(),
		)
	}

	m.commitStart = startIdx
	m.commitEnd = endIdx
	m.commitPickerActive = false
	m.loading = true

	var label string
	if startIdx == endIdx {
		label = fmt.Sprintf("Loading commit %s…", m.commits[startIdx].ShortSHA)
	} else {
		label = fmt.Sprintf("Loading %s..%s…", m.commits[startIdx].ShortSHA, m.commits[endIdx].ShortSHA)
	}
	return m, tea.Batch(
		flash.ShowMsg{ID: "diffview", Text: label, Style: flash.StyleSpinner}.Cmd(),
		m.fetchRangeDiffCmd(startIdx, endIdx),
	)
}

// handleCommitPickerKey handles key events in the commit picker overlay.
func (m Model) handleCommitPickerKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Back), key.Matches(msg, keys.Quit):
		m.commitPickerActive = false
		m.commitPickerAnchor = -1
		return m, nil

	case key.Matches(msg, keys.Up):
		if m.commitPickerCursor > 0 {
			m.commitPickerCursor--
		}
		return m, nil

	case key.Matches(msg, keys.Down):
		if m.commitPickerCursor < len(m.commits)-1 {
			m.commitPickerCursor++
		}
		return m, nil

	case key.Matches(msg, keys.SelectLine): // space — toggle range anchor
		if len(m.commits) == 0 {
			return m, nil
		}
		if m.commitPickerAnchor == m.commitPickerCursor {
			m.commitPickerAnchor = -1 // unset anchor
		} else {
			m.commitPickerAnchor = m.commitPickerCursor
		}
		return m, nil

	case key.Matches(msg, keys.Enter):
		if len(m.commits) == 0 {
			return m, nil
		}
		if m.commitPickerAnchor >= 0 && m.commitPickerAnchor != m.commitPickerCursor {
			// Range: anchor to cursor
			return m.selectCommitRange(m.commitPickerAnchor, m.commitPickerCursor)
		}
		// Single commit
		return m.selectCommitRange(m.commitPickerCursor, m.commitPickerCursor)
	}

	return m, nil
}

// isCommitView returns true when viewing a commit or range diff (not full PR).
func (m Model) isCommitView() bool {
	return m.commitStart >= 0
}

// commitViewLabel returns a display label for the current commit selection.
func (m Model) commitViewLabel() string {
	if !m.isCommitView() {
		return ""
	}
	if m.commitStart == m.commitEnd {
		return fmt.Sprintf("commit %s", m.commits[m.commitStart].ShortSHA)
	}
	return fmt.Sprintf("%s..%s", m.commits[m.commitStart].ShortSHA, m.commits[m.commitEnd].ShortSHA)
}

// renderCommitPicker renders the commit picker overlay.
func (m Model) renderCommitPicker() string {
	if !m.commitPickerActive {
		return ""
	}

	maxW := m.width - 4
	if maxW > 100 {
		maxW = 100
	}
	if maxW < 40 {
		maxW = 40
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.Primary)
	cursorStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.Primary).Reverse(true)
	activeStyle := lipgloss.NewStyle().Foreground(styles.Success)
	anchorStyle := lipgloss.NewStyle().Foreground(styles.Warning)
	rangeStyle := lipgloss.NewStyle().Foreground(styles.Info)
	mutedStyle := lipgloss.NewStyle().Foreground(styles.Muted)

	var lines []string
	lines = append(lines, titleStyle.Render("Select Commit"))
	if m.isCommitView() {
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("  current: %s", m.commitViewLabel())))
	} else {
		lines = append(lines, mutedStyle.Render("  viewing: all changes"))
	}
	lines = append(lines, "")

	if !m.commitsLoaded {
		lines = append(lines, mutedStyle.Render("  Loading commits…"))
	} else if len(m.commits) == 0 {
		lines = append(lines, mutedStyle.Render("  No commits found"))
	} else {
		msgWidth := maxW - 40
		if msgWidth < 20 {
			msgWidth = 20
		}

		// Compute range preview (anchor to cursor)
		anchorMin, anchorMax := -1, -1
		if m.commitPickerAnchor >= 0 {
			anchorMin = m.commitPickerAnchor
			anchorMax = m.commitPickerCursor
			if anchorMin > anchorMax {
				anchorMin, anchorMax = anchorMax, anchorMin
			}
		}

		for i, c := range m.commits {
			cursor := "  "
			if i == m.commitPickerCursor {
				cursor = "> "
			}

			msg := c.Message
			if len(msg) > msgWidth {
				msg = msg[:msgWidth-1] + "…"
			}

			line := fmt.Sprintf("%s%s %s", cursor, c.ShortSHA, msg)

			ago := timeAgo(c.CommittedAt)
			line += mutedStyle.Render(fmt.Sprintf("  %s", ago))

			// Style priority: cursor > anchor > range preview > active selection
			switch {
			case i == m.commitPickerCursor:
				line = cursorStyle.Render(line)
			case i == m.commitPickerAnchor:
				line = anchorStyle.Render(line)
			case anchorMin >= 0 && i >= anchorMin && i <= anchorMax:
				line = rangeStyle.Render(line)
			case m.commitStart >= 0 && i >= m.commitStart && i <= m.commitEnd:
				line = activeStyle.Render(line)
			}

			lines = append(lines, line)
		}
	}

	lines = append(lines, "")
	help := "  enter: select  space: range anchor  esc: close"
	if m.commitPickerAnchor >= 0 {
		help = "  enter: select range  space: clear anchor  esc: close"
	}
	lines = append(lines, mutedStyle.Render(help))

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.Primary).
		Padding(1, 2).
		Width(maxW).
		Render(content)

	return box
}

// timeAgo returns a human-friendly relative time string.
func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		m := int(d.Minutes())
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		return fmt.Sprintf("%dh ago", h)
	default:
		days := int(d.Hours() / 24)
		return fmt.Sprintf("%dd ago", days)
	}
}
