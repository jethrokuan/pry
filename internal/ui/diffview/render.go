package diffview

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/glamour"
	glamourstyles "github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"

	"github.com/jkuan/pr-review/internal/diff"
	"github.com/jkuan/pr-review/internal/review"
	"github.com/jkuan/pr-review/internal/ui/styles"
)

// mdCacheKey is the cache key for rendered markdown output.
type mdCacheKey struct {
	body    string
	width   int
	bgColor string
}

// renderMarkdown renders a markdown string using Glamour with caching.
// Falls back to the raw text on any error. The result is trimmed of
// leading/trailing whitespace. If bgColor is non-empty, it is set as
// the document background color in the Glamour style.
func (m *Model) renderMarkdown(body string, width int, bgColor string) string {
	if width < 10 {
		width = 10
	}

	key := mdCacheKey{body: body, width: width, bgColor: bgColor}
	if cached, ok := m.mdCache[key]; ok {
		return cached
	}

	opts := []glamour.TermRendererOption{
		glamour.WithWordWrap(width),
	}

	if bgColor != "" {
		sc := glamourstyles.DarkStyleConfig
		sc.Document.BackgroundColor = stringPtr(bgColor)
		// Propagate background to text and paragraph so resets don't clear it
		sc.Document.StylePrimitive.BackgroundColor = stringPtr(bgColor)
		sc.Text.BackgroundColor = stringPtr(bgColor)
		sc.Paragraph.StylePrimitive.BackgroundColor = stringPtr(bgColor)
		opts = append(opts, glamour.WithStyles(sc))
	} else {
		opts = append(opts, glamour.WithAutoStyle())
	}

	renderer, err := glamour.NewTermRenderer(opts...)
	if err != nil {
		return body
	}
	rendered, err := renderer.Render(body)
	if err != nil {
		return body
	}
	result := strings.TrimRight(rendered, "\n")
	m.mdCache[key] = result
	return result
}

func stringPtr(s string) *string { return &s }

// glamourBgForComment returns the hex background color string for use with
// Glamour rendering, derived from a lipgloss.Color.
func glamourBgForComment(bg lipgloss.Color) string {
	return string(bg)
}


func (m *Model) renderDiffWithCursor(file *diff.DiffFile) string {
	if file.IsBinary {
		return "Binary file changed"
	}

	var b strings.Builder
	lineIdx := 0

	hunkStyle := styles.HunkHeader
	commentMarker := lipgloss.NewStyle().Foreground(styles.Warning).Render("▎")
	noMarker := " "
	searchQuery := strings.ToLower(m.search.query)
	cursorBg := lipgloss.Color(styles.Current.BgCursor)
	searchBg := lipgloss.Color(styles.Current.BgSearch)
	activeHunkBg := lipgloss.Color(styles.Current.BgActiveHunk)

	// Determine which hunk is active (contains the cursor)
	activeHunkIdx := -1
	if m.nav.focus == FocusDiff && len(m.nav.diffLines) > 0 && m.nav.diffCursor < len(m.nav.diffLines) {
		activeHunkIdx = m.nav.diffLines[m.nav.diffCursor].hunkIdx
	}

	hunkIndex := 0
	for _, hunk := range file.Hunks {
		header := fmt.Sprintf("@@ -%d,%d +%d,%d @@", hunk.OldStart, hunk.OldLines, hunk.NewStart, hunk.NewLines)
		if hunk.Header != "" {
			header += " " + hunk.Header
		}
		isActiveHunk := hunkIndex == activeHunkIdx && styles.Current.BgActiveHunk != ""
		hs := hunkStyle
		if isActiveHunk {
			hs = hs.Background(activeHunkBg)
		}
		renderedHeader := hs.Render(header)
		if w := lipgloss.Width(renderedHeader); w < m.nav.diffViewport.Width {
			renderedHeader += hs.Render(strings.Repeat(" ", m.nav.diffViewport.Width-w))
		}
		b.WriteString(renderedHeader + "\n")

		for _, line := range hunk.Lines {
			isCurrent := m.nav.focus == FocusDiff && lineIdx == m.nav.diffCursor
			isVisualSelected := m.nav.visualMode && m.nav.focus == FocusDiff &&
				lineIdx >= min(m.nav.visualStart, m.nav.visualEnd) &&
				lineIdx <= max(m.nav.visualStart, m.nav.visualEnd)

			highlighted := isCurrent || isVisualSelected

			// Determine line background: cursor > diff-type background > active hunk
			var bg lipgloss.Color
			hasBg := false
			if highlighted {
				bg = cursorBg
				hasBg = true
			} else if line.Type == diff.LineAddition {
				bg = lipgloss.Color(styles.Current.BgDiffAdd)
				hasBg = true
			} else if line.Type == diff.LineDeletion {
				bg = lipgloss.Color(styles.Current.BgDiffDelete)
				hasBg = true
			} else if isActiveHunk {
				bg = activeHunkBg
				hasBg = true
			}

			// Build line number segment with background baked in
			lineNumStyle := lipgloss.NewStyle().Foreground(styles.Muted)
			if hasBg {
				lineNumStyle = lineNumStyle.Background(bg)
			}

			var num int
			switch line.Type {
			case diff.LineDeletion:
				num = line.OldNum
			default:
				num = line.NewNum
			}
			numStr := "    "
			if num > 0 {
				numStr = fmt.Sprintf("%4d", num)
			}
			nums := lineNumStyle.Render(fmt.Sprintf("%s │", numStr))

			// Build content segment with background baked in
			var lineContent string
			switch line.Type {
			case diff.LineAddition:
				s := styles.Addition
				if hasBg {
					s = s.Background(bg)
				}
				text := "+" + line.Content
				if searchQuery != "" && !hasBg {
					text = highlightMatches(text, searchQuery, s, searchBg)
					lineContent = text
				} else {
					lineContent = s.Render(text)
				}
			case diff.LineDeletion:
				s := styles.Deletion
				if hasBg {
					s = s.Background(bg)
				}
				text := "-" + line.Content
				if searchQuery != "" && !hasBg {
					text = highlightMatches(text, searchQuery, s, searchBg)
					lineContent = text
				} else {
					lineContent = s.Render(text)
				}
			default:
				text := " " + line.Content
				if hasBg {
					lineContent = lipgloss.NewStyle().Background(bg).Render(text)
				} else if searchQuery != "" {
					lineContent = highlightMatches(text, searchQuery, lipgloss.NewStyle(), searchBg)
				} else {
					lineContent = text
				}
			}

			// Comment marker on left edge
			hasComment := m.lineHasComments(file.Path, diffLineInfo{newLine: line.NewNum, oldLine: line.OldNum})
			marker := noMarker
			if hasComment {
				marker = commentMarker
			}

			// Join with background-aware separator
			sep := " "
			if hasBg {
				sep = lipgloss.NewStyle().Background(bg).Render(" ")
			}
			fullLine := marker + nums + sep + lineContent

			// Pad to full viewport width for lines with background
			if hasBg {
				visWidth := lipgloss.Width(fullLine)
				if visWidth < m.nav.diffViewport.Width {
					fullLine += lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", m.nav.diffViewport.Width-visWidth))
				}
			}

			b.WriteString(fullLine + "\n")

			// Render comments for this line (with folding and selection)
			commentSelBase := 0
			renderSideComments := func(lineNum int, side string) {
				sel := -1
				if isCurrent && m.comments.cursor >= 0 {
					sel = m.comments.cursor - commentSelBase
				}
				n := m.renderLineComments(&b, file.Path, lineNum, side, sel)
				commentSelBase += n
			}
			if line.NewNum > 0 {
				renderSideComments(line.NewNum, "RIGHT")
			}
			if line.OldNum > 0 {
				renderSideComments(line.OldNum, "LEFT")
			}

			lineIdx++
		}
		b.WriteString("\n")
		hunkIndex++
	}

	return b.String()
}

// renderLineComments renders comments for a specific line/side.
// selectedIdx is the 0-based index of the comment to highlight within this call's comments (-1 for none).
// Returns the number of individual comments rendered (0 if folded or no comments).
func (m *Model) renderLineComments(b *strings.Builder, path string, line int, side string, selectedIdx int) int {
	ck := commentKey(path, line)
	expanded := m.comments.expanded[ck]

	existing := m.commentsForLine(path, line, side)
	localPending := m.localPendingForLine(path, line, side)

	totalCount := len(existing) + len(localPending)
	if totalCount == 0 {
		return 0
	}

	commentStyle := lipgloss.NewStyle().Foreground(styles.Cyan)
	pendingStyle := lipgloss.NewStyle().Foreground(styles.Warning)
	mutedStyle := lipgloss.NewStyle().Foreground(styles.Muted)

	// Gutter matching diff lines: marker(▎) + line-num-area("     │") + space
	// Content area gets a subtle background to visually group the comment
	commentBg := lipgloss.Color(styles.Current.BgSurface)
	cursorBg := lipgloss.Color(styles.Current.BgCursor)
	gutterPipe := lipgloss.NewStyle().Foreground(styles.Muted).Render("     │")
	bar := lipgloss.NewStyle().Foreground(styles.Warning).Render("▎")
	gutterBase := bar + gutterPipe

	if !expanded {
		foldPrefix := gutterBase + " "
		parts := make([]string, 0, 2)
		if len(existing) > 0 {
			parts = append(parts, commentStyle.Render(fmt.Sprintf("💬 %d comment(s)", len(existing))))
		}
		if len(localPending) > 0 {
			parts = append(parts, pendingStyle.Render(fmt.Sprintf("📝 %d pending", len(localPending))))
		}
		b.WriteString(foldPrefix + strings.Join(parts, "  ") + "  " +
			mutedStyle.Render("[tab to expand]") + "\n")
		return 0
	}

	contentWidth := m.nav.diffViewport.Width - 8 // 8 = gutter visual width
	if contentWidth < 20 {
		contentWidth = 20
	}

	// Build all comment lines into a buffer for potential capping
	var allLines []string

	buildComment := func(header, bodyText string, bg lipgloss.Color) {
		bgS := lipgloss.NewStyle().Background(bg)
		hdrS := lipgloss.NewStyle().Background(bg)

		padLine := func(content string) string {
			w := lipgloss.Width(content)
			if w < contentWidth {
				content += bgS.Render(strings.Repeat(" ", contentWidth-w))
			}
			return content
		}

		// Header line with background
		hdr := hdrS.Render(" " + header)
		allLines = append(allLines, gutterBase+padLine(hdr))
		// Body lines — render markdown with comment bg baked in
		rendered := m.renderMarkdown(bodyText, contentWidth-2, glamourBgForComment(bg))
		for _, bodyLine := range strings.Split(rendered, "\n") {
			content := bgS.Render(" ") + bodyLine
			allLines = append(allLines, gutterBase+padLine(content))
		}
		// Blank separator
		allLines = append(allLines, gutterBase)
	}

	idx := 0
	for _, c := range existing {
		bg := commentBg
		if idx == selectedIdx {
			bg = cursorBg
		}
		label := "💬"
		if c.IsPending {
			label = "📝 (draft)"
		}
		header := fmt.Sprintf("%s %s:", label,
			styles.CommentAuthor.Render("@"+c.Author))
		buildComment(header, c.Body, bg)
		idx++
	}
	for _, c := range localPending {
		bg := commentBg
		if idx == selectedIdx {
			bg = cursorBg
		}
		syncIndicator := ""
		switch c.SyncStatus {
		case review.SyncInFlight:
			syncIndicator = " ..."
		case review.SyncComplete:
			syncIndicator = " ✓"
		case review.SyncFailed:
			syncIndicator = " ✗"
		}
		header := fmt.Sprintf("📝 %s%s:",
			pendingStyle.Render("(pending)"), syncIndicator)
		buildComment(header, c.Body, bg)
		idx++
	}

	// Check if capping is needed
	maxH := m.maxCommentBlockHeight()
	if len(allLines) <= maxH {
		for _, l := range allLines {
			b.WriteString(l + "\n")
		}
		return totalCount
	}

	// Truncate: show first (maxH-1) lines + a "truncated" hint
	for i := 0; i < maxH-1 && i < len(allLines); i++ {
		b.WriteString(allLines[i] + "\n")
	}
	hidden := len(allLines) - (maxH - 1)
	truncText := " " +
		lipgloss.NewStyle().Foreground(styles.Warning).Background(commentBg).Render(fmt.Sprintf("… %d more lines", hidden)) +
		" " + lipgloss.NewStyle().Foreground(styles.Muted).Background(commentBg).Render("[enter to view all]")
	w := lipgloss.Width(truncText)
	if w < contentWidth {
		truncText += lipgloss.NewStyle().Background(commentBg).Render(strings.Repeat(" ", contentWidth-w))
	}
	truncLine := gutterBase + truncText
	b.WriteString(truncLine + "\n")

	return totalCount
}

// renderCommentBox renders the inline comment editor box.
func (m Model) renderCommentBox() string {
	modeStr := "Comment"
	if m.comments.inlineMode == commentModeSuggestion {
		modeStr = "Suggestion"
	}

	location := fmt.Sprintf("%s:%d", m.comments.inlinePath, m.comments.inlineLine)
	if m.comments.inlineStartLine > 0 {
		location = fmt.Sprintf("%s:%d-%d", m.comments.inlinePath, m.comments.inlineStartLine, m.comments.inlineLine)
	}

	header := lipgloss.NewStyle().Foreground(styles.Warning).Bold(true).
		Render(fmt.Sprintf(" %s on %s ", modeStr, location))

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.Warning).
		Padding(0, 1).
		Width(m.inlineTextareaWidth() - 2)

	helpText := "ctrl+s save  ctrl+e $EDITOR  ctrl+t toggle mode  esc cancel"
	if m.comments.confirmDiscard {
		helpText = "Press esc again to discard  ctrl+s save"
	}
	help := styles.HelpStyle.Render(helpText)

	return header + "\n" + boxStyle.Render(m.comments.inlineTextarea.View()) + "\n" + help
}

// renderCommentPopup builds the bordered comment popup.
func (m Model) renderCommentPopup() string {
	path := ""
	lineNum := 0
	if len(m.files) > 0 && m.nav.diffCursor < len(m.nav.diffLines) {
		path = m.files[m.nav.fileCursor].Path
		dl := m.nav.diffLines[m.nav.diffCursor]
		if dl.newLine > 0 {
			lineNum = dl.newLine
		} else {
			lineNum = dl.oldLine
		}
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.Primary)
	title := titleStyle.Render(fmt.Sprintf("  Comments on %s:%d", path, lineNum))

	scrollPct := ""
	if m.comments.popupViewport.TotalLineCount() > m.comments.popupViewport.Height {
		pct := int(m.comments.popupViewport.ScrollPercent() * 100)
		scrollPct = lipgloss.NewStyle().Foreground(styles.Muted).Render(fmt.Sprintf(" (%d%%)", pct))
	}

	help := styles.HelpStyle.Render("  j/k scroll  esc close") + scrollPct

	content := title + "\n" + m.comments.popupViewport.View() + "\n" + help

	popupW := m.width - 6
	if popupW > 120 {
		popupW = 120
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.Primary).
		Background(lipgloss.Color(styles.Current.BgOverlay)).
		Padding(0, 1).
		Width(popupW)

	return boxStyle.Render(content)
}

// --- PR Info Popup ---

// openPRInfoPopup opens the PR details popup with description and metadata.
func (m *Model) openPRInfoPopup() {
	popupW := m.width - 6
	if popupW > 120 {
		popupW = 120
	}
	popupH := m.height - 6
	if popupH < 5 {
		popupH = 5
	}
	contentW := popupW - 4 // border(2) + padding(2)
	vpH := popupH - 2     // title + footer

	content := m.buildPRInfoContent(contentW)

	vp := viewport.New(contentW, vpH)
	vp.SetContent(content)

	m.prInfoActive = true
	m.prInfoViewport = vp
}

// buildPRInfoContent builds the PR info popup content showing metadata and description.
func (m *Model) buildPRInfoContent(width int) string {
	authorStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.Cyan)
	labelStyle := lipgloss.NewStyle().Foreground(styles.Muted)
	sepStyle := lipgloss.NewStyle().Foreground(styles.Muted)
	separator := sepStyle.Render(strings.Repeat("─", width))

	var b strings.Builder

	// Metadata
	b.WriteString(authorStyle.Render("@"+m.pr.Author) + " → " + m.pr.Base + "\n")
	b.WriteString(fmt.Sprintf("+%d/-%d  |  %d files\n", m.pr.Additions, m.pr.Deletions, m.pr.Files))

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
	b.WriteString(labelStyle.Render("Review: ") + reviewStyle.Render(reviewStatus) + "\n")

	b.WriteString(separator + "\n\n")

	// Body
	if m.pr.Body == "" {
		b.WriteString(labelStyle.Render("No description provided."))
	} else {
		rendered := m.renderMarkdown(m.pr.Body, width, styles.Current.BgOverlay)
		b.WriteString(rendered)
	}

	// Existing PR comments section
	if len(m.comments.existing) > 0 || len(m.comments.forgeComments) > 0 {
		b.WriteString("\n\n" + separator + "\n")
		commentHeader := lipgloss.NewStyle().Bold(true).Foreground(styles.Primary)
		total := len(m.comments.existing) + len(m.comments.forgeComments)
		b.WriteString(commentHeader.Render(fmt.Sprintf("Review Comments (%d)", total)) + "\n\n")

		bodyStyle := lipgloss.NewStyle().Width(width)
		innerSep := sepStyle.Render(strings.Repeat("─", width/2))

		for _, c := range m.comments.existing {
			icon := "💬"
			if c.IsPending {
				icon = "📝"
			}
			b.WriteString(icon + " " + authorStyle.Render("@"+c.Author))
			if c.Path != "" {
				b.WriteString("  " + labelStyle.Render(fmt.Sprintf("%s:%d", c.Path, c.Line)))
			}
			b.WriteString("\n")
			rendered := m.renderMarkdown(c.Body, width, styles.Current.BgOverlay)
			b.WriteString(bodyStyle.Render(rendered) + "\n")
			b.WriteString(innerSep + "\n\n")
		}

		for _, c := range m.comments.forgeComments {
			b.WriteString("📝 " + authorStyle.Render("@"+c.Author))
			if c.Path != "" {
				b.WriteString("  " + labelStyle.Render(fmt.Sprintf("%s:%d", c.Path, c.Line)))
			}
			b.WriteString("\n")
			rendered := m.renderMarkdown(c.Body, width, styles.Current.BgOverlay)
			b.WriteString(bodyStyle.Render(rendered) + "\n")
			b.WriteString(innerSep + "\n\n")
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

// renderPRInfoPopup builds the bordered PR info popup.
func (m Model) renderPRInfoPopup() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.Primary)
	title := titleStyle.Render(fmt.Sprintf("  PR #%d: %s", m.pr.Number, m.pr.Title))

	scrollPct := ""
	if m.prInfoViewport.TotalLineCount() > m.prInfoViewport.Height {
		pct := int(m.prInfoViewport.ScrollPercent() * 100)
		scrollPct = lipgloss.NewStyle().Foreground(styles.Muted).Render(fmt.Sprintf(" (%d%%)", pct))
	}

	help := styles.HelpStyle.Render("  j/k scroll  i/esc close") + scrollPct

	content := title + "\n" + m.prInfoViewport.View() + "\n" + help

	popupW := m.width - 6
	if popupW > 120 {
		popupW = 120
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.Primary).
		Background(lipgloss.Color(styles.Current.BgOverlay)).
		Padding(0, 1).
		Width(popupW)

	return boxStyle.Render(content)
}

// overlayPRInfoPopup renders the PR info popup centered over the base content.
func (m Model) overlayPRInfoPopup(base string) string {
	popup := m.renderPRInfoPopup()
	return m.overlayGeneric(base, popup)
}

// overlayCommentPopup renders the comment popup centered over the base content.
func (m Model) overlayCommentPopup(base string) string {
	popup := m.renderCommentPopup()
	return m.overlayGeneric(base, popup)
}

// overlayGeneric places a popup string centered over a base string.
func (m Model) overlayGeneric(base, popup string) string {
	popupLines := strings.Split(popup, "\n")
	baseLines := strings.Split(base, "\n")

	popupWidth := 0
	for _, l := range popupLines {
		if w := lipgloss.Width(l); w > popupWidth {
			popupWidth = w
		}
	}
	popupHeight := len(popupLines)

	startRow := (m.height - popupHeight) / 2
	startCol := (m.width - popupWidth) / 2
	if startRow < 0 {
		startRow = 0
	}
	if startCol < 0 {
		startCol = 0
	}

	for len(baseLines) < m.height {
		baseLines = append(baseLines, "")
	}

	for i, popupLine := range popupLines {
		row := startRow + i
		if row >= len(baseLines) {
			break
		}
		baseLine := baseLines[row]
		baseWidth := lipgloss.Width(baseLine)

		left := ""
		if startCol > 0 {
			if baseWidth >= startCol {
				left = strings.Repeat(" ", startCol)
			} else {
				left = baseLine + strings.Repeat(" ", startCol-baseWidth)
			}
		}
		baseLines[row] = left + popupLine
	}

	return strings.Join(baseLines[:m.height], "\n")
}

// highlightMatches renders text with the base style, but applies a highlight
// background to substrings matching the query (case-insensitive).
func highlightMatches(text, query string, base lipgloss.Style, hlBg lipgloss.Color) string {
	if query == "" {
		return base.Render(text)
	}
	lower := strings.ToLower(text)
	qLen := len(query)
	var b strings.Builder
	hlStyle := base.Background(hlBg).Foreground(lipgloss.Color(styles.Current.LabelFg)).Bold(true)
	pos := 0
	for {
		idx := strings.Index(lower[pos:], query)
		if idx < 0 {
			break
		}
		idx += pos
		if idx > pos {
			b.WriteString(base.Render(text[pos:idx]))
		}
		b.WriteString(hlStyle.Render(text[idx : idx+qLen]))
		pos = idx + qLen
	}
	if pos < len(text) {
		b.WriteString(base.Render(text[pos:]))
	}
	return b.String()
}
