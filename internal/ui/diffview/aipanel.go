package diffview

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"

	"github.com/jethrokuan/pry/internal/ai"
	"github.com/jethrokuan/pry/internal/ui/components/scrollbar"
	"github.com/jethrokuan/pry/internal/ui/styles"
)

// --- AI Panel messages ---

type aiStreamChunkMsg struct {
	text   string
	status string
}
type aiStreamDoneMsg struct{}
type aiStreamErrorMsg struct{ err error }
type aiDraftResultMsg struct {
	result *ai.DraftResult
	err    error
}

// aiActionMsg carries parsed actions from the agent's response.
type aiActionMsg struct {
	actions []ai.Action
}
type aiFirstChunkMsg struct {
	text   string
	status string
	ch     <-chan ai.StreamChunk
}
type aiNextChunkMsg struct {
	text   string
	status string
	ch     <-chan ai.StreamChunk
}

// --- AI Panel state ---

type aiPanelState int

const (
	aiPanelClosed    aiPanelState = iota
	aiPanelActive                 // panel open, input focused, ready for questions
	aiPanelStreaming              // agent is streaming a response
	aiPanelDrafting               // drafting a comment (second pass)
)

// AIPanel manages the AI review assistant panel.
// Terminal-like: single-line prompt always at bottom, conversation scrolls above.
type AIPanel struct {
	state   aiPanelState
	visible bool // whether the panel is shown (state can be active while hidden)

	viewport viewport.Model
	input    textinput.Model
	spinner  spinner.Model

	// Conversation state
	conversation []ai.ConversationEntry
	responseText *strings.Builder
	fullResponse string
	errorText    string
	statusText   string // current tool status ("Reading handler.go...")

	// @ context autocomplete
	contextState aiContextState

	// Agent
	agent    *ai.Agent
	cancelFn context.CancelFunc

	// Layout
	width  int
	height int
}

func initAIPanel() AIPanel {
	s := spinner.New()
	s.Spinner = spinner.Dot

	ti := textinput.New()
	ti.Prompt = "> "
	ti.Placeholder = "Ask about the code... (@ for context)"
	ti.CharLimit = 0

	return AIPanel{
		state:        aiPanelClosed,
		spinner:      s,
		input:        ti,
		responseText: &strings.Builder{},
	}
}

// IsOpen returns true if the panel is visible on screen.
func (p *AIPanel) IsOpen() bool {
	return p.visible
}

// IsInputActive returns true when the panel is visible and accepting text input.
func (p *AIPanel) IsInputActive() bool {
	return p.visible && p.state == aiPanelActive
}

// IsWorking returns true if the agent is doing background work (streaming/drafting),
// regardless of visibility.
func (p *AIPanel) IsWorking() bool {
	return p.state == aiPanelStreaming || p.state == aiPanelDrafting
}

// StatusText returns the current tool status for background indicator.
func (p *AIPanel) StatusText() string {
	return p.statusText
}

func (p *AIPanel) SetAgent(agent *ai.Agent) {
	p.agent = agent
}

// Open shows the AI panel. If already active (hidden), just shows it.
func (p *AIPanel) Open(width, height int) tea.Cmd {
	p.width = width
	p.height = height
	p.visible = true
	p.errorText = ""

	p.initViewport()
	p.rebuildViewportContent()

	// If not already doing something, focus the input
	if p.state == aiPanelClosed {
		p.state = aiPanelActive
	}
	if p.state == aiPanelActive {
		p.input.Focus()
		return textinput.Blink
	}
	return nil
}

// OpenWithDefaultPrompt opens and immediately starts analysis.
func (p *AIPanel) OpenWithDefaultPrompt(width, height int, task string) tea.Cmd {
	p.width = width
	p.height = height
	p.visible = true
	p.state = aiPanelStreaming
	p.errorText = ""
	p.responseText.Reset()
	p.fullResponse = ""

	p.initViewport()
	p.input.Blur()

	p.conversation = append(p.conversation, ai.ConversationEntry{
		Role: "reviewer",
		Text: "(analyze current hunk)",
	})
	p.rebuildViewportContent()

	return tea.Batch(p.spinner.Tick, p.startStream(task))
}

// Hide toggles the panel off-screen. Work continues in background.
func (p *AIPanel) Hide() {
	p.visible = false
	p.input.Blur()
}

// Close cancels any active work and hides the panel. Conversation preserved.
func (p *AIPanel) Close() {
	if p.cancelFn != nil {
		p.cancelFn()
		p.cancelFn = nil
	}
	p.visible = false
	p.state = aiPanelActive // ready for next question
	p.statusText = ""
	p.input.Blur()
	p.errorText = ""
}

// Clear resets conversation (/clear).
func (p *AIPanel) Clear() {
	if p.cancelFn != nil {
		p.cancelFn()
		p.cancelFn = nil
	}
	p.conversation = nil
	p.responseText.Reset()
	p.fullResponse = ""
	p.errorText = ""
	p.statusText = ""
	p.state = aiPanelActive
	p.input.SetValue("")
	if p.visible {
		p.input.Focus()
		p.rebuildViewportContent()
	}
}

func (p *AIPanel) HasHistory() bool {
	return len(p.conversation) > 0
}

// Submit sends the current input as a question.
func (p *AIPanel) Submit(task string) tea.Cmd {
	question := strings.TrimSpace(p.input.Value())
	if question == "" {
		return nil
	}

	p.conversation = append(p.conversation, ai.ConversationEntry{
		Role: "reviewer",
		Text: question,
	})
	p.input.SetValue("")
	p.state = aiPanelStreaming
	p.responseText.Reset()
	p.errorText = ""
	p.input.Blur()
	p.rebuildViewportContent()

	return tea.Batch(p.spinner.Tick, p.startStream(task))
}

// Resize updates the panel dimensions.
func (p *AIPanel) Resize(width, height int) {
	p.width = width
	p.height = height
	if p.IsOpen() {
		p.initViewport()
		p.input.SetWidth(width - 6) // account for border + prompt
		p.rebuildViewportContent()
	}
}

func (p *AIPanel) initViewport() {
	// viewport height = total height - title(1) - input(1) - help(1) - borders(1)
	vpH := p.height - 4
	if vpH < 1 {
		vpH = 1
	}
	p.viewport = viewport.New(viewport.WithWidth(p.width-5), viewport.WithHeight(vpH)) // -5 = border(2) + padding(2) + scrollbar(1)
	p.input.SetWidth(p.width - 6)
}

// startStream begins the agent query.
func (p *AIPanel) startStream(task string) tea.Cmd {
	if p.agent == nil {
		p.state = aiPanelActive
		p.input.Focus()
		p.errorText = "AI agent not configured"
		p.rebuildViewportContent()
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	p.cancelFn = cancel

	agent := p.agent
	return func() tea.Msg {
		ch, err := agent.Query(ctx, task)
		if err != nil {
			return aiStreamErrorMsg{err: err}
		}
		chunk, ok := <-ch
		if !ok {
			return aiStreamDoneMsg{}
		}
		return aiFirstChunkMsg{text: chunk.Text, status: chunk.Status, ch: ch}
	}
}

func streamContinueCmd(ch <-chan ai.StreamChunk) tea.Cmd {
	return func() tea.Msg {
		chunk, ok := <-ch
		if !ok {
			return aiStreamDoneMsg{}
		}
		return aiNextChunkMsg{text: chunk.Text, status: chunk.Status, ch: ch}
	}
}

// HandleStreamMsg processes streaming messages.
func (p *AIPanel) HandleStreamMsg(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case aiFirstChunkMsg:
		p.responseText.WriteString(msg.text)
		if msg.status != "" {
			p.statusText = msg.status
		}
		p.rebuildViewportContent()
		return streamContinueCmd(msg.ch)
	case aiNextChunkMsg:
		p.responseText.WriteString(msg.text)
		if msg.status != "" {
			p.statusText = msg.status
		}
		p.rebuildViewportContent()
		return streamContinueCmd(msg.ch)
	case aiStreamChunkMsg:
		p.responseText.WriteString(msg.text)
		if msg.status != "" {
			p.statusText = msg.status
		}
		p.rebuildViewportContent()
		return nil
	case aiStreamDoneMsg:
		p.fullResponse = p.responseText.String()
		p.statusText = ""

		// Parse action blocks from the response
		actions, cleanText := ai.ParseActions(p.fullResponse)

		p.conversation = append(p.conversation, ai.ConversationEntry{
			Role: "assistant",
			Text: cleanText,
		})
		p.state = aiPanelActive
		p.input.Focus()
		p.rebuildViewportContent()

		var cmds []tea.Cmd
		cmds = append(cmds, textinput.Blink)
		if len(actions) > 0 {
			acts := actions // capture for closure
			cmds = append(cmds, func() tea.Msg {
				return aiActionMsg{actions: acts}
			})
		}
		return tea.Batch(cmds...)
	case aiStreamErrorMsg:
		p.errorText = msg.err.Error()
		p.statusText = ""
		p.state = aiPanelActive
		p.input.Focus()
		p.rebuildViewportContent()
		return textinput.Blink
	case spinner.TickMsg:
		if p.state == aiPanelStreaming || p.state == aiPanelDrafting {
			var cmd tea.Cmd
			p.spinner, cmd = p.spinner.Update(msg)
			return cmd
		}
	}
	return nil
}

func (p *AIPanel) rebuildViewportContent() {
	var b strings.Builder

	promptStyle := lipgloss.NewStyle().Foreground(styles.Cyan).Bold(true)
	userTextStyle := lipgloss.NewStyle().Bold(true)
	sepStyle := lipgloss.NewStyle().Foreground(styles.Muted)

	contentWidth := p.width - 6 // account for border + padding
	if contentWidth < 20 {
		contentWidth = 20
	}

	for i, entry := range p.conversation {
		if entry.Role == "reviewer" {
			if i > 0 {
				b.WriteString(sepStyle.Render("───") + "\n")
			}
			b.WriteString(promptStyle.Render("> ") + userTextStyle.Render(entry.Text) + "\n\n")
		} else {
			// Render completed assistant text as markdown
			rendered := p.renderMD(entry.Text, contentWidth)
			b.WriteString(rendered + "\n")
		}
	}

	if p.state == aiPanelStreaming {
		current := p.responseText.String()
		if current != "" {
			// Streaming text: render raw (partial markdown looks broken)
			b.WriteString(current)
			if p.statusText != "" {
				b.WriteString("\n" + lipgloss.NewStyle().Foreground(styles.Muted).Italic(true).Render(p.statusText))
			}
		} else {
			status := "Thinking..."
			if p.statusText != "" {
				status = p.statusText
			}
			b.WriteString(p.spinner.View() + " " + status)
		}
	}

	if p.errorText != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(styles.Danger).Render("Error: "+p.errorText) + "\n")
	}

	p.viewport.SetContent(b.String())
	p.viewport.GotoBottom()
}

// renderMD renders markdown text for the AI panel using Glamour
// with the same style config as the rest of the app.
func (p *AIPanel) renderMD(text string, width int) string {
	if width < 10 {
		width = 10
	}
	renderer, err := glamour.NewTermRenderer(
		glamour.WithWordWrap(width),
		glamour.WithStyles(mdStyleConfig()),
	)
	if err != nil {
		return text
	}
	rendered, err := renderer.Render(text)
	if err != nil {
		return text
	}
	return strings.TrimRight(rendered, "\n")
}

func (p *AIPanel) Conversation() []ai.ConversationEntry {
	return p.conversation
}

// View renders the AI panel as a bordered popup box.
func (p *AIPanel) View(contextDropdown ...string) string {
	if !p.IsOpen() {
		return ""
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.Primary)
	title := titleStyle.Render("  AI Assistant")
	if p.state == aiPanelStreaming {
		title += " " + p.spinner.View()
	}

	vpView := p.viewport.View()

	// Add scrollbar alongside viewport
	sb := scrollbar.New()
	sb.Height = p.viewport.Height()
	sb.TotalItems = p.viewport.TotalLineCount()
	sb.VisibleItems = p.viewport.Height()
	sb.Offset = p.viewport.YOffset()
	sb.ThumbColor = styles.Primary
	if sbView := sb.View(); sbView != "" {
		vpView = lipgloss.JoinHorizontal(lipgloss.Top, vpView, sbView)
	}

	// Input line (always visible)
	inputView := p.input.View()
	if p.state == aiPanelStreaming || p.state == aiPanelDrafting {
		status := "Thinking..."
		if p.statusText != "" {
			status = p.statusText
		}
		if p.state == aiPanelDrafting {
			status = "Drafting comment..."
		}
		inputView = lipgloss.NewStyle().Foreground(styles.Muted).Render(p.spinner.View() + " " + status)
	}

	// @ autocomplete dropdown above input
	dropdown := ""
	if len(contextDropdown) > 0 {
		dropdown = contextDropdown[0]
	}

	help := p.renderHelp()

	// Assemble inner content
	var parts []string
	parts = append(parts, vpView)
	if dropdown != "" {
		parts = append(parts, dropdown)
	}
	parts = append(parts, inputView)
	inner := strings.Join(parts, "\n")

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.Primary).
		Padding(0, 1).
		Width(p.width)

	return title + "\n" + boxStyle.Render(inner) + "\n" + help
}

func (p *AIPanel) renderHelp() string {
	var parts []string
	switch p.state {
	case aiPanelActive:
		parts = append(parts, "enter send", "@ context", "/clear reset", "a/esc hide")
	case aiPanelStreaming:
		parts = append(parts, "ctrl+c cancel", "a/esc hide")
	case aiPanelDrafting:
		parts = append(parts, "ctrl+c cancel", "a/esc hide")
	}
	return styles.HelpStyle.Render(strings.Join(parts, "  "))
}

// StartDraft initiates the second-pass draft call.
func (p *AIPanel) StartDraft(mode ai.DraftMode, directive string, diffFiles []ai.DiffFileSummary) tea.Cmd {
	if p.agent == nil || len(p.conversation) == 0 {
		return nil
	}

	p.state = aiPanelDrafting
	p.input.Blur()
	agent := p.agent
	conv := make([]ai.ConversationEntry, len(p.conversation))
	copy(conv, p.conversation)

	return func() tea.Msg {
		result, err := agent.DraftComment(context.Background(), conv, directive, mode, diffFiles)
		return aiDraftResultMsg{result: result, err: err}
	}
}

func (p *AIPanel) HandleDraftResult(msg aiDraftResultMsg) {
	p.state = aiPanelActive
	p.input.Focus()
	if msg.err != nil {
		p.errorText = fmt.Sprintf("Draft failed: %v", msg.err)
		p.rebuildViewportContent()
	}
}
