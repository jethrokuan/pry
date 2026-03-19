package diffview

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// inputMode represents the current input mode of the diffview.
type inputMode int

const (
	modeNormal inputMode = iota
	modeGoto
	modeSearch
	modeFilter
	modePendingG
	modePendingD
	modeHelp
	modeCommentPopup
	modeCommentSelect
)

// activeMode returns the current input mode based on model state.
// Mode priority mirrors the original handleKey cascade: text-input modes
// first, then overlay modes, then normal.
func (m Model) activeMode() inputMode {
	switch {
	case m.search.gotoActive:
		return modeGoto
	case m.search.active:
		return modeSearch
	case m.search.filterActive:
		return modeFilter
	case m.nav.pendingG:
		return modePendingG
	case m.nav.pendingD:
		return modePendingD
	case m.showHelp:
		return modeHelp
	case m.comments.popupActive:
		return modeCommentPopup
	case m.comments.cursor >= 0 && m.nav.focus == FocusDiff:
		return modeCommentSelect
	default:
		return modeNormal
	}
}

// handleKey dispatches a key event to the appropriate mode handler.
func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch m.activeMode() {
	case modeGoto:
		return m.handleGotoKey(msg)
	case modeSearch:
		return m.handleSearchKey(msg)
	case modeFilter:
		return m.handleFilterKey(msg)
	case modePendingG:
		return m.handlePendingG(msg)
	case modePendingD:
		return m.handlePendingD(msg)
	case modeHelp:
		m.showHelp = false
		return m, nil
	case modeCommentPopup:
		return m.handleCommentPopupKey(msg)
	case modeCommentSelect:
		if m, cmd, handled := m.handleCommentSelectKey(msg); handled {
			return m, cmd
		}
		// Unhandled keys deselect and fall through to normal
	}

	return m.handleNormalKey(msg)
}

// handleCommentSelectKey handles keys when a comment is selected in the diff.
// Returns handled=true if the key was consumed, false to fall through to normal mode.
func (m Model) handleCommentSelectKey(msg tea.KeyMsg) (Model, tea.Cmd, bool) {
	switch {
	case key.Matches(msg, keys.Up):
		if m.comments.cursor > 0 {
			m.comments.cursor--
			m.updateDiffContent()
			return m, nil, true
		}
		m.comments.cursor = -1
		m.updateDiffContent()
		return m, nil, true
	case key.Matches(msg, keys.Down):
		refs := m.commentRefsAtCursor()
		if m.comments.cursor < len(refs)-1 {
			m.comments.cursor++
			m.updateDiffContent()
			return m, nil, true
		}
		// Past last comment → move to next diff line
		m.comments.cursor = -1
		if m.nav.diffCursor < len(m.nav.diffLines)-1 {
			m.nav.diffCursor++
			m.syncViewportToCursor()
		}
		return m, nil, true
	case key.Matches(msg, keys.Back):
		m.comments.cursor = -1
		m.updateDiffContent()
		return m, nil, true
	}

	switch {
	case key.Matches(msg, keys.Reply):
		m.comments.cursor = -1
		return m, m.startComment(), true
	case key.Matches(msg, keys.EditComment):
		newM, cmd := m.editSelectedComment()
		return newM, cmd, true
	case key.Matches(msg, keys.DeleteComment):
		newM, cmd := m.deleteSelectedComment()
		return newM, cmd, true
	}

	// Unhandled: deselect comment and fall through
	m.comments.cursor = -1
	return m, nil, false
}

// handleNormalKey handles keys in normal mode (no active input/overlay).
// It processes shared bindings first, then delegates to focus-specific handlers.
func (m Model) handleNormalKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	// Reset confirmQuit on any non-quit key
	if !key.Matches(msg, keys.Quit) {
		m.confirmQuit = false
	}

	switch {
	case key.Matches(msg, keys.Quit):
		if !m.confirmQuit && m.hasUnsavedWork() {
			m.confirmQuit = true
			return m, nil
		}
		return m, tea.Quit
	case key.Matches(msg, keys.Back):
		if m.search.query != "" {
			m.search.query = ""
			m.updateDiffContent()
			return m, nil
		}
		if m.nav.visualMode {
			m.nav.visualMode = false
			m.updateDiffContent()
			return m, nil
		}
		return m, func() tea.Msg { return BackMsg{} }

	case key.Matches(msg, keys.ToggleTree):
		if m.nav.showTree && m.nav.focus != FocusFileTree {
			m.nav.focus = FocusFileTree
			m.nav.syncTreeCursorToFileCursor()
			m.updateDiffContent()
		} else if m.nav.showTree && m.nav.focus == FocusFileTree {
			m.nav.focus = FocusDiff
			m.updateDiffContent()
		} else {
			m.nav.showTree = true
			m.nav.focus = FocusFileTree
			m.nav.syncTreeCursorToFileCursor()
			m.updateViewports()
		}
		return m, nil

	case key.Matches(msg, keys.SelectLine):
		if m.nav.focus == FocusDiff {
			if !m.nav.visualMode {
				m.nav.visualMode = true
				m.nav.visualStart = m.nav.diffCursor
				m.nav.visualEnd = m.nav.diffCursor
			} else {
				m.nav.visualEnd = m.nav.diffCursor
			}
			m.updateDiffContent()
		}
		return m, nil

	case key.Matches(msg, keys.ToggleComment):
		if m.nav.focus == FocusFileTree {
			return m.toggleFoldAtCursor(), nil
		}
		return m.toggleCommentAtCursor(), nil

	case key.Matches(msg, keys.FoldComment):
		if m.nav.focus == FocusFileTree {
			return m.toggleFoldAll(), nil
		}
		return m.toggleAllComments(), nil

	case key.Matches(msg, keys.MarkViewed):
		if m.nav.focus == FocusFileTree {
			return m.markTreeItemViewed()
		}
		return m.markCurrentFileViewed()

	case key.Matches(msg, keys.OpenInBrowser):
		url := m.pr.URL + "/changes"
		return m, openBrowser(url)

	case key.Matches(msg, keys.Editor):
		return m, m.openInEditor()

	case key.Matches(msg, keys.Submit):
		return m, func() tea.Msg { return SubmitReviewMsg{} }
	}

	if m.nav.focus == FocusFileTree {
		return m.handleTreeKey(msg)
	}
	return m.handleDiffKey(msg)
}
