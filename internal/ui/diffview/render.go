package diffview

import (
	"fmt"
	"strings"

	"image/color"

	"charm.land/bubbles/v2/viewport"
	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	"charm.land/lipgloss/v2"

	"github.com/jethrokuan/pry/internal/diff"
	"github.com/jethrokuan/pry/internal/review"
	"github.com/jethrokuan/pry/internal/ui/mdutil"
	"github.com/jethrokuan/pry/internal/ui/styles"
)

// mdCacheKey is the cache key for rendered markdown output.
type mdCacheKey struct {
	body  string
	width int
}

// renderMarkdown renders a markdown string using Glamour with caching.
// Falls back to the raw text on any error. The result is trimmed of
// leading/trailing whitespace. An optional bgColor sets the background
// on all Glamour style elements so it matches the surrounding container.
func (m *Model) renderMarkdown(body string, width int, bgColor ...color.Color) string {
	if width < 10 {
		width = 10
	}

	body = mdutil.ReplaceImages(body)

	key := mdCacheKey{body: body, width: width}
	if cached, ok := m.mdCache[key]; ok {
		return cached
	}

	sc := mdStyleConfig()
	if len(bgColor) > 0 && bgColor[0] != nil {
		applyBackground(&sc, colorToANSIString(bgColor[0]))
	}

	opts := []glamour.TermRendererOption{
		glamour.WithWordWrap(width),
		glamour.WithStyles(sc),
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
func boolPtr(b bool) *bool    { return &b }
func uintPtr(u uint) *uint    { return &u }

// colorToANSIString converts a color.Color to a Glamour-compatible color
// string, preserving ANSI color indices so they match the terminal theme.
func colorToANSIString(c color.Color) string {
	switch v := c.(type) {
	case lipgloss.ANSIColor:
		return fmt.Sprintf("%d", v)
	default:
		r, g, b, _ := v.RGBA()
		return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
	}
}

// applyBackground sets the background color on all Glamour style elements.
func applyBackground(sc *ansi.StyleConfig, bg string) {
	bgp := &bg
	sc.Document.BackgroundColor = bgp
	sc.Document.StylePrimitive.BackgroundColor = bgp
	sc.Text.BackgroundColor = bgp
	sc.Paragraph.StylePrimitive.BackgroundColor = bgp
	sc.Heading.StylePrimitive.BackgroundColor = bgp
	sc.H1.StylePrimitive.BackgroundColor = bgp
	sc.H2.StylePrimitive.BackgroundColor = bgp
	sc.H3.StylePrimitive.BackgroundColor = bgp
	sc.H4.StylePrimitive.BackgroundColor = bgp
	sc.H5.StylePrimitive.BackgroundColor = bgp
	sc.H6.StylePrimitive.BackgroundColor = bgp
	sc.Strikethrough.BackgroundColor = bgp
	sc.Emph.BackgroundColor = bgp
	sc.Strong.BackgroundColor = bgp
	sc.HorizontalRule.BackgroundColor = bgp
	sc.Item.BackgroundColor = bgp
	sc.Enumeration.BackgroundColor = bgp
	sc.Link.BackgroundColor = bgp
	sc.LinkText.BackgroundColor = bgp
	sc.ImageText.BackgroundColor = bgp
	sc.Code.StylePrimitive.BackgroundColor = bgp
	sc.CodeBlock.StyleBlock.StylePrimitive.BackgroundColor = bgp
	sc.BlockQuote.StylePrimitive.BackgroundColor = bgp
}

// mdStyleConfig returns a glamour style that uses ANSI 0-15 colors.
// No 256-color or hex backgrounds — everything adapts to the terminal theme.
func mdStyleConfig() ansi.StyleConfig {
	return ansi.StyleConfig{
		Document: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				BlockPrefix: "\n",
				BlockSuffix: "\n",
			},
			Margin: uintPtr(0),
		},
		Heading: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				BlockSuffix: "\n",
				Color:       stringPtr("4"), // blue
				Bold:        boolPtr(true),
			},
		},
		H1: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "# ",
				Bold:   boolPtr(true),
			},
		},
		H2: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "## ",
			},
		},
		H3: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "### ",
			},
		},
		H4: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "#### ",
			},
		},
		H5: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "##### ",
			},
		},
		H6: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "###### ",
			},
		},
		Strikethrough: ansi.StylePrimitive{
			CrossedOut: boolPtr(true),
		},
		Emph: ansi.StylePrimitive{
			Italic: boolPtr(true),
		},
		Strong: ansi.StylePrimitive{
			Bold: boolPtr(true),
		},
		HorizontalRule: ansi.StylePrimitive{
			Color:  stringPtr("8"), // bright black
			Format: "\n--------\n",
		},
		Item: ansi.StylePrimitive{
			BlockPrefix: "• ",
		},
		Enumeration: ansi.StylePrimitive{
			BlockPrefix: ". ",
		},
		Task: ansi.StyleTask{
			Ticked:   "[✓] ",
			Unticked: "[ ] ",
		},
		Link: ansi.StylePrimitive{
			Color:     stringPtr("6"), // cyan
			Underline: boolPtr(true),
		},
		LinkText: ansi.StylePrimitive{
			Color: stringPtr("4"), // blue
			Bold:  boolPtr(true),
		},
		ImageText: ansi.StylePrimitive{
			Color:  stringPtr("8"), // bright black
			Format: "Image: {{.text}} →",
		},
		Code: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Color: stringPtr("3"), // yellow
			},
		},
		CodeBlock: ansi.StyleCodeBlock{
			StyleBlock: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					Color: stringPtr("8"), // bright black (muted)
				},
				Margin: uintPtr(2),
			},
		},
		BlockQuote: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Color: stringPtr("8"), // bright black (muted)
			},
			Indent:      uintPtr(1),
			IndentToken: stringPtr("│ "),
		},
		Table: ansi.StyleTable{
			CenterSeparator: stringPtr("┼"),
			ColumnSeparator: stringPtr("│"),
			RowSeparator:    stringPtr("─"),
		},
	}
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
	searchQuery := strings.ToLower(m.search.Query())
	cursorBg := styles.BgCursor
	searchBg := styles.BgSearch
	activeHunkBg := styles.BgActiveHunk

	// Determine which hunk is active (contains the cursor)
	activeHunkIdx := -1
	if m.nav.focus == FocusDiff && len(m.nav.diffLines) > 0 && m.nav.cursor.LineIdx < len(m.nav.diffLines) {
		activeHunkIdx = m.nav.diffLines[m.nav.cursor.LineIdx].hunkIdx
	}

	hunkIndex := 0
	for _, hunk := range file.Hunks {
		hk := hunkKey(file.Path, hunkIndex)
		isCollapsed := m.nav.collapsedHunks[hk]

		header := fmt.Sprintf("@@ -%d,%d +%d,%d @@", hunk.OldStart, hunk.OldLines, hunk.NewStart, hunk.NewLines)
		if hunk.Header != "" {
			header += " " + hunk.Header
		}
		if isCollapsed {
			header += fmt.Sprintf("  ▸ %d lines", len(hunk.Lines))
		}
		isActiveHunk := hunkIndex == activeHunkIdx && styles.BgActiveHunk != nil
		hs := hunkStyle
		if isActiveHunk {
			hs = hs.Background(activeHunkBg)
		}
		renderedHeader := hs.Render(header)
		if w := lipgloss.Width(renderedHeader); w < m.nav.diffViewport.Width() {
			renderedHeader += hs.Render(strings.Repeat(" ", m.nav.diffViewport.Width()-w))
		}
		b.WriteString(renderedHeader + "\n")

		if isCollapsed {
			// Render a single selectable placeholder line for the collapsed hunk
			isCurrent := m.nav.focus == FocusDiff && lineIdx == m.nav.cursor.LineIdx
			summary := fmt.Sprintf("  ⋯ %d lines hidden [tab to expand]", len(hunk.Lines))
			if isCurrent {
				foldBg := lipgloss.NewStyle().Background(cursorBg)
				foldText := foldBg.Render(summary)
				w := lipgloss.Width(foldText)
				if w < m.nav.diffViewport.Width() {
					foldText += foldBg.Render(strings.Repeat(" ", m.nav.diffViewport.Width()-w))
				}
				b.WriteString(foldText + "\n")
			} else {
				b.WriteString(lipgloss.NewStyle().Foreground(styles.Muted).Render(summary) + "\n")
			}
			lineIdx++
			b.WriteString("\n")
			hunkIndex++
			continue
		}

		for _, line := range hunk.Lines {
			isCurrent := m.nav.focus == FocusDiff && lineIdx == m.nav.cursor.LineIdx
			isVisualSelected := m.nav.visualMode && m.nav.focus == FocusDiff &&
				lineIdx >= min(m.nav.visualStart, m.nav.visualEnd) &&
				lineIdx <= max(m.nav.visualStart, m.nav.visualEnd)

			highlighted := isCurrent || isVisualSelected

			// Determine line background: cursor > diff-type background > active hunk
			var bg color.Color
			hasBg := false
			if highlighted {
				bg = cursorBg
				hasBg = true
			} else if line.Type == diff.LineAddition {
				bg = styles.BgDiffAdd
				hasBg = true
			} else if line.Type == diff.LineDeletion {
				bg = styles.BgDiffDelete
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
				if searchQuery != "" {
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
				if searchQuery != "" {
					text = highlightMatches(text, searchQuery, s, searchBg)
					lineContent = text
				} else {
					lineContent = s.Render(text)
				}
			default:
				text := " " + line.Content
				if searchQuery != "" {
					s := lipgloss.NewStyle()
					if hasBg {
						s = s.Background(bg)
					}
					lineContent = highlightMatches(text, searchQuery, s, searchBg)
				} else if hasBg {
					lineContent = lipgloss.NewStyle().Background(bg).Render(text)
				} else {
					lineContent = text
				}
			}

			// Comment marker on left edge
			hasComment := m.comments.LineHasComments(file.Path, diffLineInfo{newLine: line.NewNum, oldLine: line.OldNum})
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
				if visWidth < m.nav.diffViewport.Width() {
					fullLine += lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", m.nav.diffViewport.Width()-visWidth))
				}
			}

			b.WriteString(fullLine + "\n")

			// Render comments for this line (with folding and selection)
			commentSelBase := 0
			renderSideComments := func(lineNum int, side string) {
				sel := -1
				if isCurrent && m.nav.cursor.IsComment() {
					sel = m.nav.cursor.CommentIdx - commentSelBase
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

	existing := m.comments.CommentsForLine(path, line, side)
	localPending := m.comments.LocalPendingForLine(path, line, side)

	totalCount := len(existing) + len(localPending)
	if totalCount == 0 {
		return 0
	}

	commentStyle := lipgloss.NewStyle().Foreground(styles.Cyan)
	pendingStyle := lipgloss.NewStyle().Foreground(styles.Warning)
	mutedStyle := lipgloss.NewStyle().Foreground(styles.Muted)

	// Gutter matching diff lines: marker(▎) + line-num-area("     │") + space
	// Content area gets a subtle background to visually group the comment
	commentBg := styles.BgSurface
	cursorBg := styles.BgCursor
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

	contentWidth := m.nav.diffViewport.Width() - 8 // 8 = gutter visual width
	if contentWidth < 20 {
		contentWidth = 20
	}

	// Build all comment lines into a buffer for potential capping
	var allLines []string

	buildComment := func(header, bodyText string, bg color.Color) {
		borderChar := lipgloss.NewStyle().Foreground(styles.Muted).Render("│")
		selectedBorder := lipgloss.NewStyle().Foreground(styles.Warning).Render("│")
		border := borderChar
		if bg == cursorBg {
			border = selectedBorder
		}

		// Header line with border
		allLines = append(allLines, gutterBase+" "+border+" "+header)
		// Body lines — render markdown, prefix each with border
		rendered := m.renderMarkdown(bodyText, contentWidth-4)
		for _, bodyLine := range strings.Split(rendered, "\n") {
			allLines = append(allLines, gutterBase+" "+border+" "+bodyLine)
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
	truncText :=
		lipgloss.NewStyle().Foreground(styles.Warning).Render(fmt.Sprintf("  … %d more lines", hidden)) +
		" " + lipgloss.NewStyle().Foreground(styles.Muted).Render("[enter to view all]")
	truncLine := gutterBase + truncText
	b.WriteString(truncLine + "\n")

	return totalCount
}

// renderCommentPopup builds the bordered comment popup.
func (m Model) renderCommentPopup() string {
	path := ""
	lineNum := 0
	if len(m.files) > 0 && m.nav.cursor.LineIdx < len(m.nav.diffLines) {
		path = m.files[m.nav.fileCursor].Path
		dl := m.nav.diffLines[m.nav.cursor.LineIdx]
		if dl.newLine > 0 {
			lineNum = dl.newLine
		} else {
			lineNum = dl.oldLine
		}
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.Primary)
	title := titleStyle.Render(fmt.Sprintf("  Comments on %s:%d", path, lineNum))

	scrollPct := ""
	if m.comments.popupViewport.TotalLineCount() > m.comments.popupViewport.Height() {
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
		Background(styles.BgOverlay).
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

	vp := viewport.New(viewport.WithWidth(contentW), viewport.WithHeight(vpH))
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

	// Merge status
	mergeLabel := "unknown"
	mergeStyle := lipgloss.NewStyle().Foreground(styles.Muted)
	switch m.pr.MergeState {
	case "CLEAN", "HAS_HOOKS":
		mergeLabel = "ready to merge"
		mergeStyle = lipgloss.NewStyle().Foreground(styles.Success)
	case "BLOCKED":
		mergeLabel = "blocked"
		mergeStyle = lipgloss.NewStyle().Foreground(styles.Danger)
	case "UNSTABLE":
		mergeLabel = "unstable"
		mergeStyle = lipgloss.NewStyle().Foreground(styles.Warning)
	case "DIRTY":
		mergeLabel = "merge conflicts"
		mergeStyle = lipgloss.NewStyle().Foreground(styles.Danger)
	case "DRAFT":
		mergeLabel = "draft"
		mergeStyle = lipgloss.NewStyle().Foreground(styles.Muted)
	}
	b.WriteString(labelStyle.Render("Merge: ") + mergeStyle.Render(mergeLabel) + "\n")

	b.WriteString(separator + "\n\n")

	// Body
	if m.pr.Body == "" {
		b.WriteString(labelStyle.Render("No description provided."))
	} else {
		rendered := m.renderMarkdown(m.pr.Body, width, styles.BgOverlay)
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
			rendered := m.renderMarkdown(c.Body, width, styles.BgOverlay)
			b.WriteString(bodyStyle.Render(rendered) + "\n")
			b.WriteString(innerSep + "\n\n")
		}

		for _, c := range m.comments.forgeComments {
			b.WriteString("📝 " + authorStyle.Render("@"+c.Author))
			if c.Path != "" {
				b.WriteString("  " + labelStyle.Render(fmt.Sprintf("%s:%d", c.Path, c.Line)))
			}
			b.WriteString("\n")
			rendered := m.renderMarkdown(c.Body, width, styles.BgOverlay)
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
	if m.prInfoViewport.TotalLineCount() > m.prInfoViewport.Height() {
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
		Background(styles.BgOverlay).
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

// overlayGeneric places a popup string centered over a base string using Canvas compositing.
func (m Model) overlayGeneric(base, popup string) string {
	popupW := lipgloss.Width(popup)
	popupH := lipgloss.Height(popup)

	x := (m.width - popupW) / 2
	if x < 0 {
		x = 0
	}
	y := (m.height - popupH) / 2
	if y < 0 {
		y = 0
	}

	baseLayer := lipgloss.NewLayer(base)
	popupLayer := lipgloss.NewLayer(popup).X(x).Y(y).Z(1)
	return lipgloss.NewCompositor(baseLayer, popupLayer).Render()
}

// highlightMatches renders text with the base style, but applies a highlight
// background to substrings matching the query (case-insensitive).
func highlightMatches(text, query string, base lipgloss.Style, hlBg color.Color) string {
	if query == "" {
		return base.Render(text)
	}
	lower := strings.ToLower(text)
	qLen := len(query)
	var b strings.Builder
	hlStyle := base.Background(hlBg).Foreground(styles.LabelFg).Bold(true)
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
