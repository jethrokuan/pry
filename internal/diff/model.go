package diff

// FileStatus represents the type of change to a file.
type FileStatus int

const (
	StatusModified FileStatus = iota
	StatusAdded
	StatusDeleted
	StatusRenamed
)

func (s FileStatus) String() string {
	switch s {
	case StatusModified:
		return "M"
	case StatusAdded:
		return "A"
	case StatusDeleted:
		return "D"
	case StatusRenamed:
		return "R"
	default:
		return "?"
	}
}

// LineType represents a diff line type.
type LineType int

const (
	LineContext  LineType = iota
	LineAddition
	LineDeletion
)

// DiffLine represents a single line in a diff.
type DiffLine struct {
	Type    LineType
	OldNum  int    // 0 if addition
	NewNum  int    // 0 if deletion
	Content string
}

// Hunk represents a diff hunk.
type Hunk struct {
	OldStart int
	OldLines int
	NewStart int
	NewLines int
	Header   string
	Lines    []DiffLine
}

// DiffFile represents all changes to a single file.
type DiffFile struct {
	Path      string
	OldPath   string // for renames
	Status    FileStatus
	Additions int
	Deletions int
	Hunks     []Hunk
	IsBinary  bool
}

// LinePosition maps a new-file line number to its diff position.
// The diff position is needed for the GitHub review comment API.
type LinePosition struct {
	NewLine  int
	OldLine  int
	Position int // 1-based position in the diff (for API)
}
