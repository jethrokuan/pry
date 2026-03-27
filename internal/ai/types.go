package ai

// StreamChunk represents a piece of streaming output from the agent.
type StreamChunk struct {
	Text   string // text content (may be empty for tool-use-only messages)
	Status string // tool status like "Reading handler.go..." (empty for text chunks)
}

// DraftMode is the type of draft to produce (comment or suggestion).
type DraftMode int

const (
	DraftComment    DraftMode = iota
	DraftSuggestion
)

// DraftResult is the structured output of the second-pass draft call.
type DraftResult struct {
	Path string `json:"path"` // file path within the PR
	Line int    `json:"line"` // line number in the diff
	Side string `json:"side"` // "RIGHT" (new code) or "LEFT" (old code)
	Body string `json:"body"` // the comment text, or suggestion block
}

// DiffFileSummary provides the agent with valid comment target ranges.
type DiffFileSummary struct {
	Path  string      `json:"path"`
	Hunks []HunkRange `json:"hunks"`
}

// HunkRange describes the line range of a single hunk.
type HunkRange struct {
	OldStart int `json:"old_start"`
	OldLines int `json:"old_lines"`
	NewStart int `json:"new_start"`
	NewLines int `json:"new_lines"`
}

// ConversationEntry is a single turn in the conversation transcript.
type ConversationEntry struct {
	Role string // "reviewer" or "assistant"
	Text string
}

// Action represents a structured action the agent wants pry to execute.
type Action struct {
	Action string `json:"action"` // "comment" or "suggest"
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Side   string `json:"side"` // "RIGHT" or "LEFT"
	Body   string `json:"body"`
}
