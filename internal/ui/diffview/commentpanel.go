package diffview

import (
	"fmt"

	"charm.land/bubbles/v2/viewport"

	"github.com/jethrokuan/pry/internal/review"
)

// CommentPanel manages comment display state and comment CRUD operations.
type CommentPanel struct {
	existing      []review.ExistingComment // comments already on the PR
	forgeComments []review.ExistingComment // from forge's pending review (crash recovery)
	expanded      map[string]bool          // per-line expanded state (key: "path:line")

	// Indexed comment lookups — rebuilt on mutation via RebuildIndex().
	// Key format: "path:line:side" (side may be "" for existing comments with no side).
	existingIndex     map[string][]review.ExistingComment // indexes existing + forgeComments
	localPendingIndex map[string][]review.InlineComment   // indexes review.Comments
	fileCommentIndex  map[string]bool                     // files that have any comments
	popupActive       bool                                // comment popup overlay is open
	popupViewport     viewport.Model                      // scrollable viewport for popup
}

// --- Index management ---

// RebuildIndex rebuilds the existingIndex and localPendingIndex maps
// from the current comment data. Called automatically by mutation methods.
func (cp *CommentPanel) RebuildIndex(pendingComments []review.InlineComment) {
	cp.existingIndex = make(map[string][]review.ExistingComment)
	cp.fileCommentIndex = make(map[string]bool)
	for _, c := range cp.existing {
		k := commentIndexKey(c.Path, c.Line, c.Side)
		cp.existingIndex[k] = append(cp.existingIndex[k], c)
		cp.fileCommentIndex[c.Path] = true
	}
	for _, c := range cp.forgeComments {
		k := commentIndexKey(c.Path, c.Line, c.Side)
		cp.existingIndex[k] = append(cp.existingIndex[k], c)
		cp.fileCommentIndex[c.Path] = true
	}

	cp.localPendingIndex = make(map[string][]review.InlineComment)
	for _, c := range pendingComments {
		k := commentIndexKey(c.Path, c.Line, c.Side)
		cp.localPendingIndex[k] = append(cp.localPendingIndex[k], c)
		cp.fileCommentIndex[c.Path] = true
	}
}

// --- Query methods ---

// CommentsForLine returns existing comments for a given file path, line, and side.
func (cp *CommentPanel) CommentsForLine(path string, line int, side string) []review.ExistingComment {
	exact := cp.existingIndex[commentIndexKey(path, line, side)]
	// Also include comments with empty side (they match any side query)
	if side != "" {
		emptySide := cp.existingIndex[commentIndexKey(path, line, "")]
		if len(emptySide) > 0 {
			result := make([]review.ExistingComment, 0, len(exact)+len(emptySide))
			result = append(result, exact...)
			result = append(result, emptySide...)
			return result
		}
	}
	return exact
}

// LocalPendingForLine returns local pending comments for a given file path, line, and side.
func (cp *CommentPanel) LocalPendingForLine(path string, line int, side string) []review.InlineComment {
	return cp.localPendingIndex[commentIndexKey(path, line, side)]
}

// LineHasComments returns true if the given diff line has any comments (existing or pending).
func (cp *CommentPanel) LineHasComments(path string, dl diffLineInfo) bool {
	if dl.newLine > 0 && len(cp.CommentsForLine(path, dl.newLine, "RIGHT"))+len(cp.LocalPendingForLine(path, dl.newLine, "RIGHT")) > 0 {
		return true
	}
	if dl.oldLine > 0 && len(cp.CommentsForLine(path, dl.oldLine, "LEFT"))+len(cp.LocalPendingForLine(path, dl.oldLine, "LEFT")) > 0 {
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
