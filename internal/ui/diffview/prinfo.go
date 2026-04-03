package diffview

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/jethrokuan/pry/internal/review"
	"github.com/jethrokuan/pry/internal/ui/styles"
)

// PRInfoPanel manages the PR info popup state, including scrollable content,
// block navigation, and text search.
type PRInfoPanel struct {
	active   bool
	viewport viewport.Model
	contentW int    // usable content width inside the popup
	content  string // raw content string (kept for search)
	blocks   []int  // line offsets of each block (description, comments) for n/N nav

	// Data — set by the parent before Open.
	pr            *review.PullRequest
	issueComments []review.IssueComment
	renderMD      func(body string, width int) string // markdown renderer

	// Search
	searchActive bool   // true when typing in search input
	searchInput  string // current input while typing
	searchQuery  string // confirmed search query
	searchLines  []int  // line numbers containing matches
	searchCursor int    // index into searchLines for current match
}

// IsActive returns true if the PR info popup is open.
func (p *PRInfoPanel) IsActive() bool { return p.active }

// Close closes the popup and resets search state.
func (p *PRInfoPanel) Close() {
	p.active = false
	p.searchActive = false
	p.searchInput = ""
	p.searchQuery = ""
	p.searchLines = nil
	p.searchCursor = 0
}

// Open builds the popup content and opens the viewport at the given dimensions.
func (p *PRInfoPanel) Open(totalWidth, totalHeight int) {
	popupW := totalWidth - 6
	if popupW > 120 {
		popupW = 120
	}
	popupH := totalHeight - 6
	if popupH < 5 {
		popupH = 5
	}
	p.contentW = popupW - 4 // border(2) + padding(2)
	vpH := popupH - 2       // title + footer

	content := p.buildContent("")
	vp := viewport.New(viewport.WithWidth(p.contentW), viewport.WithHeight(vpH))
	vp.SetContent(content)

	p.active = true
	p.viewport = vp
	p.content = content
	// Reset search on open
	p.searchActive = false
	p.searchInput = ""
	p.searchQuery = ""
	p.searchLines = nil
	p.searchCursor = 0
}

// HandleKey processes a key event while the PR info popup is active.
func (p *PRInfoPanel) HandleKey(msg tea.KeyPressMsg) tea.Cmd {
	if p.searchActive {
		return p.handleSearchInput(msg)
	}

	switch msg.String() {
	case "esc", "ctrl+c":
		if p.searchQuery != "" {
			p.clearSearch()
			return nil
		}
		p.Close()
		return nil
	case "i":
		p.Close()
		return nil
	case "/":
		p.searchActive = true
		p.searchInput = ""
		return nil
	case "n":
		if p.searchQuery != "" {
			p.jumpSearch(1)
			return nil
		}
		p.jumpBlock(1)
		return nil
	case "N":
		if p.searchQuery != "" {
			p.jumpSearch(-1)
			return nil
		}
		p.jumpBlock(-1)
		return nil
	}

	var cmd tea.Cmd
	p.viewport, cmd = p.viewport.Update(msg)
	return cmd
}

func (p *PRInfoPanel) handleSearchInput(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "enter":
		p.searchActive = false
		p.searchQuery = p.searchInput
		if p.searchQuery != "" {
			p.computeSearchMatches()
			if len(p.searchLines) > 0 {
				p.viewport.SetYOffset(p.searchLines[0])
			}
		}
		return nil
	case "esc", "ctrl+c":
		p.searchActive = false
		p.searchInput = ""
		return nil
	case "backspace":
		if len(p.searchInput) > 0 {
			p.searchInput = p.searchInput[:len(p.searchInput)-1]
		}
		return nil
	default:
		if text := msg.Text; text != "" {
			p.searchInput += text
		}
		return nil
	}
}

func (p *PRInfoPanel) clearSearch() {
	p.searchQuery = ""
	p.searchLines = nil
	p.searchCursor = 0
	p.content = p.buildContent("")
	p.viewport.SetContent(p.content)
}

// computeSearchMatches rebuilds content with search-aware borders, finds
// matching lines, and applies text highlighting.
func (p *PRInfoPanel) computeSearchMatches() {
	p.searchLines = nil
	p.searchCursor = 0
	if p.searchQuery == "" {
		return
	}
	p.content = p.buildContent(p.searchQuery)
	query := strings.ToLower(p.searchQuery)
	lines := strings.Split(p.content, "\n")
	highlighted := make([]string, len(lines))
	for i, line := range lines {
		plain := xansi.Strip(line)
		if strings.Contains(strings.ToLower(plain), query) {
			p.searchLines = append(p.searchLines, i)
			highlighted[i] = highlightANSILine(line, plain, query)
		} else {
			highlighted[i] = line
		}
	}
	p.viewport.SetContent(strings.Join(highlighted, "\n"))
}

// --- Content building ---

// buildContent builds the scrollable popup content. When searchQuery is
// non-empty, comment boxes whose body matches get a highlighted border.
func (p *PRInfoPanel) buildContent(searchQuery string) string {
	width := p.contentW
	authorStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.Cyan)
	labelStyle := lipgloss.NewStyle().Foreground(styles.Muted)
	sepStyle := lipgloss.NewStyle().Foreground(styles.Muted)
	separator := sepStyle.Render(strings.Repeat("─", width))

	var b strings.Builder
	lineCount := func() int { return strings.Count(b.String(), "\n") }
	var blocks []int

	// --- Block: Description ---
	blocks = append(blocks, 0)

	b.WriteString(authorStyle.Render("@"+p.pr.Author) + " → " + p.pr.Base + "\n")
	b.WriteString(fmt.Sprintf("+%d/-%d  |  %d files\n", p.pr.Additions, p.pr.Deletions, p.pr.Files))

	if len(p.pr.Labels) > 0 {
		var labels []string
		for _, l := range p.pr.Labels {
			labels = append(labels, styles.LabelStyle.Render(l))
		}
		b.WriteString("Labels: " + strings.Join(labels, " ") + "\n")
	}

	mergeLabel := "unknown"
	mergeStyle := lipgloss.NewStyle().Foreground(styles.Muted)
	switch p.pr.MergeState {
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

	if p.pr.Body == "" {
		b.WriteString(labelStyle.Render("No description provided."))
	} else {
		rendered := p.renderMD(p.pr.Body, width)
		b.WriteString(rendered)
	}

	// --- Blocks: Issue comments ---
	if len(p.issueComments) > 0 {
		b.WriteString("\n\n" + separator + "\n")
		commentHeader := lipgloss.NewStyle().Bold(true).Foreground(styles.Primary)
		b.WriteString(commentHeader.Render(fmt.Sprintf("Comments (%d)", len(p.issueComments))) + "\n\n")

		lowerQuery := strings.ToLower(searchQuery)
		commentBoxWidth := width - 4 // border(2) + padding(2)

		for _, c := range p.issueComments {
			blocks = append(blocks, lineCount())

			header := fmt.Sprintf("💬 %s:", styles.CommentAuthor.Render("@"+c.Author))
			rendered := p.renderMD(c.Body, commentBoxWidth)
			inner := header + "\n" + rendered

			borderColor := styles.Cyan
			if searchQuery != "" {
				plain := xansi.Strip(rendered)
				if strings.Contains(strings.ToLower(plain), lowerQuery) {
					borderColor = styles.Warning
				}
			}

			box := lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(borderColor).
				Padding(0, 1).
				Width(width).
				Render(inner)
			b.WriteString(box + "\n")
		}
	}

	p.blocks = blocks
	return strings.TrimRight(b.String(), "\n")
}

// --- Search text highlighting ---

// highlightANSILine highlights all case-insensitive occurrences of query in a
// line that may contain ANSI escape sequences.
func highlightANSILine(styled, plain, query string) string {
	lowerPlain := strings.ToLower(plain)
	qLen := len(query)

	var matches [][2]int
	pos := 0
	for {
		idx := strings.Index(lowerPlain[pos:], query)
		if idx < 0 {
			break
		}
		start := pos + idx
		matches = append(matches, [2]int{start, start + qLen})
		pos = start + qLen
	}
	if len(matches) == 0 {
		return styled
	}

	hlStart := "\x1b[1;30;43m" // bold, black fg, yellow bg
	hlEnd := "\x1b[0m"

	var b strings.Builder
	visPos := 0
	matchIdx := 0
	inHighlight := false
	i := 0
	for i < len(styled) {
		if styled[i] == '\x1b' && i+1 < len(styled) && styled[i+1] == '[' {
			j := i + 2
			for j < len(styled) && styled[j] != 'm' {
				j++
			}
			if j < len(styled) {
				j++
			}
			b.WriteString(styled[i:j])
			i = j
			continue
		}

		if matchIdx < len(matches) && visPos == matches[matchIdx][0] && !inHighlight {
			b.WriteString(hlStart)
			inHighlight = true
		}
		if inHighlight && matchIdx < len(matches) && visPos == matches[matchIdx][1] {
			b.WriteString(hlEnd)
			inHighlight = false
			matchIdx++
			if matchIdx < len(matches) && visPos == matches[matchIdx][0] {
				b.WriteString(hlStart)
				inHighlight = true
			}
		}

		b.WriteByte(styled[i])
		visPos++
		i++
	}
	if inHighlight {
		b.WriteString(hlEnd)
	}
	return b.String()
}

// --- Navigation ---

func (p *PRInfoPanel) jumpSearch(dir int) {
	if len(p.searchLines) == 0 {
		return
	}
	if dir > 0 {
		p.searchCursor++
		if p.searchCursor >= len(p.searchLines) {
			p.searchCursor = 0
		}
	} else {
		p.searchCursor--
		if p.searchCursor < 0 {
			p.searchCursor = len(p.searchLines) - 1
		}
	}
	p.viewport.SetYOffset(p.searchLines[p.searchCursor])
}

func (p *PRInfoPanel) jumpBlock(dir int) {
	if len(p.blocks) == 0 {
		return
	}
	current := p.viewport.YOffset()
	if dir > 0 {
		for _, offset := range p.blocks {
			if offset > current {
				p.viewport.SetYOffset(offset)
				return
			}
		}
	} else {
		for i := len(p.blocks) - 1; i >= 0; i-- {
			if p.blocks[i] < current {
				p.viewport.SetYOffset(p.blocks[i])
				return
			}
		}
	}
}

// --- Rendering ---

// RenderPopup builds the bordered PR info popup.
func (p *PRInfoPanel) RenderPopup(totalWidth int) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.Primary)
	title := titleStyle.Render(fmt.Sprintf("  PR #%d: %s", p.pr.Number, p.pr.Title))

	scrollPct := ""
	if p.viewport.TotalLineCount() > p.viewport.Height() {
		pct := int(p.viewport.ScrollPercent() * 100)
		scrollPct = lipgloss.NewStyle().Foreground(styles.Muted).Render(fmt.Sprintf(" (%d%%)", pct))
	}

	var footer string
	if p.searchActive {
		prompt := lipgloss.NewStyle().Bold(true).Render("/")
		footer = prompt + p.searchInput + "█"
	} else if p.searchQuery != "" {
		matchInfo := ""
		if len(p.searchLines) > 0 {
			matchInfo = fmt.Sprintf(" [%d/%d]", p.searchCursor+1, len(p.searchLines))
		} else {
			matchInfo = " [no matches]"
		}
		searchStatus := lipgloss.NewStyle().Foreground(styles.Cyan).Render(
			fmt.Sprintf("  /%s%s", p.searchQuery, matchInfo))
		footer = styles.HelpStyle.Render("  n/N next/prev match  esc clear  i close") + searchStatus + scrollPct
	} else {
		footer = styles.HelpStyle.Render("  j/k scroll  n/N next/prev block  / search  i/esc close") + scrollPct
	}

	content := title + "\n" + p.viewport.View() + "\n" + footer

	popupW := totalWidth - 6
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
