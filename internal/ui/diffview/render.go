package diffview

import (
	"fmt"
	"strings"

	"image/color"

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
	currentY := 0

	// Initialize render offsets
	m.offsets = RenderOffsets{
		DiffLineY:          make([]int, len(m.nav.diffLines)),
		CommentBlockHeight: make([]int, len(m.nav.diffLines)),
	}

	hunkStyle := styles.HunkHeader
	commentMarker := lipgloss.NewStyle().Foreground(styles.Warning).Render("▎")
	noMarker := " "
	searchQuery := strings.ToLower(m.search.Query())
	cursorBg := styles.BgCursor
	searchBg := styles.BgSearch
	hunkBg := styles.BgHunk
	activeHunkBg := styles.BgSelected

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
		isActiveHunk := hunkIndex == activeHunkIdx && styles.BgSelected != nil
		hs := hunkStyle
		if isActiveHunk {
			hs = hs.Background(activeHunkBg)
		}
		renderedHeader := hs.Render(header)
		if w := lipgloss.Width(renderedHeader); w < m.nav.diffViewport.Width() {
			renderedHeader += hs.Render(strings.Repeat(" ", m.nav.diffViewport.Width()-w))
		}
		b.WriteString(renderedHeader + "\n")
		currentY++

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
			m.offsets.DiffLineY[lineIdx] = currentY
			currentY++
			lineIdx++
			b.WriteString("\n")
			currentY++
			hunkIndex++
			continue
		}

		for _, line := range hunk.Lines {
			isCurrent := m.nav.focus == FocusDiff && lineIdx == m.nav.cursor.LineIdx
			isVisualSelected := m.nav.visualMode && m.nav.focus == FocusDiff &&
				lineIdx >= min(m.nav.visualStart.LineIdx, m.nav.visualEnd.LineIdx) &&
				lineIdx <= max(m.nav.visualStart.LineIdx, m.nav.visualEnd.LineIdx)

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
			} else if hunkBg != nil {
				bg = hunkBg
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

			m.offsets.DiffLineY[lineIdx] = currentY
			b.WriteString(fullLine + "\n")
			currentY++

			// Render comments for this line (with folding and selection)
			commentStartY := currentY
			commentSelBase := 0
			renderSideComments := func(lineNum int, side string) {
				sel := -1
				if isCurrent && m.nav.cursor.IsComment() {
					sel = m.flatCommentIndex() - commentSelBase
				}
				n, selInfo, renderedLines := m.renderLineComments(&b, file.Path, lineNum, side, sel)
				if sel >= 0 && sel < n && selInfo.height > 0 {
					m.offsets.SelectedCommentY = currentY + selInfo.offset
					m.offsets.SelectedCommentHeight = selInfo.height
				}
				currentY += renderedLines
				commentSelBase += n
			}
			if line.NewNum > 0 {
				renderSideComments(line.NewNum, "RIGHT")
			}
			if line.OldNum > 0 {
				renderSideComments(line.OldNum, "LEFT")
			}

			// Render inline editor below cursor line's comments (skip for replies — rendered inside thread)
			if m.editor.IsActive() && !m.editor.IsReply() && lineIdx == m.nav.cursor.LineIdx {
				editorView := m.editor.View()
				if editorView != "" {
					b.WriteString(editorView + "\n")
					currentY += strings.Count(editorView, "\n") + 1
				}
			}

			m.offsets.CommentBlockHeight[lineIdx] = currentY - commentStartY
			lineIdx++
		}
		b.WriteString("\n")
		currentY++
		hunkIndex++
	}

	return b.String()
}

// commentSelInfo holds the position of the selected comment within the rendered output.
type commentSelInfo struct {
	offset int // lines from start of this block to the selected comment box
	height int // height of the selected comment box in lines
}

// renderLineComments renders comments for a specific line/side.
// selectedIdx is the 0-based index of the comment to highlight within this call's comments (-1 for none).
// Returns the number of individual comments rendered (0 if folded or no comments),
// selection info if a comment was selected, and the total rendered line count written.
func (m *Model) renderLineComments(b *strings.Builder, path string, line int, side string, selectedIdx int) (int, commentSelInfo, int) {
	ck := commentKey(path, line)
	expanded := m.comments.expanded[ck]

	threads := m.comments.ThreadsForLine(path, line, side)
	if len(threads) == 0 {
		return 0, commentSelInfo{}, 0
	}

	// Collect all comments flat (for selection indexing)
	var allComments []review.Comment
	for _, t := range threads {
		allComments = append(allComments, t.Comments...)
	}
	totalCount := len(allComments)

	commentStyle := lipgloss.NewStyle().Foreground(styles.Cyan)
	pendingStyle := lipgloss.NewStyle().Foreground(styles.Warning)
	mutedStyle := lipgloss.NewStyle().Foreground(styles.Muted)

	boxWidth := m.nav.diffViewport.Width() - 4
	if boxWidth < 20 {
		boxWidth = 20
	}

	if !expanded {
		submitted := 0
		pending := 0
		resolved := 0
		for _, t := range threads {
			if t.IsResolved {
				resolved++
			}
			for _, c := range t.Comments {
				if c.IsPending {
					pending++
				} else {
					submitted++
				}
			}
		}
		parts := make([]string, 0, 3)
		if submitted > 0 {
			parts = append(parts, commentStyle.Render(fmt.Sprintf("💬 %d comment(s)", submitted)))
		}
		if pending > 0 {
			parts = append(parts, pendingStyle.Render(fmt.Sprintf("📝 %d pending", pending)))
		}
		if resolved > 0 {
			parts = append(parts, mutedStyle.Render("✓ resolved"))
		}
		inner := strings.Join(parts, "  ") + "  " +
			mutedStyle.Render("[tab to expand]")
		box := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(styles.Muted).
			Padding(0, 1).
			Width(boxWidth).
			Render(inner)
		b.WriteString(box + "\n")
		foldedLines := strings.Count(box, "\n") + 1
		return 0, commentSelInfo{}, foldedLines
	}

	// Per-comment body budget: each comment gets its own height cap and
	// its own "… N more lines" hint when truncated.
	maxBodyLines := m.maxCommentBlockHeight()
	commentBoxWidth := boxWidth - 4 // inner box width inside outer box
	commentContentWidth := commentBoxWidth - 4 // account for border + padding
	if commentContentWidth < 20 {
		commentContentWidth = 20
	}

	// Build each comment as its own bordered box, grouped by thread.
	// Keep a flat list of all comment boxes for position tracking.
	type commentBoxInfo struct {
		box    string
		height int // lines in rendered box
	}
	var allBoxes []commentBoxInfo
	var threadBoxes []string
	commentIdx := 0
	// threadCommentCounts[i] = number of comments in thread i
	var threadCommentCounts []int
	for _, t := range threads {
		var commentBoxes []string
		for _, c := range t.Comments {
			isSelected := commentIdx == selectedIdx

			label := "💬"
			if c.IsPending {
				label = "📝 (draft)"
			}

			header := fmt.Sprintf("%s %s:", label,
				styles.CommentAuthor.Render("@"+c.Author))

			// Body
			rendered := m.renderMarkdown(c.Body, commentContentWidth)
			bodyLines := strings.Split(rendered, "\n")

			var body string
			if len(bodyLines) <= maxBodyLines {
				body = rendered
			} else {
				body = strings.Join(bodyLines[:maxBodyLines], "\n") + "\n" +
					lipgloss.NewStyle().Foreground(styles.Warning).Render(fmt.Sprintf("… %d more lines", len(bodyLines)-maxBodyLines)) +
					" " + lipgloss.NewStyle().Foreground(styles.Muted).Render("[") +
					lipgloss.NewStyle().Foreground(styles.Info).Bold(true).Render(keys.ViewComment.Help().Key) +
					lipgloss.NewStyle().Foreground(styles.Muted).Render(" to expand]")
			}

			inner := header + "\n" + body

			borderColor := styles.Muted
			if c.IsPending {
				borderColor = styles.Warning
			}
			if isSelected {
				borderColor = styles.Primary
			}

			box := lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(borderColor).
				Padding(0, 1).
				Width(commentBoxWidth).
				Render(inner)
			commentBoxes = append(commentBoxes, box)
			allBoxes = append(allBoxes, commentBoxInfo{box: box, height: strings.Count(box, "\n") + 1})
			commentIdx++
		}

		threadCommentCounts = append(threadCommentCounts, len(t.Comments))

		// Thread status header
		statusParts := []string{
			mutedStyle.Render(fmt.Sprintf("%d comment(s)", len(t.Comments))),
		}
		if t.IsResolved {
			statusParts = append(statusParts, lipgloss.NewStyle().Foreground(styles.Success).Render("✓ resolved"))
		} else {
			statusParts = append(statusParts, mutedStyle.Render("○ open"))
		}

		// Determine thread border color
		threadBorderColor := styles.Cyan
		if t.IsResolved {
			threadBorderColor = styles.Muted
		}
		for _, c := range t.Comments {
			if c.IsPending {
				threadBorderColor = styles.Warning
				break
			}
		}

		threadHeader := lipgloss.NewStyle().Foreground(threadBorderColor).
			Render(strings.Join(statusParts, "  "))
		boxContent := threadHeader + "\n" + strings.Join(commentBoxes, "\n")

		// If the inline editor is replying to this thread, render it inside the thread box
		if m.isEditorReplyingToThread(t) {
			editorView := m.editor.View()
			if editorView != "" {
				boxContent += "\n" + editorView
			}
		}

		box := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(threadBorderColor).
			Padding(0, 1).
			Width(boxWidth).
			Render(boxContent)
		threadBoxes = append(threadBoxes, box)
	}

	// Write thread boxes and compute selected comment position.
	var sel commentSelInfo
	// Write thread boxes and compute total rendered lines + selected comment offset.
	totalRenderedLines := 0
	lineOffset := 0
	flatIdx := 0
	for ti, tb := range threadBoxes {
		b.WriteString(tb + "\n")
		tbLines := strings.Count(tb, "\n") + 1
		totalRenderedLines += tbLines

		// Track selected comment position within this thread
		if selectedIdx >= 0 && sel.height == 0 {
			innerOffset := 2 // thread box top border + status header
			for ci := 0; ci < threadCommentCounts[ti]; ci++ {
				if flatIdx == selectedIdx {
					sel.offset = lineOffset + innerOffset
					sel.height = allBoxes[flatIdx].height
				}
				innerOffset += allBoxes[flatIdx].height
				flatIdx++
			}
		} else {
			flatIdx += threadCommentCounts[ti]
		}
		lineOffset += tbLines
	}

	return totalCount, sel, totalRenderedLines
}

// renderCommentPopup builds the bordered comment popup.
func (m Model) renderCommentPopup() string {
	c := m.selectedComment()
	author := ""
	if c != nil {
		author = "@" + c.Author
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.Primary)
	title := titleStyle.Render(fmt.Sprintf("  Comment by %s", author))

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
	m.prInfo.Open(content, contentW, vpH)
}

// buildPRInfoContent builds the PR info popup content showing metadata,
// description, issue comments, and review comments. It also records
// block line offsets in m.prInfo.blocks for n/N navigation.
func (m *Model) buildPRInfoContent(width int) string {
	authorStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.Cyan)
	labelStyle := lipgloss.NewStyle().Foreground(styles.Muted)
	sepStyle := lipgloss.NewStyle().Foreground(styles.Muted)
	separator := sepStyle.Render(strings.Repeat("─", width))

	var b strings.Builder
	lineCount := func() int { return strings.Count(b.String(), "\n") }
	var blocks []int

	// --- Block: Description ---
	blocks = append(blocks, 0)

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
		rendered := m.renderMarkdown(m.pr.Body, width)
		b.WriteString(rendered)
	}

	// --- Blocks: Issue comments (top-level conversation) ---
	if len(m.prInfo.issueComments) > 0 {
		b.WriteString("\n\n" + separator + "\n")
		commentHeader := lipgloss.NewStyle().Bold(true).Foreground(styles.Primary)
		b.WriteString(commentHeader.Render(fmt.Sprintf("Comments (%d)", len(m.prInfo.issueComments))) + "\n\n")

		innerSep := sepStyle.Render(strings.Repeat("─", width/2))
		for _, c := range m.prInfo.issueComments {
			blocks = append(blocks, lineCount())
			b.WriteString("💬 " + authorStyle.Render("@"+c.Author) + "\n")
			rendered := m.renderMarkdown(c.Body, width)
			b.WriteString(rendered + "\n")
			b.WriteString(innerSep + "\n\n")
		}
	}

	m.prInfo.blocks = blocks
	return strings.TrimRight(b.String(), "\n")
}

// renderPRInfoPopup builds the bordered PR info popup.
func (m Model) renderPRInfoPopup() string {
	return m.prInfo.RenderPopup(m.pr, m.width)
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

// overlayAutocompleteDropdown composites the @mention autocomplete dropdown
// over the base view when the inline editor is active and showing suggestions.
func (m Model) overlayAutocompleteDropdown(base string) string {
	dropdown := m.editor.DropdownView()
	if dropdown == "" {
		return base
	}

	// Y: header lines + cursor position within viewport + 1 (below cursor line)
	// + comments height + editor offset to textarea cursor
	cursor := m.nav.cursor.LineIdx
	rendered := 0
	commentH := 0
	if cursor < len(m.offsets.DiffLineY) {
		rendered = m.offsets.DiffLineY[cursor]
		commentH = m.offsets.CommentBlockHeight[cursor] - m.editor.Height()
	}
	cursorScreenY := rendered - m.nav.diffViewport.YOffset()
	// editor border top(1) + header(1) + cursor line within textarea + 1 (below cursor)
	editorCursorOffset := 2 + m.editor.CursorLine() + 1
	y := editorHeaderLines + cursorScreenY + 1 + commentH + editorCursorOffset

	// X: offset by tree panel width if tree is visible, plus editor padding
	x := 2 + m.treePanelWidth()

	baseLayer := lipgloss.NewLayer(base)
	drop := lipgloss.NewLayer(dropdown).X(x).Y(y).Z(2)
	return lipgloss.NewCompositor(baseLayer, drop).Render()
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
