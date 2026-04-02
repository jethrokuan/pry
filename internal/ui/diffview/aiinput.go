package diffview

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/jethrokuan/pry/internal/ai"
	"github.com/jethrokuan/pry/internal/git"
	"github.com/jethrokuan/pry/internal/jj"
	"github.com/jethrokuan/pry/internal/ui/components/flash"
)

// toggleAIPanel toggles AI panel visibility. Work continues in background when hidden.
func (m Model) toggleAIPanel() (Model, tea.Cmd) {
	if m.aiPanel.IsOpen() {
		m.aiPanel.Hide()
		return m, nil
	}

	panelW := m.width - 6
	if panelW > 120 {
		panelW = 120
	}
	panelH := m.height - 6
	if panelH < 10 {
		panelH = 10
	}

	cmd := m.aiPanel.Open(panelW, panelH)
	return m, cmd
}

// overlayAIPanel renders the AI panel centered over the base content.
func (m Model) overlayAIPanel(base, dropdown string) string {
	popup := m.aiPanel.View(dropdown)
	return m.overlayGeneric(base, popup)
}

// handleAIInputKey handles keyboard input when the AI panel prompt is focused.
func (m Model) handleAIInputKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	// When @ autocomplete is active, intercept navigation keys
	if m.aiPanel.contextState.active {
		if m.handleAIContextKey(msg.String()) {
			return m, nil
		}
	}

	switch {
	case key.Matches(msg, keys.Back):
		if m.aiPanel.contextState.active {
			m.aiPanel.contextState.active = false
			return m, nil
		}
		// Hide panel — work continues in background
		m.aiPanel.Hide()
		return m, nil

	case msg.String() == "enter":
		question := strings.TrimSpace(m.aiPanel.input.Value())
		if question == "" {
			return m, nil
		}
		if question == "/clear" {
			m.aiPanel.Clear()
			return m, flash.ShowMsg{ID: "diffview", Text: "Conversation cleared", Expires: 1500 * time.Millisecond}.Cmd()
		}
		// Resolve thread refs, build task, submit
		resolved := m.resolveThreadRefs(question)
		task := m.buildTask(resolved)

		// Auto-checkout PR branch before first AI query
		if !m.aiCheckedOut {
			m.aiCheckedOut = true
			return m, tea.Batch(
				flash.ShowMsg{ID: "ai-checkout", Text: "Checking out " + m.pr.Branch + " for AI…", Style: flash.StyleSpinner}.Cmd(),
				m.aiCheckoutThenQuery(task),
			)
		}

		cmd := m.aiPanel.Submit(task)
		return m, cmd
	}

	// Scroll viewport with ctrl+d/ctrl+u even while input is focused
	switch {
	case key.Matches(msg, keys.PageDown):
		m.aiPanel.viewport.HalfPageDown()
		return m, nil
	case key.Matches(msg, keys.PageUp):
		m.aiPanel.viewport.HalfPageUp()
		return m, nil
	}

	// Forward to textinput
	var cmd tea.Cmd
	m.aiPanel.input, cmd = m.aiPanel.input.Update(msg)

	// Check for @ context trigger after each keystroke
	m.updateAIContextState()

	return m, cmd
}

// handleAIPanelKey handles keys when AI panel is visible but streaming/drafting (input not focused).
func (m Model) handleAIPanelKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.AIAsk):
		// Toggle hide — work continues in background
		m.aiPanel.Hide()
		return m, nil

	case key.Matches(msg, keys.Back):
		// Just hide — work continues in background
		m.aiPanel.Hide()
		return m, nil

	case key.Matches(msg, keys.Quit):
		// ctrl+c cancels the active stream
		if m.aiPanel.cancelFn != nil {
			m.aiPanel.cancelFn()
			m.aiPanel.cancelFn = nil
		}
		if m.aiPanel.state == aiPanelStreaming {
			m.aiPanel.fullResponse = m.aiPanel.responseText.String()
			if m.aiPanel.fullResponse != "" {
				m.aiPanel.conversation = append(m.aiPanel.conversation, ai.ConversationEntry{
					Role: "assistant",
					Text: m.aiPanel.fullResponse,
				})
			}
		}
		m.aiPanel.state = aiPanelActive
		m.aiPanel.statusText = ""
		m.aiPanel.input.Focus()
		m.aiPanel.rebuildViewportContent()
		return m, nil

	case key.Matches(msg, keys.Up):
		m.aiPanel.viewport.ScrollUp(1)
		return m, nil
	case key.Matches(msg, keys.Down):
		m.aiPanel.viewport.ScrollDown(1)
		return m, nil
	case key.Matches(msg, keys.PageUp):
		m.aiPanel.viewport.HalfPageUp()
		return m, nil
	case key.Matches(msg, keys.PageDown):
		m.aiPanel.viewport.HalfPageDown()
		return m, nil
	}

	return m, nil
}

// aiCheckoutDoneMsg signals that the AI checkout completed and carries the queued task.
type aiCheckoutDoneMsg struct {
	task string
	err  error
}

// aiCheckoutThenQuery checks out the PR branch, then submits the task.
func (m Model) aiCheckoutThenQuery(task string) tea.Cmd {
	branch := m.pr.Branch
	useJJ := m.useJJ
	prNumber := m.pr.Number
	return func() tea.Msg {
		var err error
		if useJJ {
			err = jj.Checkout(branch)
		} else {
			err = git.CheckoutPR(prNumber)
		}
		return aiCheckoutDoneMsg{task: task, err: err}
	}
}

// buildTask builds a task prompt with PR metadata and conversation history.
func (m *Model) buildTask(question string) string {
	return ai.BuildTask(ai.TaskInput{
		PRNumber: m.pr.Number,
		PRTitle:  m.pr.Title,
		PRBody:   m.pr.Body,
		History:  m.aiPanel.conversation,
		Question: question,
	})
}

// startAIDraft initiates the two-pass draft call.
func (m Model) startAIDraft(mode ai.DraftMode) (Model, tea.Cmd) {
	diffFiles := m.buildDiffFileSummaries()
	cmd := m.aiPanel.StartDraft(mode, "", diffFiles)
	return m, cmd
}

// buildDiffFileSummaries creates summaries of all changed files for the draft prompt.
func (m *Model) buildDiffFileSummaries() []ai.DiffFileSummary {
	summaries := make([]ai.DiffFileSummary, 0, len(m.files))
	for _, f := range m.files {
		s := ai.DiffFileSummary{Path: f.Path}
		for _, h := range f.Hunks {
			s.Hunks = append(s.Hunks, ai.HunkRange{
				OldStart: h.OldStart,
				OldLines: h.OldLines,
				NewStart: h.NewStart,
				NewLines: h.NewLines,
			})
		}
		summaries = append(summaries, s)
	}
	return summaries
}

// applyDraftResult navigates to the proposed location and opens the comment editor pre-filled.
func (m Model) applyDraftResult(result *ai.DraftResult) tea.Cmd {
	fileIdx := -1
	for i, f := range m.files {
		if f.Path == result.Path {
			fileIdx = i
			break
		}
	}
	if fileIdx < 0 {
		return flash.ShowMsg{ID: "diffview", Text: fmt.Sprintf("Draft target file not found: %s", result.Path), Style: flash.StyleDanger}.Cmd()
	}

	if fileIdx != m.nav.cursor.FileIdx {
		oldIdx := m.nav.cursor.FileIdx
		m.nav.cursor.FileIdx = fileIdx
		m.nav.buildDiffLines(m.files)
		m.autoFollowFile(oldIdx, fileIdx)
	}

	targetLine := -1
	for i, dl := range m.nav.diffLines {
		if result.Side == "RIGHT" && dl.newLine == result.Line {
			targetLine = i
			break
		}
		if result.Side == "LEFT" && dl.oldLine == result.Line {
			targetLine = i
			break
		}
	}
	if targetLine < 0 {
		m.updateDiffContent()
		return flash.ShowMsg{ID: "diffview", Text: fmt.Sprintf("Draft target line %d not found in diff", result.Line), Style: flash.StyleDanger}.Cmd()
	}

	m.nav.cursor.LineIdx = targetLine
	m.syncViewport()

	side := result.Side
	if side == "" {
		side = "RIGHT"
	}
	mode := commentModeComment
	if strings.Contains(result.Body, "```suggestion") {
		mode = commentModeSuggestion
	}

	m.editor.Open(result.Path, result.Line, 0, side, mode, "", m.inlineTextareaWidth())
	m.editor.ta.SetValue(result.Body)
	m.updateViewports()

	return m.editor.BlinkCmd()
}
