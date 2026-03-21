package diffview

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jkuan/pr-review/internal/ui/styles"
)

// mentionTrigger extracts the @prefix being typed at the cursor position.
// Returns the prefix (without @) and the byte offset of the @ character,
// or ("", -1) if no mention is being typed.
func mentionTrigger(ta textarea.Model) (string, int) {
	value := ta.Value()
	if value == "" {
		return "", -1
	}

	// Compute absolute cursor offset: sum of all lines before current + current column
	lines := strings.Split(value, "\n")
	curLine := ta.Line()
	if curLine >= len(lines) {
		return "", -1
	}
	li := ta.LineInfo()
	col := li.CharOffset

	offset := 0
	for i := 0; i < curLine; i++ {
		offset += len(lines[i]) + 1 // +1 for newline
	}
	offset += col

	// Scan backwards from cursor to find @
	text := value[:offset]
	atIdx := -1
	for i := len(text) - 1; i >= 0; i-- {
		ch := text[i]
		if ch == '@' {
			// @ must be at start of line or preceded by whitespace
			if i == 0 || text[i-1] == ' ' || text[i-1] == '\t' || text[i-1] == '\n' {
				atIdx = i
			}
			break
		}
		// Stop if we hit whitespace (no @ mention in progress)
		if ch == ' ' || ch == '\t' || ch == '\n' {
			break
		}
	}

	if atIdx < 0 {
		return "", -1
	}

	prefix := text[atIdx+1:]
	return prefix, atIdx
}

// filterMentionUsers returns users matching the prefix (case-insensitive).
func filterMentionUsers(users []string, prefix string) []string {
	if len(users) == 0 {
		return nil
	}
	prefix = strings.ToLower(prefix)
	var matches []string
	for _, u := range users {
		if strings.HasPrefix(strings.ToLower(u), prefix) {
			matches = append(matches, u)
		}
	}
	return matches
}

// updateMentionState checks the textarea for an @ mention prefix and
// updates the autocomplete state accordingly.
func (m *Model) updateMentionState() {
	if len(m.comments.mentionAll) == 0 {
		m.comments.mentionActive = false
		return
	}

	prefix, atIdx := mentionTrigger(m.comments.inlineTextarea)
	if atIdx < 0 {
		m.comments.mentionActive = false
		return
	}

	matches := filterMentionUsers(m.comments.mentionAll, prefix)
	if len(matches) == 0 {
		m.comments.mentionActive = false
		return
	}

	m.comments.mentionActive = true
	m.comments.mentionPrefix = prefix
	m.comments.mentionMatches = matches
	if m.comments.mentionCursor >= len(matches) {
		m.comments.mentionCursor = 0
	}
}

// completeMention inserts the selected username, replacing the @prefix.
func (m *Model) completeMention() {
	if !m.comments.mentionActive || len(m.comments.mentionMatches) == 0 {
		return
	}

	username := m.comments.mentionMatches[m.comments.mentionCursor]
	_, atIdx := mentionTrigger(m.comments.inlineTextarea)
	if atIdx < 0 {
		return
	}

	value := m.comments.inlineTextarea.Value()

	// Compute absolute cursor offset
	lines := strings.Split(value, "\n")
	curLine := m.comments.inlineTextarea.Line()
	li := m.comments.inlineTextarea.LineInfo()
	col := li.CharOffset
	cursorOffset := 0
	for i := 0; i < curLine; i++ {
		cursorOffset += len(lines[i]) + 1
	}
	cursorOffset += col

	// Replace @prefix with @username + space
	newValue := value[:atIdx] + "@" + username + " " + value[cursorOffset:]

	// Calculate where the cursor should be after insertion
	newCursorPos := atIdx + 1 + len(username) + 1 // @username + space

	m.comments.inlineTextarea.SetValue(newValue)

	// Position cursor after the inserted mention
	// SetValue resets cursor to end, so we need to reposition
	m.setCursorToOffset(newCursorPos)

	m.comments.mentionActive = false
}

// setCursorToOffset positions the textarea cursor at the given byte offset.
func (m *Model) setCursorToOffset(offset int) {
	value := m.comments.inlineTextarea.Value()
	lines := strings.Split(value, "\n")

	targetLine := 0
	targetCol := offset
	for i, line := range lines {
		if targetCol <= len(line) {
			targetLine = i
			break
		}
		targetCol -= len(line) + 1 // +1 for newline
		if i == len(lines)-1 {
			targetLine = i
			targetCol = len(line)
		}
	}

	// Move to start, then navigate to target line
	m.comments.inlineTextarea.CursorStart()
	curLine := m.comments.inlineTextarea.Line()
	// Go to first line first
	for curLine > 0 {
		m.comments.inlineTextarea.CursorUp()
		curLine--
	}
	// Now go to target line
	for i := 0; i < targetLine; i++ {
		m.comments.inlineTextarea.CursorDown()
	}
	// Set column
	m.comments.inlineTextarea.SetCursorColumn(targetCol)
}

// renderMentionDropdown renders the autocomplete dropdown for @ mentions.
func (m Model) renderMentionDropdown() string {
	maxVisible := 5
	matches := m.comments.mentionMatches
	if len(matches) > maxVisible {
		matches = matches[:maxVisible]
	}

	selected := lipgloss.NewStyle().
		Background(styles.Primary).
		Foreground(styles.LabelFg).
		Padding(0, 1)
	normal := lipgloss.NewStyle().
		Padding(0, 1)

	var rows []string
	for i, u := range matches {
		label := fmt.Sprintf("@%s", u)
		if i == m.comments.mentionCursor {
			rows = append(rows, selected.Render(label))
		} else {
			rows = append(rows, normal.Render(label))
		}
	}

	if len(m.comments.mentionMatches) > maxVisible {
		more := lipgloss.NewStyle().Foreground(styles.Muted).Padding(0, 1).
			Render(fmt.Sprintf("... and %d more", len(m.comments.mentionMatches)-maxVisible))
		rows = append(rows, more)
	}

	return strings.Join(rows, "\n")
}

// handleMentionKey handles key presses when the mention autocomplete is active.
// Returns true if the key was consumed.
func (m *Model) handleMentionKey(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	switch msg.String() {
	case "up":
		if m.comments.mentionCursor > 0 {
			m.comments.mentionCursor--
		} else {
			m.comments.mentionCursor = len(m.comments.mentionMatches) - 1
		}
		return *m, nil, true

	case "down":
		if m.comments.mentionCursor < len(m.comments.mentionMatches)-1 {
			m.comments.mentionCursor++
		} else {
			m.comments.mentionCursor = 0
		}
		return *m, nil, true

	case "tab", "enter":
		m.completeMention()
		return *m, nil, true

	case "esc":
		m.comments.mentionActive = false
		return *m, nil, true
	}

	return *m, nil, false
}
