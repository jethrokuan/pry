# AI Review Assistant — Design Document

## Motivation

Code review is bottlenecked by human cognition, not tooling. Reviewers already have the diff — what they lack is the ability to quickly understand unfamiliar code, trace the implications of a change, and articulate their concerns clearly. Existing AI review tools (Coderabbit, Copilot PR reviews) operate as autonomous bots that post comments. They replace the reviewer rather than augmenting them.

Pry's AI assistant takes a different approach: **the human asks the questions, the AI helps answer them.** The reviewer decides what to look at, what matters, and what to say. The AI acts as a research tool — one that can read the entire local codebase, follow references, and draft responses — available on demand at any point during review.

## Principles

1. **Human-driven.** The AI never acts unprompted. No automatic comments, no background analysis, no unsolicited suggestions. Every AI interaction starts with a deliberate reviewer action.

2. **Codebase-grounded.** The AI has full access to the local checkout — not just the diff. It can read files, follow imports, check types, grep for callers. This is the key differentiator over web-based tools that only see the patch.

3. **Agent, not prompt.** Pry spawns a coding agent (Claude Code / Agent SDK) rather than making raw LLM API calls. The agent decides what context to gather. Pry doesn't try to build a context package — it describes the task and lets the agent explore.

4. **Comment-ready output.** Every AI response is one keystroke away from becoming a review comment or code suggestion. The AI drafts; the reviewer edits and owns the final word.

5. **Zero footprint when unused.** If no AI backend is configured, no UI elements appear and no keybindings are registered. The review experience is unchanged.

## Conversation and output

The AI panel is a single, unified conversation. There are no separate "modes" — the reviewer asks whatever they want, and the conversation flows naturally. The distinction between understanding and responding happens at **output time**, not input time:

- **`p` (post comment)** — distills the conversation into a review comment and proposes where to place it
- **`s` (post suggestion)** — distills the conversation into a code suggestion block with a concrete fix

The conversation itself is open-ended. The reviewer might:
- Ask what a function does, then ask about its callers, then press `p` to post a comment about a missing error case
- Describe a problem ("this looks like a race condition on the map"), discuss it with the AI, then press `s` to get a concrete fix
- Ask a question, get the answer, and never post anything — the understanding was the goal

Examples of reviewer questions:
- "What does this function do?"
- "Does this change break any callers?"
- "This looks like it duplicates logic in utils.go"
- "Should this handle nil?"
- (no question, press `A`) → general analysis of the hunk the reviewer is looking at

## Interaction

### Opening the panel

| Key | Behavior |
|-----|----------|
| `a` | Opens input prompt → type question → enter → AI panel opens |
| `A` | Opens AI panel immediately with default analysis prompt (no input step) |

Pressing `a`/`A` while on a diff line provides the agent with **initial context** — the current file, hunk, and cursor position. This helps the agent orient ("the reviewer is looking at handler.go, the hunk that adds a context timeout") but does **not** anchor the conversation to that location. The conversation is free to roam across the PR.

If visual selection is active (lines selected via `space`), the selected lines are included as additional context.

### Panel layout

The AI panel appears as a right-side split, similar to the existing file tree but on the opposite side. The diff viewport shrinks to accommodate it.

```
┌─ file tree ─┬─ diff ──────────────────────┬─ AI ──────────────────┐
│ src/        │ @@ -140,6 +142,8 @@         │                       │
│  handler.go │   func (s *Server) Handle(  │ This adds a context   │
│  worker.go  │ +   ctx, cancel := ...      │ timeout to the RPC    │
│  types.go   │ +   defer cancel()          │ call. However, the    │
│             │     resp, err := s.client.  │ callers in worker.go  │
│             │                             │ expect ErrTimeout,    │
│             │                             │ not DeadlineExceeded. │
│             │                             │                       │
│             │                             │ See worker.go:78 — the│
│             │                             │ retry logic won't     │
│             │                             │ trigger for this new  │
│             │                             │ error type.           │
│             │                             │                       │
│             │                             │ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ │
│             │                             │ > _                   │
└─────────────┴─────────────────────────────┴───────────────────────┘
 j/k scroll · enter follow-up · p post comment · s post suggestion · esc close
```

### In-panel keybindings

| Key | Action |
|-----|--------|
| `j`/`k` | Scroll AI response |
| `enter` | Type a follow-up question (conversation continues with full context) |
| `p` | **Post as comment**: opens input prompt for a brief directive ("the nil check", "the retry logic"), then AI drafts a comment with a proposed location. Pry navigates there and opens the comment editor pre-filled. Reviewer edits before saving. Empty input uses the full conversation. |
| `s` | **Post as suggestion**: same as `p` but the draft includes a `suggestion` code block with a concrete fix. |
| `esc` | Close panel, return to diff navigation |

### Conversation model

The conversation is **not anchored** to a specific line. It's a free-form dialogue about the PR, seeded with context from wherever the reviewer was when they opened the panel.

This means:
- The reviewer can ask about any part of the PR, not just the initial hunk
- Follow-up questions can span files ("what about the callers in worker.go?")
- The agent's responses can reference any location in the codebase

The conversation persists while the panel is open. Closing the panel (`esc`) ends the conversation. The reviewer can open a new conversation at any time from any location in the diff.

### Comment placement

When the reviewer presses `p` or `s`, the **draft call** (second pass) asks the agent to produce structured output:

1. **The comment body** — a clean, review-ready comment distilled from the conversation
2. **The proposed location** — `{path, line, side}` where the comment should be placed

Pry then:
1. Navigates the diff to the proposed file and line
2. Opens the inline comment editor pre-filled with the draft body
3. The reviewer sees both the proposed location and the text
4. The reviewer can edit the text, or press `esc` and navigate to a different line before re-invoking

This keeps the human in control of the final placement while leveraging the agent's understanding of where the issue actually is. The agent knows "the problem is in worker.go:78" because it read the code during the conversation — the reviewer doesn't need to find the line themselves.

**Multiple comments from one conversation:** The reviewer can press `p`/`s` repeatedly, each time with a different directive targeting a different finding from the conversation. "The nil check" → post → "the retry logic" → post. The conversation stays open throughout.

**Constraint:** The proposed location must be within the PR's diff. The agent can only suggest lines that appear in a hunk (additions/context on the right side, deletions/context on the left side). Lines outside the diff are not valid comment targets on GitHub. The draft prompt enforces this by including the list of changed files and their line ranges.

## Agent integration

### SDK

Pry uses [claude-agent-sdk-go](https://github.com/severity1/claude-agent-sdk-go) — a Go wrapper around the Claude Code CLI that provides typed message parsing, streaming, session management, and process lifecycle handling. Prerequisite: the user must have `claude` installed (`npm install -g @anthropic-ai/claude-code`).

The SDK spawns `claude` as a subprocess with `--output-format stream-json`, communicating via structured JSON lines over stdin/stdout. Pry consumes this through the SDK's iterator API rather than parsing raw JSON.

### Architecture

```
┌─────────────┐                      ┌──────────────────────┐
│  diffview    │   claudecode.Query   │  claude CLI process  │
│  (Bubble Tea)│ ──────────────────── │  (spawned by SDK)    │
│              │                      │                      │
│  AI panel    │ ←── MessageIterator  │  Tools:              │
│  (viewport)  │     typed messages   │  · Read (files)      │
│              │     text deltas      │  · Glob (find files) │
│  compose task│                      │  · Grep (search)     │
│  stream text │                      │                      │
│  post comment│                      │  Working dir:        │
│              │                      │  · repo root (local  │
└──────────────┘                      │    checkout of PR)   │
                                      └──────────────────────┘
```

Pry's responsibility is minimal:

1. **Compose the task** — the user's question, the hunk, the file path, line range, PR description, and the mode (ask vs. suggest)
2. **Call the SDK** — `claudecode.Query()` with options for tools, model, system prompt, working directory
3. **Stream output** — iterate `MessageIterator`, dispatch text deltas as Bubble Tea messages to the panel viewport
4. **Capture result** — accumulate full response text for the `p`/`s` actions

Pry does **not** decide what files the agent should read or how to build context. The agent has read-only access to the local checkout and explores autonomously.

### SDK usage

```go
// internal/ai/agent.go

func (a *Agent) Query(ctx context.Context, task string) (<-chan StreamChunk, error) {
    iter, err := claudecode.Query(ctx, task,
        claudecode.WithCwd(a.repoRoot),
        claudecode.WithAllowedTools("Read", "Glob", "Grep"),
        claudecode.WithSystemPrompt(a.systemPrompt),
        claudecode.WithModel(a.model),
        claudecode.WithMaxTurns(a.maxTurns),
    )
    if err != nil {
        return nil, err
    }

    ch := make(chan StreamChunk)
    go func() {
        defer close(ch)
        defer iter.Close()
        for {
            msg, err := iter.Next(ctx)
            if err != nil {
                // context cancelled (user pressed esc) or stream ended
                return
            }
            // extract text deltas from assistant messages
            if text, ok := extractTextDelta(msg); ok {
                ch <- StreamChunk{Text: text}
            }
        }
    }()
    return ch, nil
}
```

The `StreamChunk` channel integrates with Bubble Tea's command pattern:

```go
// internal/ui/diffview/ai_panel.go

func streamAgentCmd(ch <-chan ai.StreamChunk) tea.Cmd {
    return func() tea.Msg {
        chunk, ok := <-ch
        if !ok {
            return aiStreamDoneMsg{}
        }
        return aiStreamChunkMsg{text: chunk.Text}
    }
}
```

Each chunk message appends to the panel content and triggers a re-render. The command re-subscribes to the channel until it closes (standard Bubble Tea pattern for streaming).

### Two-pass drafting

The panel conversation is exploratory — the agent explains, reasons, references files. This output is valuable for the reviewer but not formatted for a review comment, and doesn't specify where a comment should go.

When the reviewer presses `p` (post as comment) or `s` (post as suggestion), a **second-pass draft call** produces a structured result: the comment body *and* the proposed location.

```go
// DraftResult is the structured output of the second-pass draft call.
type DraftResult struct {
    Path string `json:"path"`    // file path within the PR
    Line int    `json:"line"`    // line number in the diff
    Side string `json:"side"`    // "RIGHT" (new code) or "LEFT" (old code)
    Body string `json:"body"`    // the comment text, or suggestion block
}

func (a *Agent) DraftComment(ctx context.Context, conversation string, directive string, mode DraftMode, diffFiles []DiffFileSummary) (*DraftResult, error) {
    task := buildDraftPrompt(conversation, directive, mode, diffFiles)
    iter, err := claudecode.Query(ctx, task,
        claudecode.WithMaxTurns(1),  // no exploration needed, just formatting
        claudecode.WithModel(a.model),
    )
    // ... collect full response, parse JSON, return DraftResult
}
```

The draft prompt includes:
- The full conversation transcript
- The draft mode (`comment` or `suggestion`)
- A summary of all changed files and their valid line ranges (so the agent proposes a valid location)

The prompt instructs the agent to output a JSON object with `path`, `line`, `side`, and `body`. For suggestions, `body` contains a `` ```suggestion `` block. No preamble, no explanation — just the structured result.

Pry parses the result, navigates the diff to `path:line`, and opens the inline comment editor pre-filled with `body`. The reviewer sees the proposed location in context and edits before saving.

### Task format

The task sent to the agent for the initial query:

```
You are assisting a code reviewer in a terminal UI. The reviewer is
examining a pull request and has a question. You have read-only access
to the local repository checkout — use it freely to read files, search
for references, and understand the codebase.

PR: #{number} — {title}
PR description:
{body}

The reviewer is currently looking at this part of the diff:

File: {path}
Lines: {start}-{end}

{hunk content}

Reviewer's question: {question or "Explain this change and flag anything concerning."}

Answer the reviewer's question. Be concise and reference specific file
paths and line numbers. The conversation is not limited to the file
shown above — follow references across the codebase as needed.
```

The initial file/hunk context seeds the conversation but doesn't constrain it. Follow-up questions are appended to the conversation history naturally by the SDK's session management.

The **draft call** (second pass, on `p`/`s`) uses a separate prompt:

```
Given this conversation between a code reviewer and an AI assistant,
produce a review {comment | suggestion} for the pull request.

Conversation:
{transcript}

Reviewer's directive: {directive, or "Address the main finding from the conversation."}

The comment must target a line within the PR's diff. Valid targets:
{for each file: path, list of (start, end, side) ranges from hunks}

Respond with ONLY a JSON object:
{
  "path": "file path",
  "line": line_number,
  "side": "RIGHT" or "LEFT",
  "body": "the comment body"
}

For suggestions, format the body as:
The explanation of the issue.

```suggestion
the suggested replacement code
```

No preamble. No explanation outside the JSON.
```

The directive lets the reviewer steer which finding becomes a comment. This enables multiple comments from one conversation — each `p`/`s` press with a different directive extracts a different finding.

### Streaming

The SDK's `MessageIterator` yields typed messages as JSON lines arrive from the subprocess. Pry filters for text content deltas and dispatches them as Bubble Tea messages. The panel viewport appends each chunk and re-renders incrementally.

A spinner in the panel header indicates the agent is working. When the agent reads files or runs searches, those tool-use events could optionally be shown as status updates ("Reading handler.go...", "Searching for callers...") to give the reviewer visibility into the agent's exploration.

### Cancellation

Pressing `esc` while the agent is streaming cancels the context passed to `claudecode.Query`. The SDK handles graceful shutdown (SIGTERM → SIGKILL). The partial response is kept — the reviewer can still use `p` to post what's there.

### Future: MCP tools

The SDK supports MCP (Model Context Protocol) servers. In a later phase, pry could expose custom tools to the agent via an in-process MCP server:

- `get_pr_threads(file)` — return all review threads on a file (no need for the agent to understand GitHub's API)
- `get_pr_description()` — return the PR body
- `get_review_summary()` — return comments posted so far in this session

This would let the agent access pry's in-memory review state directly, without needing to re-derive it from the filesystem.

## Configuration

```toml
# -------------------------------------------------------------------------
# AI Review Assistant
# -------------------------------------------------------------------------

# [ai]
# Enable the AI review assistant. Requires `claude` CLI installed.
# enabled = false

# Model to use. Optional — uses Claude Code's default if not set.
# model = "sonnet"

# Maximum agent turns per query. Higher = deeper exploration, slower response.
# max_turns = 10
```

When `ai.enabled` is false (or `claude` is not found on `$PATH`), the `a`/`A` keybindings are not registered and no AI-related UI elements appear.

## Implementation plan

### Phase 1: Panel infrastructure

Add a right-side panel component to diffview — a scrollable viewport that can be toggled independently of the file tree. No AI yet; just the panel shell with open/close, scrolling, and the input prompt. This is reusable UI infrastructure.

### Phase 2: Agent spawning and streaming

Implement the agent subprocess lifecycle: spawn on panel open, stream output into the panel viewport, handle cancellation. The task format is hardcoded to ask mode with the default analysis prompt. Test with real Claude Code pointed at a local repo.

### Phase 3: Ask mode

Full ask mode: input prompt for custom questions, follow-up conversation, conversation persistence per anchor. The `p` (post as comment) action bridges to the existing inline comment editor.

### Phase 4: Suggest mode

Add suggest mode with its own keybinding and task instructions. The `s` action parses code blocks from the AI response and creates suggestion comments.

### Phase 5: Session summary

Track AI conversations and posted comments across the review session. On the submit screen, offer to generate a review body summarizing findings.

## Test plan

### Unit tests (no agent, no network)

These test pry's own logic — the panel component, task composition, draft parsing, and integration with the existing comment flow. All use mock data, no Claude CLI required.

**Panel UI component:**
- Panel opens/closes with `a`/`esc`, diff viewport resizes correctly
- Panel coexists with file tree (three-way split renders without overlap)
- `j`/`k` scrolls panel content, respects bounds
- `enter` opens input prompt, `esc` from input returns to panel scroll mode
- Input prompt captures text and emits correct message on enter
- Panel keybindings are inactive when panel is closed
- Panel keybindings don't leak to diff navigation when panel is focused

**Task composition:**
- Initial task includes PR number, title, body, file path, line range, hunk content, and reviewer question
- When invoked with visual selection, task includes selected lines instead of full hunk
- When invoked with `A` (no input), task uses default analysis prompt
- Task escapes/formats hunk content correctly (no prompt injection from diff content)

**Draft result parsing:**
- Parses valid JSON `{path, line, side, body}` correctly
- Handles suggestion body with `` ```suggestion `` blocks
- Rejects responses where `path` is not in the PR's changed files
- Rejects responses where `line` is outside valid hunk ranges
- Returns error on malformed JSON (partial response, preamble, etc.)
- Handles escaped characters in body (newlines, quotes, backticks)

**Comment placement flow:**
- On valid draft result: navigates diff to `path:line`, opens comment editor pre-filled with `body`
- On suggestion draft: opens suggestion editor with code block extracted
- On invalid location: shows error in panel, does not open editor
- Editor pre-fill preserves exact body text (no re-escaping or truncation)
- Pressing `esc` in the pre-filled editor cancels without posting (existing behavior)

**Configuration:**
- AI keybindings registered when `ai.enabled = true` and `claude` found on `$PATH`
- AI keybindings not registered when `ai.enabled = false`
- AI keybindings not registered when `claude` not found on `$PATH`
- Config defaults: `enabled = false`, `max_turns = 10`

**Streaming integration (with mock channel):**
- Chunk messages append to panel content incrementally
- Done message stops spinner, marks conversation complete
- Error message displays error in panel
- Cancellation (esc during stream) stops consuming from channel
- Partial response preserved after cancellation — `p`/`s` still work

### Integration tests (with agent, no network)

These test the full loop: pry composes a task, the agent runs against a local repo, and pry processes the output. Uses a real Claude CLI pointed at a test repository (a small, checked-in fixture repo). These tests are slow and require a valid API key — gated behind a build tag (`//go:build integration`).

**End-to-end ask flow:**
- Open panel on a hunk → agent responds with relevant analysis referencing the correct file
- Ask a follow-up question → agent response is aware of prior conversation context
- Agent reads files from the local checkout (verify via tool-use events in the stream)

**End-to-end draft flow:**
- After a conversation, press `p` with directive → draft result has valid `path`/`line` within the diff
- Press `s` with directive → draft result contains a `` ```suggestion `` block
- Press `p` with empty directive → draft addresses the main finding
- Press `p` twice with different directives → produces two distinct comments at different locations

**Agent constraints:**
- Agent only uses allowed tools (Read, Glob, Grep) — no Write, Edit, or Bash
- Agent working directory is the repo root
- Agent respects `max_turns` limit

### Manual testing scenarios

These are exercised by hand during development and before release. They test the full UX with real PRs.

**Basic flow:**
1. Open a real PR in pry, navigate to a non-trivial hunk
2. Press `a`, type "what does this change do?", enter
3. Verify: panel opens, streaming response appears, agent references correct files
4. Press `enter`, ask "does this break any callers?"
5. Verify: follow-up response is contextual, references other files in the repo
6. Press `p`, type "the missing error handling", enter
7. Verify: diff navigates to the correct line, comment editor opens pre-filled with a clean comment
8. Edit the comment, `ctrl+s` to save
9. Verify: comment appears in the diff as a pending review comment at the proposed location

**Suggestion flow:**
1. Navigate to a hunk with an obvious improvement opportunity
2. Press `a`, type "this should handle the nil case"
3. Press `s`, type "nil check", enter
4. Verify: diff navigates to the right line, suggestion editor opens with a `` ```suggestion `` block
5. Verify: suggested code is syntactically valid and references correct types

**Edge cases:**
- Press `A` on a trivial hunk (e.g., import change) — agent should respond briefly, not over-analyze
- Press `p` immediately after opening panel with no conversation — draft uses the initial analysis
- Open panel, `esc` mid-stream — partial response displayed, `p` still produces a draft from partial content
- Open panel on a deleted file — agent correctly references the old content
- Open panel on a renamed file — agent understands it's a rename, not a new file
- Try `p` when agent proposes a location outside the diff — error shown, editor does not open

**Disabled state:**
- With `ai.enabled = false`: pressing `a`/`A` does nothing, no AI UI elements visible
- With `claude` not installed: same as disabled, no error on startup
