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
	content  string // raw content string (kept for search)
	blocks   []int  // line offsets of each block (description, comments) for n/N nav

	// Issue comments (top-level conversation) — loaded lazily.
	issueComments []review.IssueComment

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

// Open opens the popup with the given content at the given dimensions.
func (p *PRInfoPanel) Open(content string, contentW, vpH int) {
	vp := viewport.New(viewport.WithWidth(contentW), viewport.WithHeight(vpH))
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

// SetContent updates the viewport content (and cached raw content).
func (p *PRInfoPanel) SetContent(content string) {
	yOff := p.viewport.YOffset()
	p.viewport.SetContent(content)
	p.viewport.SetYOffset(yOff)
	p.content = content
}

// HandleKey processes a key event while the PR info popup is active.
// Returns whether the popup should close (so the parent can set prInfoActive = false).
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

	// Delegate scrolling to the viewport
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
	// Restore original (unhighlighted) content
	p.viewport.SetContent(p.content)
}

// computeSearchMatches finds all lines in the viewport content that match the
// search query (case-insensitive) and updates the viewport with highlighted content.
func (p *PRInfoPanel) computeSearchMatches() {
	p.searchLines = nil
	p.searchCursor = 0
	if p.searchQuery == "" {
		return
	}
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

// highlightANSILine highlights all case-insensitive occurrences of query in a
// line that may contain ANSI escape sequences. plain is the ANSI-stripped version
// of line and must correspond character-for-character to the visible output.
func highlightANSILine(styled, plain, query string) string {
	lowerPlain := strings.ToLower(plain)
	qLen := len(query)

	// Find all match positions in the plain text
	var matches [][2]int // [start, end) in plain-text byte offsets
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

	// Walk through styled text, tracking visible character position.
	// When we hit a match boundary, insert highlight start/end markers.
	var b strings.Builder
	visPos := 0 // position in plain text
	matchIdx := 0
	inHighlight := false
	i := 0
	for i < len(styled) {
		// Check if this is an ANSI escape sequence
		if styled[i] == '\x1b' && i+1 < len(styled) && styled[i+1] == '[' {
			// Find end of sequence
			j := i + 2
			for j < len(styled) && styled[j] != 'm' {
				j++
			}
			if j < len(styled) {
				j++ // include 'm'
			}
			b.WriteString(styled[i:j])
			i = j
			continue
		}

		// Visible character — check match boundaries
		if matchIdx < len(matches) && visPos == matches[matchIdx][0] && !inHighlight {
			b.WriteString(hlStart)
			inHighlight = true
		}
		if inHighlight && matchIdx < len(matches) && visPos == matches[matchIdx][1] {
			b.WriteString(hlEnd)
			inHighlight = false
			matchIdx++
			// Check if next match starts at this same position
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

// jumpSearch jumps to the next (dir=1) or previous (dir=-1) search match.
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

// jumpBlock scrolls to the next (dir=1) or previous (dir=-1) content block.
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

// RenderPopup builds the bordered PR info popup.
func (p *PRInfoPanel) RenderPopup(pr *review.PullRequest, totalWidth int) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.Primary)
	title := titleStyle.Render(fmt.Sprintf("  PR #%d: %s", pr.Number, pr.Title))

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
