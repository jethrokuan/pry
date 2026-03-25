package diffview

// CursorKind distinguishes what the cursor is currently pointing at.
type CursorKind int

const (
	CursorLine    CursorKind = iota // cursor is on a diff line
	CursorComment                   // cursor is on a comment within an expanded comment block
)

// CursorTarget represents the current selection in the diff view.
// It replaces the previous dual-cursor model (diffCursor + comments.cursor)
// with a single tagged value that explicitly represents what is selected.
type CursorTarget struct {
	Kind       CursorKind
	FileIdx    int // index into the files slice
	LineIdx    int // index into diffLines
	CommentIdx int // index into comment refs at LineIdx (only meaningful when Kind == CursorComment)
}

// AsLine returns a copy of this cursor reset to line mode, preserving LineIdx.
func (c CursorTarget) AsLine() CursorTarget {
	return CursorTarget{Kind: CursorLine, FileIdx: c.FileIdx, LineIdx: c.LineIdx}
}

// IsComment returns true if the cursor is selecting a comment.
func (c CursorTarget) IsComment() bool {
	return c.Kind == CursorComment
}
