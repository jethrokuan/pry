package diffview

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/jethrokuan/pry/internal/ui/styles"
)

// --- SearchBar outbound messages ---

// searchGotoLineMsg is emitted when the user confirms a goto-line input.
type searchGotoLineMsg struct{ line int }

// searchQueryConfirmedMsg is emitted when the user confirms a search query.
type searchQueryConfirmedMsg struct{ query string }

// searchDismissedMsg is emitted when the user cancels search without confirming.
type searchDismissedMsg struct{}

// searchFilterSelectedMsg is emitted when the user selects a file from the filter list.
type searchFilterSelectedMsg struct{ fileIdx int }

// searchFilterDismissedMsg is emitted when the user cancels the file filter.
type searchFilterDismissedMsg struct{}

// --- SearchBar ---

// SearchBar manages search, goto-line, and file filter input modes.
type SearchBar struct {
	// Search
	active bool   // actively typing in search
	query  string // confirmed query (persists for highlighting)
	input  string // current input while typing

	// Goto line
	gotoActive bool
	gotoInput  string

	// File filter
	filterActive bool
	filterInput  string
	filterFiles  []int // indices into files matching filter
	filterCursor int

	// file names for filter matching (set by parent)
	fileNames []string
	// includedFn checks whether a file index passes the active narrowing filters.
	// Set by parent via SetIncludedFn.
	includedFn func(int) bool
}

// Query returns the confirmed search query (used for diff highlighting).
func (s SearchBar) Query() string { return s.query }

// ClearQuery clears the confirmed search query.
func (s *SearchBar) ClearQuery() { s.query = "" }

// IsActive returns true if any SearchBar input mode is active.
func (s SearchBar) IsActive() bool {
	return s.active || s.gotoActive || s.filterActive
}

// ActivateSearch opens the search input mode.
func (s *SearchBar) ActivateSearch() {
	s.active = true
	s.input = ""
}

// ActivateGoto opens the goto-line input mode.
func (s *SearchBar) ActivateGoto() {
	s.gotoActive = true
	s.gotoInput = ""
}

// ActivateFilter opens the file filter input mode.
// fileNames are the display names for each file index; includedFn
// filters out files excluded by narrowing.
func (s *SearchBar) ActivateFilter(fileNames []string, includedFn func(int) bool) {
	s.filterActive = true
	s.filterInput = ""
	s.fileNames = fileNames
	s.includedFn = includedFn
	s.recomputeFilterFiles()
}

// SetIncludedFn updates the narrowing filter used for file filter mode.
func (s *SearchBar) SetIncludedFn(fn func(int) bool) {
	s.includedFn = fn
}

// HandleKey processes a key event while one of the SearchBar modes is active.
// Returns the updated SearchBar and an optional outbound message (nil if none).
// The caller should only call this when s.IsActive() is true.
func (s SearchBar) HandleKey(keyStr string, text string) (SearchBar, any) {
	switch {
	case s.gotoActive:
		return s.handleGotoKey(keyStr)
	case s.active:
		return s.handleSearchKey(keyStr, text)
	case s.filterActive:
		return s.handleFilterKey(keyStr, text)
	}
	return s, nil
}

func (s SearchBar) handleGotoKey(keyStr string) (SearchBar, any) {
	switch keyStr {
	case "enter":
		var msg any
		if s.gotoInput != "" {
			if lineNum, err := strconv.Atoi(s.gotoInput); err == nil {
				msg = searchGotoLineMsg{line: lineNum}
			}
		}
		s.gotoActive = false
		s.gotoInput = ""
		return s, msg
	case "esc", "ctrl+c":
		s.gotoActive = false
		s.gotoInput = ""
		return s, nil
	case "backspace":
		if len(s.gotoInput) > 0 {
			s.gotoInput = s.gotoInput[:len(s.gotoInput)-1]
		}
		return s, nil
	default:
		if len(keyStr) == 1 && keyStr[0] >= '0' && keyStr[0] <= '9' {
			s.gotoInput += keyStr
		}
		return s, nil
	}
}

func (s SearchBar) handleSearchKey(keyStr string, text string) (SearchBar, any) {
	switch keyStr {
	case "enter":
		s.query = s.input
		s.active = false
		if s.query != "" {
			return s, searchQueryConfirmedMsg{query: s.query}
		}
		return s, searchDismissedMsg{}
	case "esc", "ctrl+c":
		s.active = false
		s.input = ""
		return s, searchDismissedMsg{}
	case "backspace":
		if len(s.input) > 0 {
			s.input = s.input[:len(s.input)-1]
		}
		return s, nil
	default:
		if text != "" {
			s.input += text
		}
		return s, nil
	}
}

func (s SearchBar) handleFilterKey(keyStr string, text string) (SearchBar, any) {
	switch keyStr {
	case "enter":
		var msg any
		if len(s.filterFiles) > 0 && s.filterCursor < len(s.filterFiles) {
			msg = searchFilterSelectedMsg{fileIdx: s.filterFiles[s.filterCursor]}
		}
		s.filterActive = false
		s.filterInput = ""
		if msg != nil {
			return s, msg
		}
		return s, searchFilterDismissedMsg{}
	case "esc", "ctrl+c":
		s.filterActive = false
		s.filterInput = ""
		return s, searchFilterDismissedMsg{}
	case "up":
		if s.filterCursor > 0 {
			s.filterCursor--
		}
		return s, nil
	case "down":
		if s.filterCursor < len(s.filterFiles)-1 {
			s.filterCursor++
		}
		return s, nil
	case "backspace":
		if len(s.filterInput) > 0 {
			s.filterInput = s.filterInput[:len(s.filterInput)-1]
			s.recomputeFilterFiles()
		}
		return s, nil
	default:
		if text != "" {
			s.filterInput += text
			s.recomputeFilterFiles()
		}
		return s, nil
	}
}

func (s *SearchBar) recomputeFilterFiles() {
	s.filterFiles = nil
	s.filterCursor = 0
	query := strings.ToLower(s.filterInput)
	for i, name := range s.fileNames {
		if s.includedFn != nil && !s.includedFn(i) {
			continue
		}
		if query == "" || strings.Contains(strings.ToLower(name), query) {
			s.filterFiles = append(s.filterFiles, i)
		}
	}
}

// View renders the SearchBar footer when one of its modes is active.
// Returns empty string if no mode is active.
func (s SearchBar) View() string {
	switch {
	case s.gotoActive:
		prompt := lipgloss.NewStyle().Bold(true).Render("Go to line: ")
		return prompt + s.gotoInput + "█"
	case s.active:
		prompt := lipgloss.NewStyle().Bold(true).Render("/")
		return prompt + s.input + "█"
	case s.filterActive:
		var b strings.Builder
		prompt := lipgloss.NewStyle().Bold(true).Render("Filter files: ")
		b.WriteString(prompt + s.filterInput + "█\n")
		for i, idx := range s.filterFiles {
			if i >= 10 {
				b.WriteString(styles.HelpStyle.Render(
					fmt.Sprintf("  ... and %d more", len(s.filterFiles)-10)))
				break
			}
			cursor := "  "
			if i == s.filterCursor {
				cursor = "> "
			}
			if idx < len(s.fileNames) {
				b.WriteString(cursor + s.fileNames[idx] + "\n")
			}
		}
		return b.String()
	}
	return ""
}
