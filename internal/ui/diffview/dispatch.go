package diffview

import (
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/jethrokuan/pry/internal/ui/components/flash"
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
	modeAIInput
	modeAIPanel
	modeCommitPicker
)

// activeMode returns the current input mode based on model state.
// Mode priority mirrors the original handleKey cascade: text-input modes
// first, then overlay modes, then normal.
func (m Model) activeMode() inputMode {
	switch {
	case m.aiPanel.IsInputActive():
		return modeAIInput
	case m.aiPanel.IsOpen():
		return modeAIPanel
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
	case m.commitPickerActive:
		return modeCommitPicker
	case m.showHelp:
		return modeHelp
	case m.prInfo.active:
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
	switch m.activeMode() {
	case modeAIInput:
		return m.handleAIInputKey(msg)
	case modeAIPanel:
		return m.handleAIPanelKey(msg)
	case modeGoto, modeSearch, modeFilter:
		return m.handleSearchBarKey(msg)
	case modeNarrowRegex:
		return m.handleNarrowRegexKey(msg)
	case modeNarrowPrefix:
		return m.handleNarrowPrefixKey(msg)
	case modeCommitPicker:
		return m.handleCommitPickerKey(msg)
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
			m.syncViewport()
			return m, nil, true
		}
		// At first comment in thread — try previous thread
		if m.nav.cursor.ThreadIdx > 0 {
			m.nav.cursor.ThreadIdx--
			threads := m.threadsAtCursor()
			if m.nav.cursor.ThreadIdx < len(threads) {
				m.nav.cursor.CommentIdx = len(threads[m.nav.cursor.ThreadIdx].Comments) - 1
			}
			m.syncViewport()
			return m, nil, true
		}
		m.nav.cursor = m.nav.cursor.AsLine()
		m.syncViewport()
		return m, nil, true
	case key.Matches(msg, keys.Down):
		threads := m.threadsAtCursor()
		if m.nav.cursor.ThreadIdx < len(threads) {
			t := threads[m.nav.cursor.ThreadIdx]
			if m.nav.cursor.CommentIdx < len(t.Comments)-1 {
				m.nav.cursor.CommentIdx++
				m.syncViewport()
				return m, nil, true
			}
			// Past last comment in thread — try next thread
			if m.nav.cursor.ThreadIdx < len(threads)-1 {
				m.nav.cursor.ThreadIdx++
				m.nav.cursor.CommentIdx = 0
				m.syncViewport()
				return m, nil, true
			}
		}
		// Past last thread → move to next diff line
		m.nav.cursor = m.nav.cursor.AsLine()
		if m.nav.cursor.LineIdx < len(m.nav.diffLines)-1 {
			m.nav.cursor.LineIdx++
			m.syncViewport()
		}
		return m, nil, true
	case key.Matches(msg, keys.Back):
		m.nav.cursor = m.nav.cursor.AsLine()
		m.updateDiffContent()
		return m, nil, true
	case key.Matches(msg, keys.Enter):
		newM, cmd := m.replyToSelectedThread()
		return newM, cmd, true
	}

	switch {
	case key.Matches(msg, keys.ViewComment):
		m.openCommentPopup()
		return m, nil, true
	case key.Matches(msg, keys.EditComment):
		newM, cmd := m.editSelectedComment()
		return newM, cmd, true
	case key.Matches(msg, keys.DeleteComment):
		c := m.selectedComment()
		if c != nil && c.IsPending && c.Author == m.currentUser {
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
				m.nav.visualStart = m.nav.cursor
				m.nav.visualEnd = m.nav.cursor
			} else {
				m.nav.visualEnd = m.nav.cursor
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

	case key.Matches(msg, keys.Checkout):
		return m, tea.Batch(
			flash.ShowMsg{ID: "checkout", Text: "Checking out " + m.pr.Branch + "…", Style: flash.StyleSpinner}.Cmd(),
			m.checkoutPR(),
		)

	case key.Matches(msg, keys.OpenInBrowser):
		url := m.pr.URL + "/changes"
		return m, openBrowser(url)

	case key.Matches(msg, keys.Editor):
		return m, m.openInEditor()

	case key.Matches(msg, keys.Info):
		m.openPRInfoPopup()
		if m.prInfo.issueComments == nil {
			return m, m.fetchIssueCommentsCmd()
		}
		return m, nil

	case key.Matches(msg, keys.NarrowPrefix):
		m.narrowPrefixActive = true
		return m, flash.ShowMsg{ID: "diffview", Text: "x: [o]wner [f]ilter [x]clear", Expires: 1500 * time.Millisecond}.Cmd()

	case key.Matches(msg, keys.JumpBack):
		return m.jumpBack()

	case key.Matches(msg, keys.JumpForward):
		return m.jumpForward()

	case key.Matches(msg, keys.Submit):
		return m, func() tea.Msg { return SubmitReviewMsg{} }

	case key.Matches(msg, keys.Refresh):
		if m.refreshing {
			return m, nil
		}
		m.refreshing = true
		m.commitStart = -1 // refresh resets to full PR diff
		m.commitEnd = -1
		return m, tea.Batch(
			flash.ShowMsg{ID: "refresh", Text: "Refreshing…", Style: flash.StyleSpinner}.Cmd(),
			m.refreshCmd(),
		)

	// Commit picker
	case key.Matches(msg, keys.CommitPicker):
		return m.openCommitPicker()

	// AI assistant
	case key.Matches(msg, keys.AIAsk):
		if !m.aiEnabled {
			return m, flash.ShowMsg{ID: "diffview", Text: "AI not available — install claude CLI (npm i -g @anthropic-ai/claude-code)", Expires: 3 * time.Second}.Cmd()
		}
		return m.toggleAIPanel()

	// Dedicated navigation keys (work in both tree and diff focus)
	case key.Matches(msg, keys.NextFile):
		m.nav.activeCycler = CyclerFile
		cmd := m.navigateFile(true)
		return m, cmd
	case key.Matches(msg, keys.PrevFile):
		m.nav.activeCycler = CyclerFile
		cmd := m.navigateFile(false)
		return m, cmd
	}

	if m.nav.focus == FocusFileTree {
		return m.handleTreeKey(msg)
	}
	return m.handleDiffKey(msg)
}
