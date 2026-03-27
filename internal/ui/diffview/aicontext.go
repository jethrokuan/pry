package diffview

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"

	"github.com/jethrokuan/pry/internal/diff"
	"github.com/jethrokuan/pry/internal/review"
	"github.com/jethrokuan/pry/internal/ui/styles"
)

// aiContextState manages @-reference autocomplete in the AI input.
type aiContextState struct {
	active  bool
	prefix  string
	matches []aiContextEntry
	cursor  int
}

// aiContextEntry is a single autocomplete completion option.
type aiContextEntry struct {
	Label string // display text in dropdown
	Ref   string // text to insert (e.g. "@hunk", "@src/worker.go")
	Kind  string // kind for resolution
}

// buildAIContextEntries builds all available @ completions from the current model state.
func (m *Model) buildAIContextEntries() []aiContextEntry {
	var entries []aiContextEntry

	// @hunk — current hunk at cursor
	if len(m.files) > 0 && m.nav.cursor.FileIdx < len(m.files) {
		if m.nav.cursor.LineIdx < len(m.nav.diffLines) {
			dl := m.nav.diffLines[m.nav.cursor.LineIdx]
			file := m.files[m.nav.cursor.FileIdx]
			if dl.hunkIdx < len(file.Hunks) {
				hunk := file.Hunks[dl.hunkIdx]
				label := fmt.Sprintf("hunk — %s @@ +%d,%d", file.Path, hunk.NewStart, hunk.NewLines)
				entries = append(entries, aiContextEntry{
					Label: label,
					Ref:   "@hunk",
					Kind:  "hunk",
				})
			}
		}
	}

	// @file — current file
	if len(m.files) > 0 && m.nav.cursor.FileIdx < len(m.files) {
		file := m.files[m.nav.cursor.FileIdx]
		entries = append(entries, aiContextEntry{
			Label: fmt.Sprintf("file — %s", file.Path),
			Ref:   "@file",
			Kind:  "file",
		})
	}

	// @selection — visual selection (only when active)
	if m.nav.visualMode && len(m.files) > 0 {
		file := m.files[m.nav.cursor.FileIdx]
		startIdx := min(m.nav.visualStart.LineIdx, m.nav.visualEnd.LineIdx)
		endIdx := max(m.nav.visualStart.LineIdx, m.nav.visualEnd.LineIdx)
		entries = append(entries, aiContextEntry{
			Label: fmt.Sprintf("selection — %s lines %d-%d", file.Path, startIdx+1, endIdx+1),
			Ref:   "@selection",
			Kind:  "selection",
		})
	}

	// @<filename> — all PR files
	for _, f := range m.files {
		entries = append(entries, aiContextEntry{
			Label: f.Path,
			Ref:   "@" + f.Path,
			Kind:  "file",
		})
	}

	// @thread entries — review threads
	for _, t := range m.comments.threads {
		if len(t.Comments) == 0 {
			continue
		}
		first := t.Comments[0]
		preview := first.Body
		if len(preview) > 40 {
			preview = preview[:40] + "…"
		}
		label := fmt.Sprintf("thread — %s:%d @%s: %s", t.Path, t.Line, first.Author, preview)
		ref := fmt.Sprintf("@thread:%s:%d", t.Path, t.Line)
		entries = append(entries, aiContextEntry{
			Label: label,
			Ref:   ref,
			Kind:  "thread",
		})
	}

	return entries
}

// filterAIContextEntries filters entries matching the given prefix (case-insensitive).
func filterAIContextEntries(entries []aiContextEntry, prefix string) []aiContextEntry {
	if prefix == "" {
		return entries
	}
	prefix = strings.ToLower(prefix)
	var matches []aiContextEntry
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Label), prefix) ||
			strings.Contains(strings.ToLower(e.Ref), prefix) {
			matches = append(matches, e)
		}
	}
	return matches
}

// textInputAtTrigger extracts the @prefix being typed at the cursor position
// in a single-line textinput. Returns (prefix, atIdx) or ("", -1).
func textInputAtTrigger(ti textinput.Model) (string, int) {
	value := ti.Value()
	pos := ti.Position()
	if pos > len(value) {
		pos = len(value)
	}
	text := value[:pos]

	for i := len(text) - 1; i >= 0; i-- {
		ch := text[i]
		if ch == '@' {
			if i == 0 || text[i-1] == ' ' || text[i-1] == '\t' {
				return text[i+1:], i
			}
			return "", -1
		}
		if ch == ' ' || ch == '\t' {
			return "", -1
		}
	}
	return "", -1
}

// updateAIContextState checks the textinput for @ trigger and updates autocomplete state.
func (m *Model) updateAIContextState() {
	prefix, atIdx := textInputAtTrigger(m.aiPanel.input)
	if atIdx < 0 {
		m.aiPanel.contextState.active = false
		return
	}

	allEntries := m.buildAIContextEntries()
	matches := filterAIContextEntries(allEntries, prefix)
	if len(matches) == 0 {
		m.aiPanel.contextState.active = false
		return
	}

	m.aiPanel.contextState.active = true
	m.aiPanel.contextState.prefix = prefix
	m.aiPanel.contextState.matches = matches
	if m.aiPanel.contextState.cursor >= len(matches) {
		m.aiPanel.contextState.cursor = 0
	}
}

// handleAIContextKey handles navigation keys when the @ autocomplete is active.
// Returns true if the key was consumed.
func (m *Model) handleAIContextKey(keyStr string) bool {
	cs := &m.aiPanel.contextState
	if !cs.active {
		return false
	}

	switch keyStr {
	case "up":
		if cs.cursor > 0 {
			cs.cursor--
		} else {
			cs.cursor = len(cs.matches) - 1
		}
		return true
	case "down":
		if cs.cursor < len(cs.matches)-1 {
			cs.cursor++
		} else {
			cs.cursor = 0
		}
		return true
	case "tab", "enter":
		m.completeAIContext()
		return true
	case "esc":
		cs.active = false
		return true
	}
	return false
}

// completeAIContext resolves the selected @ reference and inlines its content into the input.
func (m *Model) completeAIContext() {
	cs := &m.aiPanel.contextState
	if !cs.active || len(cs.matches) == 0 {
		return
	}

	entry := cs.matches[cs.cursor]
	_, atIdx := textInputAtTrigger(m.aiPanel.input)
	if atIdx < 0 {
		return
	}

	content := m.resolveContextEntry(entry)
	value := m.aiPanel.input.Value()
	pos := m.aiPanel.input.Position()
	if pos > len(value) {
		pos = len(value)
	}

	// Replace @prefix with the resolved content + space
	newValue := value[:atIdx] + content + " " + value[pos:]
	newPos := atIdx + len(content) + 1

	m.aiPanel.input.SetValue(newValue)
	m.aiPanel.input.SetCursor(newPos)
	cs.active = false
}

// resolveContextEntry resolves a single context entry to its inline content.
func (m *Model) resolveContextEntry(entry aiContextEntry) string {
	switch {
	case entry.Kind == "hunk" && entry.Ref == "@hunk":
		// Emit @filepath:startLine-endLine for Claude Code to resolve
		if len(m.files) > 0 && m.nav.cursor.FileIdx < len(m.files) {
			file := m.files[m.nav.cursor.FileIdx]
			if m.nav.cursor.LineIdx < len(m.nav.diffLines) {
				dl := m.nav.diffLines[m.nav.cursor.LineIdx]
				if dl.hunkIdx < len(file.Hunks) {
					hunk := file.Hunks[dl.hunkIdx]
					return fmt.Sprintf("@%s:%d-%d", file.Path, hunk.NewStart, hunk.NewStart+hunk.NewLines-1)
				}
			}
		}
	case entry.Kind == "file" && entry.Ref == "@file":
		// Substitute with @filepath — Claude Code resolves file refs natively
		if len(m.files) > 0 && m.nav.cursor.FileIdx < len(m.files) {
			return "@" + m.files[m.nav.cursor.FileIdx].Path
		}
	case entry.Kind == "selection" && entry.Ref == "@selection":
		// Emit @filepath:startLine-endLine for Claude Code to resolve
		if m.nav.visualMode && len(m.files) > 0 {
			file := m.files[m.nav.cursor.FileIdx]
			startIdx := min(m.nav.visualStart.LineIdx, m.nav.visualEnd.LineIdx)
			endIdx := max(m.nav.visualStart.LineIdx, m.nav.visualEnd.LineIdx)
			startLine := m.resolveLineNumber(startIdx)
			endLine := m.resolveLineNumber(endIdx)
			if startLine > 0 && endLine > 0 {
				return fmt.Sprintf("@%s:%d-%d", file.Path, startLine, endLine)
			}
			return "@" + file.Path
		}
	case entry.Kind == "file" && strings.HasPrefix(entry.Ref, "@"):
		// @<filepath> — keep as token for Claude Code to resolve
		return entry.Ref
	case entry.Kind == "thread":
		// Keep thread refs as tokens — they're too long to inline.
		// Resolved at submit time.
		return entry.Ref
	}
	return entry.Ref // fallback: insert the raw ref
}


// renderAIContextDropdown renders the @ autocomplete dropdown.
func (m *Model) renderAIContextDropdown() string {
	cs := &m.aiPanel.contextState
	if !cs.active || len(cs.matches) == 0 {
		return ""
	}

	maxVisible := 8
	matches := cs.matches
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
	for i, e := range matches {
		label := e.Label
		if i == cs.cursor {
			rows = append(rows, selected.Render(label))
		} else {
			rows = append(rows, normal.Render(label))
		}
	}

	if len(cs.matches) > maxVisible {
		more := lipgloss.NewStyle().Foreground(styles.Muted).Padding(0, 1).
			Render(fmt.Sprintf("… and %d more", len(cs.matches)-maxVisible))
		rows = append(rows, more)
	}

	return strings.Join(rows, "\n")
}


// formatHunkContent formats a hunk as diff text for the prompt.
func formatHunkContent(path string, hunk *diff.Hunk) string {
	var b strings.Builder
	fmt.Fprintf(&b, "File: %s\n", path)
	fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@", hunk.OldStart, hunk.OldLines, hunk.NewStart, hunk.NewLines)
	if hunk.Header != "" {
		fmt.Fprintf(&b, " %s", hunk.Header)
	}
	b.WriteString("\n")
	for _, l := range hunk.Lines {
		switch l.Type {
		case diff.LineAddition:
			fmt.Fprintf(&b, "+%s\n", l.Content)
		case diff.LineDeletion:
			fmt.Fprintf(&b, "-%s\n", l.Content)
		default:
			fmt.Fprintf(&b, " %s\n", l.Content)
		}
	}
	return b.String()
}

// formatFileContext formats all hunks of a file as diff text.
func formatFileContext(file diff.DiffFile) string {
	var b strings.Builder
	fmt.Fprintf(&b, "File: %s\n", file.Path)
	for _, hunk := range file.Hunks {
		fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@", hunk.OldStart, hunk.OldLines, hunk.NewStart, hunk.NewLines)
		if hunk.Header != "" {
			fmt.Fprintf(&b, " %s", hunk.Header)
		}
		b.WriteString("\n")
		for _, l := range hunk.Lines {
			switch l.Type {
			case diff.LineAddition:
				fmt.Fprintf(&b, "+%s\n", l.Content)
			case diff.LineDeletion:
				fmt.Fprintf(&b, "-%s\n", l.Content)
			default:
				fmt.Fprintf(&b, " %s\n", l.Content)
			}
		}
	}
	return b.String()
}

// formatSelectionContext formats the visual selection as diff text.
func (m *Model) formatSelectionContext(file diff.DiffFile) string {
	startIdx := min(m.nav.visualStart.LineIdx, m.nav.visualEnd.LineIdx)
	endIdx := max(m.nav.visualStart.LineIdx, m.nav.visualEnd.LineIdx)

	var b strings.Builder
	fmt.Fprintf(&b, "File: %s (selected lines)\n", file.Path)
	for i := startIdx; i <= endIdx && i < len(m.nav.diffLines); i++ {
		dl := m.nav.diffLines[i]
		switch dl.lineType {
		case diff.LineAddition:
			fmt.Fprintf(&b, "+%s\n", dl.content)
		case diff.LineDeletion:
			fmt.Fprintf(&b, "-%s\n", dl.content)
		default:
			fmt.Fprintf(&b, " %s\n", dl.content)
		}
	}
	return b.String()
}

// resolveThreadRefs expands any @thread:<path>:<line> tokens in the text.
// Other @ references are already inlined by autocomplete.
func (m *Model) resolveThreadRefs(text string) string {
	for _, t := range m.comments.threads {
		ref := fmt.Sprintf("@thread:%s:%d", t.Path, t.Line)
		if strings.Contains(text, ref) {
			text = strings.Replace(text, ref, formatThreadContext(&t), 1)
		}
	}
	return text
}

// formatThreadContext formats a review thread as text.
func formatThreadContext(t *review.Thread) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Thread on %s:%d (%s):\n", t.Path, t.Line, t.Side)
	for _, c := range t.Comments {
		fmt.Fprintf(&b, "@%s: %s\n\n", c.Author, c.Body)
	}
	return b.String()
}
