package diffview

import (
	"fmt"

	"charm.land/bubbles/v2/viewport"

	"github.com/jethrokuan/pry/internal/review"
)

// CommentPanel manages comment display state and thread CRUD operations.
type CommentPanel struct {
	threads  []review.Thread // all threads (existing + pending)
	expanded map[string]bool // per-line expanded state (key: "path:line")

	// Indexed thread lookups — rebuilt on mutation via RebuildIndex().
	// Key format: "path:line:side" (side may be "" for threads with no side).
	threadIndex     map[string][]review.Thread
	fileThreadIndex map[string]bool             // files that have any threads
	popupActive     bool                        // comment popup overlay is open
	popupViewport   viewport.Model              // scrollable viewport for popup
}

// --- Index management ---

// RebuildIndex rebuilds the threadIndex from the current thread data.
// Called automatically by mutation methods.
func (cp *CommentPanel) RebuildIndex() {
	cp.threadIndex = make(map[string][]review.Thread)
	cp.fileThreadIndex = make(map[string]bool)
	for _, t := range cp.threads {
		k := commentIndexKey(t.Path, t.Line, t.Side)
		cp.threadIndex[k] = append(cp.threadIndex[k], t)
		cp.fileThreadIndex[t.Path] = true
	}
}

// --- Query methods ---

// ThreadsForLine returns all threads for a given file path, line, and side.
func (cp *CommentPanel) ThreadsForLine(path string, line int, side string) []review.Thread {
	exact := cp.threadIndex[commentIndexKey(path, line, side)]
	// Also include threads with empty side (they match any side query)
	if side != "" {
		emptySide := cp.threadIndex[commentIndexKey(path, line, "")]
		if len(emptySide) > 0 {
			result := make([]review.Thread, 0, len(exact)+len(emptySide))
			result = append(result, exact...)
			result = append(result, emptySide...)
			return result
		}
	}
	return exact
}

// CommentsForLine returns a flat list of all comments across all threads
// for a given file path, line, and side. Convenience method for rendering.
func (cp *CommentPanel) CommentsForLine(path string, line int, side string) []review.Comment {
	threads := cp.ThreadsForLine(path, line, side)
	var result []review.Comment
	for _, t := range threads {
		result = append(result, t.Comments...)
	}
	return result
}

// LineHasComments returns true if the given diff line has any threads.
func (cp *CommentPanel) LineHasComments(path string, dl diffLineInfo) bool {
	if dl.newLine > 0 && len(cp.ThreadsForLine(path, dl.newLine, "RIGHT")) > 0 {
		return true
	}
	if dl.oldLine > 0 && len(cp.ThreadsForLine(path, dl.oldLine, "LEFT")) > 0 {
		return true
	}
	return false
}

// FileHasComments returns true if any threads exist for the given file path.
func (cp *CommentPanel) FileHasComments(path string) bool {
	return cp.fileThreadIndex[path]
}

func commentIndexKey(path string, line int, side string) string {
	return fmt.Sprintf("%s:%d:%s", path, line, side)
}
