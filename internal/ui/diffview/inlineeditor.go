package diffview

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jethrokuan/pry/internal/review"
	"github.com/jethrokuan/pry/internal/ui/styles"
)

// --- InlineEditor outbound messages ---

// inlineEditorSaveMsg is emitted when the user saves the comment.
type inlineEditorSaveMsg struct {
	body        string
	path        string
	line        int
	startLine   int
	side        string
	mode        commentMode
	editCommentID int // non-zero when editing an existing comment
}

// inlineEditorCancelMsg is emitted when the user cancels the editor.
type inlineEditorCancelMsg struct{}

// inlineEditorOpenEditorMsg is emitted when the user wants to open $EDITOR.
type inlineEditorOpenEditorMsg struct{}

// inlineEditorPasteImageMsg is emitted when the user wants to paste an image.
type inlineEditorPasteImageMsg struct{}

// --- InlineEditor ---

// InlineEditor manages the inline comment textarea with @mention autocomplete.
type InlineEditor struct {
	active         bool
	ta             textarea.Model
	path           string
	line           int
	startLine      int
	side           string
	mode           commentMode
	suggestion     string // original code for suggestion mode
	editCommentID    int    // non-zero when editing an existing comment
	confirmDiscard bool   // true after first esc with unsaved content
	width          int

	// @ mention autocomplete
	mentionActive  bool
	mentionPrefix  string
	mentionAll     []review.MentionableUser // all mentionable users
	mentionMatches []review.MentionableUser // filtered matches for current prefix
	mentionCursor  int                      // selected index in mentionMatches
}

// IsActive returns true if the inline editor is open.
func (e InlineEditor) IsActive() bool { return e.active }

// Open activates the inline editor for a new comment.
func (e *InlineEditor) Open(path string, line, startLine int, side string, mode commentMode, suggestion string, width int) {
	e.active = true
	e.path = path
	e.line = line
	e.startLine = startLine
	e.side = side
	e.mode = mode
	e.suggestion = suggestion
	e.editCommentID = 0
	e.confirmDiscard = false
	e.width = width

	ta := textarea.New()
	ta.Placeholder = "Write your comment..."
	ta.Focus()
	ta.CharLimit = 0
	ta.SetWidth(width - 4)
	ta.SetHeight(5)

	if mode == commentModeSuggestion && suggestion != "" {
		ta.SetValue(suggestion)
	}

	e.ta = ta
}

// OpenForEdit activates the inline editor to edit an existing comment.
func (e *InlineEditor) OpenForEdit(path string, line, startLine int, side string, commentID int, body string, width int) {
	e.active = true
	e.path = path
	e.line = line
	e.startLine = startLine
	e.side = side
	e.mode = commentModeComment
	e.suggestion = ""
	e.editCommentID = commentID
	e.confirmDiscard = false
	e.width = width

	ta := textarea.New()
	ta.Placeholder = "Edit your comment..."
	ta.Focus()
	ta.CharLimit = 0
	ta.SetWidth(width - 4)
	ta.SetHeight(5)
	ta.SetValue(body)

	e.ta = ta
}

// Close deactivates the inline editor and resets state.
func (e *InlineEditor) Close() {
	e.active = false
	e.editCommentID = 0
	e.confirmDiscard = false
	e.mentionActive = false
}

// SetMentionUsers sets the list of mentionable users for autocomplete.
func (e *InlineEditor) SetMentionUsers(users []review.MentionableUser) {
	e.mentionAll = users
}

// SetWidth updates the editor width.
func (e *InlineEditor) SetWidth(w int) {
	e.width = w
	if e.active {
		e.ta.SetWidth(w - 4)
	}
}

// SetValue replaces the textarea content (used after $EDITOR returns).
func (e *InlineEditor) SetValue(s string) {
	if e.active {
		e.ta.SetValue(s)
	}
}

// InsertString inserts text at the current cursor position.
func (e *InlineEditor) InsertString(s string) {
	if e.active {
		e.ta.InsertString(s)
	}
}

// BlinkCmd returns the textarea blink command (needed after Open).
func (e InlineEditor) BlinkCmd() tea.Cmd {
	return textarea.Blink
}

// HandleKey processes a key event while the inline editor is active.
// Returns the updated editor, a tea.Cmd, and an optional outbound message.
func (e InlineEditor) HandleKey(msg tea.KeyPressMsg) (InlineEditor, tea.Cmd, any) {
	// When mention autocomplete is active, intercept navigation keys
	if e.mentionActive {
		if updated, consumed := e.handleMentionKey(msg); consumed {
			return updated, nil, nil
		}
	}

	switch {
	case key.Matches(msg, inlineKeys.Cancel):
		if e.ta.Value() != "" && !e.confirmDiscard {
			e.confirmDiscard = true
			return e, nil, nil
		}
		e.Close()
		return e, nil, inlineEditorCancelMsg{}

	case key.Matches(msg, inlineKeys.Save):
		text := strings.TrimSpace(e.ta.Value())
		if text == "" {
			e.Close()
			return e, nil, inlineEditorCancelMsg{}
		}
		body := text
		if e.mode == commentModeSuggestion {
			body = fmt.Sprintf("```suggestion\n%s\n```", text)
		}
		saveMsg := inlineEditorSaveMsg{
			body:        body,
			path:        e.path,
			line:        e.line,
			startLine:   e.startLine,
			side:        e.side,
			mode:        e.mode,
			editCommentID: e.editCommentID,
		}
		e.Close()
		return e, nil, saveMsg

	case key.Matches(msg, inlineKeys.OpenEditor):
		return e, nil, inlineEditorOpenEditorMsg{}

	case key.Matches(msg, inlineKeys.PasteImage):
		return e, nil, inlineEditorPasteImageMsg{}
	}

	// Any other key resets the discard confirmation
	e.confirmDiscard = false

	// Forward to textarea
	var cmd tea.Cmd
	e.ta, cmd = e.ta.Update(msg)

	// After textarea processes the key, check for @ mention trigger
	e.updateMentionState()

	return e, cmd, nil
}

// --- Mention autocomplete ---

func (e *InlineEditor) updateMentionState() {
	if len(e.mentionAll) == 0 {
		e.mentionActive = false
		return
	}

	prefix, atIdx := mentionTrigger(e.ta)
	if atIdx < 0 {
		e.mentionActive = false
		return
	}

	matches := filterMentionUsers(e.mentionAll, prefix)
	if len(matches) == 0 {
		e.mentionActive = false
		return
	}

	e.mentionActive = true
	e.mentionPrefix = prefix
	e.mentionMatches = matches
	if e.mentionCursor >= len(matches) {
		e.mentionCursor = 0
	}
}

func (e InlineEditor) handleMentionKey(msg tea.KeyPressMsg) (InlineEditor, bool) {
	switch msg.String() {
	case "up":
		if e.mentionCursor > 0 {
			e.mentionCursor--
		} else {
			e.mentionCursor = len(e.mentionMatches) - 1
		}
		return e, true
	case "down":
		if e.mentionCursor < len(e.mentionMatches)-1 {
			e.mentionCursor++
		} else {
			e.mentionCursor = 0
		}
		return e, true
	case "tab", "enter":
		e.completeMention()
		return e, true
	case "esc":
		e.mentionActive = false
		return e, true
	}
	return e, false
}

func (e *InlineEditor) completeMention() {
	if !e.mentionActive || len(e.mentionMatches) == 0 {
		return
	}

	login := e.mentionMatches[e.mentionCursor].Login
	_, atIdx := mentionTrigger(e.ta)
	if atIdx < 0 {
		return
	}

	value := e.ta.Value()

	// Compute absolute cursor offset
	lines := strings.Split(value, "\n")
	curLine := e.ta.Line()
	li := e.ta.LineInfo()
	col := li.CharOffset
	cursorOffset := 0
	for i := 0; i < curLine; i++ {
		cursorOffset += len(lines[i]) + 1
	}
	cursorOffset += col

	// Replace @prefix with @login + space
	newValue := value[:atIdx] + "@" + login + " " + value[cursorOffset:]
	newCursorPos := atIdx + 1 + len(login) + 1

	e.ta.SetValue(newValue)
	e.setCursorToOffset(newCursorPos)
	e.mentionActive = false
}

func (e *InlineEditor) setCursorToOffset(offset int) {
	value := e.ta.Value()
	lines := strings.Split(value, "\n")

	targetLine := 0
	targetCol := offset
	for i, line := range lines {
		if targetCol <= len(line) {
			targetLine = i
			break
		}
		targetCol -= len(line) + 1
		if i == len(lines)-1 {
			targetLine = i
			targetCol = len(line)
		}
	}

	e.ta.CursorStart()
	curLine := e.ta.Line()
	for curLine > 0 {
		e.ta.CursorUp()
		curLine--
	}
	for i := 0; i < targetLine; i++ {
		e.ta.CursorDown()
	}
	e.ta.SetCursorColumn(targetCol)
}

// --- Rendering ---

// View renders the inline comment editor box.
func (e InlineEditor) View() string {
	if !e.active {
		return ""
	}

	modeStr := "Comment"
	if e.mode == commentModeSuggestion {
		modeStr = "Suggestion"
	}

	location := fmt.Sprintf("%s:%d", e.path, e.line)
	if e.startLine > 0 {
		location = fmt.Sprintf("%s:%d-%d", e.path, e.startLine, e.line)
	}

	header := lipgloss.NewStyle().Foreground(styles.Warning).Bold(true).
		Render(fmt.Sprintf(" %s on %s ", modeStr, location))

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.Warning).
		Padding(0, 1).
		Width(e.width - 2)

	helpText := "ctrl+s save  ctrl+e $EDITOR  ctrl+v paste image  esc cancel"
	if e.confirmDiscard {
		helpText = "Press esc again to discard  ctrl+s save"
	}
	help := styles.HelpStyle.Render(helpText)

	content := e.ta.View()
	if e.mentionActive && len(e.mentionMatches) > 0 {
		content += "\n" + e.renderMentionDropdown()
	}

	return header + "\n" + boxStyle.Render(content) + "\n" + help
}

func (e InlineEditor) renderMentionDropdown() string {
	maxVisible := 5
	matches := e.mentionMatches
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
		label := "@" + u.Login
		if u.Name != "" {
			label = fmt.Sprintf("%s — @%s", u.Name, u.Login)
		}
		if i == e.mentionCursor {
			rows = append(rows, selected.Render(label))
		} else {
			rows = append(rows, normal.Render(label))
		}
	}

	if len(e.mentionMatches) > maxVisible {
		more := lipgloss.NewStyle().Foreground(styles.Muted).Padding(0, 1).
			Render(fmt.Sprintf("... and %d more", len(e.mentionMatches)-maxVisible))
		rows = append(rows, more)
	}

	return strings.Join(rows, "\n")
}
