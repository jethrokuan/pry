package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	claudecode "github.com/severity1/claude-agent-sdk-go"
)

// Agent wraps the Claude Code SDK for AI-assisted code review.
type Agent struct {
	repoRoot string
	model    string
	maxTurns int
}

// NewAgent creates a new AI agent.
func NewAgent(repoRoot, model string, maxTurns int) *Agent {
	return &Agent{
		repoRoot: repoRoot,
		model:    model,
		maxTurns: maxTurns,
	}
}

// Available returns true if the claude CLI is on PATH.
func Available() bool {
	_, err := exec.LookPath("claude")
	return err == nil
}

// TaskInput holds the data needed to compose a task prompt.
type TaskInput struct {
	PRNumber int
	PRTitle  string
	PRBody   string
	History  []ConversationEntry // prior conversation turns
	Question string              // the current question (empty for default analysis)
}

// BuildTask composes the task prompt for a query, including conversation history.
func BuildTask(input TaskInput) string {
	var b strings.Builder

	fmt.Fprintf(&b, "PR: #%d — %s\n", input.PRNumber, input.PRTitle)
	if input.PRBody != "" {
		fmt.Fprintf(&b, "PR description:\n%s\n\n", input.PRBody)
	}

	// Include conversation history so the agent has full context
	if len(input.History) > 0 {
		b.WriteString("## Conversation so far\n\n")
		for _, entry := range input.History {
			if entry.Role == "reviewer" {
				fmt.Fprintf(&b, "**Reviewer:** %s\n\n", entry.Text)
			} else {
				fmt.Fprintf(&b, "**Assistant:** %s\n\n", entry.Text)
			}
		}
		b.WriteString("---\n\n")
	}

	if input.Question != "" {
		fmt.Fprintf(&b, "Reviewer's question: %s\n", input.Question)
	} else {
		b.WriteString("Explain this change and flag anything concerning.\n")
	}

	return b.String()
}

// systemPrompt is the system prompt for the review assistant.
const systemPrompt = `You are assisting a code reviewer in a terminal UI. The reviewer is examining a pull request.

The PR branch is checked out locally. You have full access to the codebase and tools:
- Read, Glob, Grep — explore files, trace references, check types
- Bash — run commands, including ` + "`gh`" + ` CLI for PR data

To get the PR diff: ` + "`gh pr diff`" + `
To get PR comments: ` + "`gh pr view --comments`" + `
To get specific file diff: ` + "`gh pr diff -- path/to/file.go`" + `

Be concise. Reference specific file paths and line numbers.

## Actions

When the reviewer asks you to post a comment or suggestion, output an action block. The UI will detect it and execute it automatically.

To post a review comment:
` + "```pry-action" + `
{"action":"comment","path":"src/handler.go","line":42,"side":"RIGHT","body":"This needs a nil check before dereferencing."}
` + "```" + `

To post a code suggestion:
` + "```pry-action" + `
{"action":"suggest","path":"src/handler.go","line":42,"side":"RIGHT","body":"Add nil check:\n` + "```" + `suggestion\nif handler == nil {\n    return ErrNilHandler\n}\n` + "```" + `"}
` + "```" + `

To reply to an existing review thread:
` + "```pry-action" + `
{"action":"reply","path":"src/handler.go","line":42,"side":"RIGHT","body":"Good catch — I see the same issue at line 78 too."}
` + "```" + `

Rules for actions:
- "path" must be a file in the PR's diff
- "line" must be a line number within a changed hunk (new-side line for RIGHT, old-side for LEFT)
- "side" is "RIGHT" (commenting on new code, most common) or "LEFT" (commenting on deleted code)
- For "reply", the path+line+side must match an existing thread — the reply is appended to that thread
- You may output multiple action blocks in one response
- Always explain your reasoning before the action block
- The reviewer can edit or cancel before the comment is saved`

// Query starts a streaming conversation with the agent.
// Returns a channel that yields text chunks until the stream completes.
func (a *Agent) Query(ctx context.Context, task string) (<-chan StreamChunk, error) {
	opts := []claudecode.Option{
		claudecode.WithCwd(a.repoRoot),
		claudecode.WithAllowedTools("Read", "Glob", "Grep", "Bash"),
		claudecode.WithSystemPrompt(systemPrompt),
	}
	if a.maxTurns > 0 {
		opts = append(opts, claudecode.WithMaxTurns(a.maxTurns))
	}
	if a.model != "" {
		opts = append(opts, claudecode.WithModel(a.model))
	}

	iter, err := claudecode.Query(ctx, task, opts...)
	if err != nil {
		return nil, fmt.Errorf("claude query: %w", err)
	}

	ch := make(chan StreamChunk, 16)
	go func() {
		defer close(ch)
		defer iter.Close()
		for {
			msg, err := iter.Next(ctx)
			if err != nil {
				if !errors.Is(err, claudecode.ErrNoMoreMessages) {
					slog.Debug("agent stream error", "error", err)
				}
				return
			}
			chunk := extractChunk(msg)
			if chunk.Text != "" || chunk.Status != "" {
				select {
				case ch <- chunk:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return ch, nil
}

// DraftComment performs the second-pass draft call to produce a structured review comment.
func (a *Agent) DraftComment(ctx context.Context, conversation []ConversationEntry, directive string, mode DraftMode, diffFiles []DiffFileSummary) (*DraftResult, error) {
	task := buildDraftPrompt(conversation, directive, mode, diffFiles)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	opts := []claudecode.Option{
		claudecode.WithMaxTurns(1),
	}
	if a.model != "" {
		opts = append(opts, claudecode.WithModel(a.model))
	}

	iter, err := claudecode.Query(ctx, task, opts...)
	if err != nil {
		return nil, fmt.Errorf("draft query: %w", err)
	}
	defer iter.Close()

	var fullText strings.Builder
	for {
		msg, err := iter.Next(ctx)
		if err != nil {
			if errors.Is(err, claudecode.ErrNoMoreMessages) {
				break
			}
			return nil, fmt.Errorf("draft stream: %w", err)
		}
		if chunk := extractChunk(msg); chunk.Text != "" {
			fullText.WriteString(chunk.Text)
		}
	}

	// Parse JSON result
	raw := strings.TrimSpace(fullText.String())
	// Strip markdown code fences if present
	if strings.HasPrefix(raw, "```") {
		lines := strings.Split(raw, "\n")
		if len(lines) > 2 {
			raw = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}

	var result DraftResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("draft parse: %w (raw: %s)", err, raw)
	}
	return &result, nil
}

func buildDraftPrompt(conversation []ConversationEntry, directive string, mode DraftMode, diffFiles []DiffFileSummary) string {
	var b strings.Builder

	modeStr := "comment"
	if mode == DraftSuggestion {
		modeStr = "suggestion"
	}

	fmt.Fprintf(&b, "Given this conversation between a code reviewer and an AI assistant,\n")
	fmt.Fprintf(&b, "produce a review %s for the pull request.\n\n", modeStr)

	b.WriteString("Conversation:\n")
	for _, e := range conversation {
		fmt.Fprintf(&b, "[%s]: %s\n\n", e.Role, e.Text)
	}

	if directive != "" {
		fmt.Fprintf(&b, "Reviewer's directive: %s\n\n", directive)
	} else {
		b.WriteString("Address the main finding from the conversation.\n\n")
	}

	b.WriteString("The comment must target a line within the PR's diff. Valid targets:\n")
	for _, f := range diffFiles {
		fmt.Fprintf(&b, "  %s:\n", f.Path)
		for _, h := range f.Hunks {
			fmt.Fprintf(&b, "    OLD %d-%d, NEW %d-%d\n", h.OldStart, h.OldStart+h.OldLines-1, h.NewStart, h.NewStart+h.NewLines-1)
		}
	}

	b.WriteString("\nRespond with ONLY a JSON object:\n")
	b.WriteString("{\n")
	b.WriteString("  \"path\": \"file path\",\n")
	b.WriteString("  \"line\": line_number,\n")
	b.WriteString("  \"side\": \"RIGHT\" or \"LEFT\",\n")
	b.WriteString("  \"body\": \"the comment body\"\n")
	b.WriteString("}\n")

	if mode == DraftSuggestion {
		b.WriteString("\nFor suggestions, format the body as:\n")
		b.WriteString("The explanation of the issue.\n\n")
		b.WriteString("```suggestion\nthe suggested replacement code\n```\n")
	}

	b.WriteString("\nNo preamble. No explanation outside the JSON.\n")
	return b.String()
}

// extractChunk pulls text and tool-use status from a Claude SDK message.
func extractChunk(msg claudecode.Message) StreamChunk {
	switch m := msg.(type) {
	case *claudecode.AssistantMessage:
		var chunk StreamChunk
		for _, block := range m.Content {
			switch b := block.(type) {
			case *claudecode.TextBlock:
				chunk.Text += b.Text
			case *claudecode.ToolUseBlock:
				chunk.Status = formatToolStatus(b.Name, b.Input)
			}
		}
		return chunk
	default:
		return StreamChunk{}
	}
}

// formatToolStatus produces a human-readable status from a tool use event.
func formatToolStatus(name string, input map[string]any) string {
	switch name {
	case "Read":
		if path, ok := input["file_path"].(string); ok {
			return "Reading " + shortenPath(path)
		}
		return "Reading file..."
	case "Glob":
		if pattern, ok := input["pattern"].(string); ok {
			return "Finding " + pattern
		}
		return "Finding files..."
	case "Grep":
		if pattern, ok := input["pattern"].(string); ok {
			return "Searching for " + pattern
		}
		return "Searching..."
	default:
		return "Using " + name + "..."
	}
}

// shortenPath returns the last 2 path components for display.
func shortenPath(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) <= 2 {
		return path
	}
	return "…/" + strings.Join(parts[len(parts)-2:], "/")
}
