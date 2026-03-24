package diffview

import (
	"fmt"

	"charm.land/bubbles/v2/viewport"

	"github.com/jethrokuan/pry/internal/review"
)

// CommentPanel manages comment display state and comment CRUD operations.
type CommentPanel struct {
	comments []review.Comment   // all comments (existing + pending)
	expanded map[string]bool    // per-line expanded state (key: "path:line")

	// Indexed comment lookups — rebuilt on mutation via RebuildIndex().
	// Key format: "path:line:side" (side may be "" for comments with no side).
	commentIndex     map[string][]review.Comment
	fileCommentIndex map[string]bool             // files that have any comments
	popupActive      bool                        // comment popup overlay is open
	popupViewport    viewport.Model              // scrollable viewport for popup
}

// --- Index management ---

// RebuildIndex rebuilds the commentIndex from the current comment data.
// Called automatically by mutation methods.
func (cp *CommentPanel) RebuildIndex() {
	cp.commentIndex = make(map[string][]review.Comment)
	cp.fileCommentIndex = make(map[string]bool)
	for _, c := range cp.comments {
		k := commentIndexKey(c.Path, c.Line, c.Side)
		cp.commentIndex[k] = append(cp.commentIndex[k], c)
		cp.fileCommentIndex[c.Path] = true
	}
}

// --- Query methods ---

// CommentsForLine returns all comments for a given file path, line, and side.
func (cp *CommentPanel) CommentsForLine(path string, line int, side string) []review.Comment {
	exact := cp.commentIndex[commentIndexKey(path, line, side)]
	// Also include comments with empty side (they match any side query)
	if side != "" {
		emptySide := cp.commentIndex[commentIndexKey(path, line, "")]
		if len(emptySide) > 0 {
			result := make([]review.Comment, 0, len(exact)+len(emptySide))
			result = append(result, exact...)
			result = append(result, emptySide...)
			return result
		}
	}
	return exact
}

// LineHasComments returns true if the given diff line has any comments.
func (cp *CommentPanel) LineHasComments(path string, dl diffLineInfo) bool {
	if dl.newLine > 0 && len(cp.CommentsForLine(path, dl.newLine, "RIGHT")) > 0 {
		return true
	}
	if dl.oldLine > 0 && len(cp.CommentsForLine(path, dl.oldLine, "LEFT")) > 0 {
		return true
	}
	return false
}

// FileHasComments returns true if any comments exist for the given file path.
func (cp *CommentPanel) FileHasComments(path string) bool {
	return cp.fileCommentIndex[path]
}

func commentIndexKey(path string, line int, side string) string {
	return fmt.Sprintf("%s:%d:%s", path, line, side)
}
