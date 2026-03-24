package diffview

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/jethrokuan/pry/internal/ui/styles"
)

// errCategory identifies the source of an error in the diffview.
type errCategory int

const (
	errCatLoad   errCategory = iota // diff/comment loading failures
	errCatReview                    // pending review creation failures
	errCatViewed                    // viewed-file sync failures
)

// errorStore provides unified error tracking for the diffview.
// Errors are categorized and optionally keyed (e.g., by comment localID).
// Single-valued categories use key 0.
type errorStore struct {
	errs map[errCategory]map[int]error
}

func newErrorStore() errorStore {
	return errorStore{errs: make(map[errCategory]map[int]error)}
}

// set records an error under the given category and key.
// If err is nil, the entry is cleared instead.
func (s *errorStore) set(cat errCategory, key int, err error) {
	if err == nil {
		s.clear(cat, key)
		return
	}
	if s.errs[cat] == nil {
		s.errs[cat] = make(map[int]error)
	}
	s.errs[cat][key] = err
}

// clear removes a single keyed error. If the category becomes empty, it is deleted.
func (s *errorStore) clear(cat errCategory, key int) {
	if m, ok := s.errs[cat]; ok {
		delete(m, key)
		if len(m) == 0 {
			delete(s.errs, cat)
		}
	}
}

// get returns the single error for a category (key 0), or nil.
func (s *errorStore) get(cat errCategory) error {
	if m, ok := s.errs[cat]; ok {
		if err, ok := m[0]; ok {
			return err
		}
	}
	return nil
}

// count returns the number of errors in a category.
func (s *errorStore) count(cat errCategory) int {
	if m, ok := s.errs[cat]; ok {
		return len(m)
	}
	return 0
}

// renderSyncErrors renders non-load errors (review, viewed, comment sync)
// as styled error lines for display in the header area.
func (s *errorStore) renderSyncErrors() string {
	errStyle := lipgloss.NewStyle().Foreground(styles.Danger)
	var lines []string

	if err := s.get(errCatReview); err != nil {
		lines = append(lines, errStyle.Render(fmt.Sprintf("Review error: %v", err)))
	}
	if err := s.get(errCatViewed); err != nil {
		lines = append(lines, errStyle.Render(fmt.Sprintf("Viewed error: %v", err)))
	}


	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}
