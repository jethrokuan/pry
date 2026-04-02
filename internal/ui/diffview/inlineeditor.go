package diffview

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jethrokuan/pry/internal/review"
	"github.com/jethrokuan/pry/internal/ui/components/autocomplete"
	"github.com/jethrokuan/pry/internal/ui/styles"
)

// --- InlineEditor outbound messages ---

// inlineEditorSaveMsg is emitted when the user saves the comment.
type inlineEditorSaveMsg struct {
	body          string
	path          string
	line          int
	startLine     int
	side          string
	mode          commentMode
	editCommentID int    // non-zero when editing an existing comment
	replyToNodeID string // non-empty when replying to an existing thread
}

// inlineEditorCancelMsg is emitted when the user cancels the editor.
type inlineEditorCancelMsg struct{}

// inlineEditorOpenEditorMsg is emitted when the user wants to open $EDITOR.
type inlineEditorOpenEditorMsg struct{}

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
	replyToNodeID    string // non-empty when replying to an existing thread
	confirmDiscard bool   // true after first esc with unsaved content
	width          int

	// @ mention autocomplete
	mentionAC autocomplete.Model
}

// IsActive returns true if the inline editor is open.
func (e InlineEditor) IsActive() bool { return e.active }

func newEditorTextarea(width int) textarea.Model {
	ta := textarea.New()
	ta.Placeholder = ""
	ta.ShowLineNumbers = false
	ta.Focus()
	ta.CharLimit = 0
	ta.SetWidth(width - 8)
	ta.SetHeight(5)

	// Clear textarea backgrounds so it inherits from the terminal.
	s := ta.Styles()
	s.Focused.Base = s.Focused.Base.UnsetBackground()
	s.Focused.Text = s.Focused.Text.UnsetBackground()
	s.Focused.CursorLine = s.Focused.CursorLine.UnsetBackground()
	s.Focused.EndOfBuffer = s.Focused.EndOfBuffer.UnsetBackground()
	s.Focused.Prompt = s.Focused.Prompt.UnsetBackground()
	ta.SetStyles(s)

	return ta
}

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

	ta := newEditorTextarea(width)

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

	ta := newEditorTextarea(width)
	ta.SetValue(body)

	e.ta = ta
}

// OpenForReply activates the inline editor to reply to an existing thread.
func (e *InlineEditor) OpenForReply(path string, line, startLine int, side string, commentNodeID string, width int) {
	e.active = true
	e.path = path
	e.line = line
	e.startLine = startLine
	e.side = side
	e.mode = commentModeComment
	e.suggestion = ""
	e.editCommentID = 0
	e.replyToNodeID = commentNodeID
	e.confirmDiscard = false
	e.width = width

	ta := newEditorTextarea(width)

	e.ta = ta
}

// Close deactivates the inline editor and resets state.
func (e *InlineEditor) Close() {
	e.active = false
	e.editCommentID = 0
	e.replyToNodeID = ""
	e.confirmDiscard = false
	e.mentionAC.Hide()
}

// SetMentionUsers sets the list of mentionable users for autocomplete.
func (e *InlineEditor) SetMentionUsers(users []review.MentionableUser) {
	e.mentionAC = autocomplete.New()
	suggestions := make([]autocomplete.Suggestion, len(users))
	for i, u := range users {
		s := autocomplete.Suggestion{Value: u.Login}
		if u.Name != "" {
			s.Label = u.Name + " — @" + u.Login
		} else {
			s.Label = "@" + u.Login
		}
		suggestions[i] = s
	}
	e.mentionAC.SetSuggestions(suggestions)
}

// SetWidth updates the editor width.
func (e *InlineEditor) SetWidth(w int) {
	e.width = w
	if e.active {
		e.ta.SetWidth(w - 8)
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

// Height returns the rendered height of the editor in lines, or 0 if inactive.
func (e InlineEditor) Height() int {
	if !e.active {
		return 0
	}
	return lipgloss.Height(e.View())
}

// HandleKey processes a key event while the inline editor is active.
// Returns the updated editor, a tea.Cmd, and an optional outbound message.
func (e InlineEditor) HandleKey(msg tea.KeyPressMsg) (InlineEditor, tea.Cmd, any) {
	// When mention autocomplete is active, intercept navigation keys
	if consumed, selected := e.mentionAC.HandleKey(msg.String()); consumed {
		if selected {
			e.completeMention()
		}
		return e, nil, nil
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
			body:          body,
			path:          e.path,
			line:          e.line,
			startLine:     e.startLine,
			side:          e.side,
			mode:          e.mode,
			editCommentID: e.editCommentID,
			replyToNodeID: e.replyToNodeID,
		}
		e.Close()
		return e, nil, saveMsg

	case key.Matches(msg, inlineKeys.OpenEditor):
		return e, nil, inlineEditorOpenEditorMsg{}

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
	prefix, atIdx := mentionTrigger(e.ta)
	if atIdx < 0 {
		e.mentionAC.Hide()
		return
	}

	e.mentionAC.Show(prefix)
}

func (e *InlineEditor) completeMention() {
	if !e.mentionAC.IsActive() {
		return
	}

	login := e.mentionAC.Selected().Value
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
	e.mentionAC.Hide()
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
	if e.replyToNodeID != "" {
		modeStr = "Reply"
	} else if e.mode == commentModeSuggestion {
		modeStr = "Suggestion"
	}

	location := fmt.Sprintf("%s:%d", e.path, e.line)
	if e.startLine > 0 {
		location = fmt.Sprintf("%s:%d-%d", e.path, e.startLine, e.line)
	}

	header := lipgloss.NewStyle().Foreground(styles.Primary).Bold(true).
		Render(fmt.Sprintf(" %s on %s ", modeStr, location))

	helpText := "ctrl+s save  ctrl+e $EDITOR  esc cancel"
	if e.confirmDiscard {
		helpText = "Press esc again to discard  ctrl+s save"
	}
	help := styles.HelpStyle.Render(helpText)

	inner := header + "\n" + e.ta.View() + "\n" + help

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.Primary).
		Padding(0, 1).
		Width(e.width - 4).
		Render(inner)
}

// IsReply returns true if the editor is in reply mode.
func (e InlineEditor) IsReply() bool {
	return e.active && e.replyToNodeID != ""
}

// ReplyToNodeID returns the node ID being replied to, or "" if not in reply mode.
func (e InlineEditor) ReplyToNodeID() string {
	return e.replyToNodeID
}

// DropdownView returns the autocomplete dropdown view, or "" if inactive.
func (e InlineEditor) DropdownView() string {
	return e.mentionAC.View()
}

// CursorLine returns the textarea cursor's line index (0-based).
func (e InlineEditor) CursorLine() int {
	return e.ta.Line()
}
