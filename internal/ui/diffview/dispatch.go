package diffview

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// inputMode represents the current input mode of the diffview.
type inputMode int

const (
	modeNormal inputMode = iota
	modeGoto
	modeSearch
	modeFilter
	modeNarrowRegex
	modeNarrowPrefix
	modeHelp
	modePRInfo
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
	case m.filter.regexActive:
		return modeNarrowRegex
	case m.narrowPrefixActive:
		return modeNarrowPrefix
	case m.showHelp:
		return modeHelp
	case m.prInfoActive:
		return modePRInfo
	case m.comments.popupActive:
		return modeCommentPopup
	case m.nav.cursor.IsComment() && m.nav.focus == FocusDiff:
		return modeCommentSelect
	default:
		return modeNormal
	}
}

// handleKey dispatches a key event to the appropriate mode handler.
func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	// Clear flash message on any keypress
	m.flashMsg = ""

	switch m.activeMode() {
	case modeGoto, modeSearch, modeFilter:
		return m.handleSearchBarKey(msg)
	case modeNarrowRegex:
		return m.handleNarrowRegexKey(msg)
	case modeNarrowPrefix:
		return m.handleNarrowPrefixKey(msg)
	case modeHelp:
		m.showHelp = false
		return m, nil
	case modePRInfo:
		return m.handlePRInfoKey(msg)
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
func (m Model) handleCommentSelectKey(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	// Handle delete confirmation prompt (y/n)
	if m.confirmDelete {
		switch msg.String() {
		case "y":
			m.confirmDelete = false
			newM, cmd := m.deleteSelectedComment()
			return newM, cmd, true
		default:
			// Any other key cancels
			m.confirmDelete = false
			return m, nil, true
		}
	}

	switch {
	case key.Matches(msg, keys.Up):
		if m.nav.cursor.CommentIdx > 0 {
			m.nav.cursor.CommentIdx--
			m.updateDiffContent()
			return m, nil, true
		}
		m.nav.cursor = m.nav.cursor.AsLine()
		m.updateDiffContent()
		return m, nil, true
	case key.Matches(msg, keys.Down):
		refs := m.commentRefsAtCursor()
		if m.nav.cursor.CommentIdx < len(refs)-1 {
			m.nav.cursor.CommentIdx++
			m.updateDiffContent()
			return m, nil, true
		}
		// Past last comment → move to next diff line
		m.nav.cursor = m.nav.cursor.AsLine()
		if m.nav.cursor.LineIdx < len(m.nav.diffLines)-1 {
			m.nav.cursor.LineIdx++
			m.syncViewportToCursor()
		}
		return m, nil, true
	case key.Matches(msg, keys.Back):
		m.nav.cursor = m.nav.cursor.AsLine()
		m.updateDiffContent()
		return m, nil, true
	case key.Matches(msg, keys.Enter):
		m.openCommentPopup()
		return m, nil, true
	}

	switch {
	case key.Matches(msg, keys.Reply):
		m.nav.cursor = m.nav.cursor.AsLine()
		return m, m.startComment(), true
	case key.Matches(msg, keys.EditComment):
		newM, cmd := m.editSelectedComment()
		return newM, cmd, true
	case key.Matches(msg, keys.DeleteComment):
		refs := m.commentRefsAtCursor()
		if m.nav.cursor.CommentIdx >= 0 && m.nav.cursor.CommentIdx < len(refs) && refs[m.nav.cursor.CommentIdx].editable {
			m.confirmDelete = true
			return m, nil, true
		}
		return m, nil, true
	}

	// Unhandled: deselect comment and fall through
	m.nav.cursor = m.nav.cursor.AsLine()
	return m, nil, false
}

// handleNormalKey handles keys in normal mode (no active input/overlay).
// It processes shared bindings first, then delegates to focus-specific handlers.
func (m Model) handleNormalKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
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
		if m.search.Query() != "" {
			m.search.ClearQuery()
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
				m.nav.visualStart = m.nav.cursor.LineIdx
				m.nav.visualEnd = m.nav.cursor.LineIdx
			} else {
				m.nav.visualEnd = m.nav.cursor.LineIdx
			}
			m.updateDiffContent()
		}
		return m, nil

	case key.Matches(msg, keys.ToggleComment):
		if m.nav.focus == FocusFileTree {
			return m.toggleFoldAtCursor(), nil
		}
		return m.toggleFoldAtDiffCursor(), nil

	case key.Matches(msg, keys.FoldComment):
		if m.nav.focus == FocusFileTree {
			return m.toggleFoldAll(), nil
		}
		return m.toggleAllFolds(), nil

	case key.Matches(msg, keys.MarkViewed):
		if m.nav.focus == FocusFileTree {
			return m.markTreeItemViewed()
		}
		return m.markCurrentFileViewed()

	case key.Matches(msg, keys.CopyLink):
		return m, m.copyForgeLink()

	case key.Matches(msg, keys.OpenInBrowser):
		url := m.pr.URL + "/changes"
		return m, openBrowser(url)

	case key.Matches(msg, keys.Editor):
		return m, m.openInEditor()

	case key.Matches(msg, keys.Info):
		m.openPRInfoPopup()
		return m, nil

	case key.Matches(msg, keys.NarrowPrefix):
		m.narrowPrefixActive = true
		m.flashMsg = "T: [o]wner [f]ilter [x]clear"
		return m, nil

	case key.Matches(msg, keys.JumpBack):
		return m.jumpBack()

	case key.Matches(msg, keys.JumpForward):
		return m.jumpForward()

	case key.Matches(msg, keys.Submit):
		return m, func() tea.Msg { return SubmitReviewMsg{} }

	// Dedicated navigation keys (work in both tree and diff focus)
	case key.Matches(msg, keys.NextFile):
		m.nav.activeCycler = 'f'
		cmd := m.navigateFile(true, false)
		return m, cmd
	case key.Matches(msg, keys.PrevFile):
		m.nav.activeCycler = 'f'
		cmd := m.navigateFile(false, false)
		return m, cmd
	}

	if m.nav.focus == FocusFileTree {
		return m.handleTreeKey(msg)
	}
	return m.handleDiffKey(msg)
}
