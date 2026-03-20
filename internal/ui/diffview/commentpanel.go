package diffview

import (
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"

	"github.com/jkuan/pr-review/internal/review"
)

// CommentPanel manages comment display state, inline comment editor,
// and comment CRUD operations.
type CommentPanel struct {
	existing      []review.ExistingComment // comments already on the PR
	forgeComments []review.ExistingComment // from forge's pending review (crash recovery)
	expanded      map[string]bool          // per-line expanded state (key: "path:line")
	cursor        int                      // selected comment index (-1 = none, 0+ = index)

	// Indexed comment lookups — rebuilt on mutation via rebuildCommentIndex().
	// Key format: "path:line:side" (side may be "" for existing comments with no side).
	existingIndex     map[string][]review.ExistingComment // indexes existing + forgeComments
	localPendingIndex map[string][]review.InlineComment   // indexes review.Comments
	fileCommentIndex  map[string]bool                     // files that have any comments
	popupActive bool           // comment popup overlay is open
	popupViewport viewport.Model           // scrollable viewport for popup

	// Inline comment editor
	inlineActive      bool
	inlineTextarea    textarea.Model
	inlinePath        string
	inlineLine        int
	inlineStartLine   int
	inlineSide        string
	inlineMode        commentMode
	inlineSuggestion  string // original code for suggestion mode
	inlineEditLocalID int  // non-zero when editing an existing comment
	confirmDiscard    bool // true after first esc with unsaved content
}
