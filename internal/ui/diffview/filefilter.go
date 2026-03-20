package diffview

import (
	"os/exec"
	"regexp"
	"strings"

	"github.com/jkuan/pr-review/internal/codeowners"
	"github.com/jkuan/pr-review/internal/diff"
)

// FileFilter manages stackable file tree narrowing filters.
type FileFilter struct {
	// Regex path filter
	regexPattern  string         // current active regex pattern (empty = disabled)
	regexCompiled *regexp.Regexp // compiled regex (nil if disabled or invalid)
	regexActive   bool           // input mode active
	regexInput    string         // current input while typing

	// Owner filter (CODEOWNERS-based)
	ownerIdentities []string              // e.g. ["@username", "@org/team1"] (empty = not configured)
	ownerEnabled    bool                  // whether owner filter is active
	codeowners      *codeowners.Codeowners // parsed CODEOWNERS (nil if not found)

	// Computed state
	includedFiles map[int]bool // set of file indices that pass all filters
	totalFiles    int          // total file count (for display)
	filteredCount int          // number of files passing filters
}

// loadCodeowners finds and parses the CODEOWNERS file from the git repo root.
func loadCodeowners() *codeowners.Codeowners {
	root, err := gitRepoRoot()
	if err != nil {
		return nil
	}
	return codeowners.Find(root)
}

// gitRepoRoot returns the root directory of the current git repository.
func gitRepoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// recompute recalculates includedFiles based on all active filters.
func (ff *FileFilter) recompute(files []diff.DiffFile) {
	ff.totalFiles = len(files)

	if !ff.isActive() {
		ff.includedFiles = nil
		ff.filteredCount = len(files)
		return
	}

	ff.includedFiles = make(map[int]bool, len(files))
	for i, f := range files {
		if ff.matchesAll(f.Path) {
			ff.includedFiles[i] = true
		}
	}
	ff.filteredCount = len(ff.includedFiles)
}

// matchesAll returns true if the path passes all active filters.
func (ff *FileFilter) matchesAll(path string) bool {
	if ff.regexCompiled != nil {
		if !ff.regexCompiled.MatchString(path) {
			return false
		}
	}
	if ff.ownerEnabled && len(ff.ownerIdentities) > 0 {
		if ff.codeowners == nil {
			return false // can't determine ownership without CODEOWNERS
		}
		if !ff.codeowners.OwnedByAny(path, ff.ownerIdentities) {
			return false
		}
	}
	return true
}

// isActive returns true if any filter is currently narrowing the file tree.
func (ff *FileFilter) isActive() bool {
	return ff.regexCompiled != nil || (ff.ownerEnabled && len(ff.ownerIdentities) > 0)
}

// setRegex sets and compiles a regex pattern. Empty string disables the filter.
func (ff *FileFilter) setRegex(pattern string) {
	ff.regexPattern = pattern
	if pattern == "" {
		ff.regexCompiled = nil
		return
	}
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		// Invalid regex — treat as disabled
		ff.regexCompiled = nil
		return
	}
	ff.regexCompiled = compiled
}

// toggleOwner toggles the owner filter on/off and returns a label for the
// flash message. Errors (no config, missing CODEOWNERS) are communicated
// through the returned label so the caller can show them.
func (ff *FileFilter) toggleOwner() string {
	if len(ff.ownerIdentities) == 0 {
		return "not available (user identity not loaded)"
	}
	// Toggling off is always allowed
	if ff.ownerEnabled {
		ff.ownerEnabled = false
		return "off"
	}
	// Toggling on — need CODEOWNERS
	if ff.codeowners == nil {
		ff.codeowners = loadCodeowners()
	}
	if ff.codeowners == nil {
		return "CODEOWNERS file not found"
	}
	ff.ownerEnabled = true
	return strings.Join(ff.ownerIdentities, ", ")
}

// clearAll removes all active filters.
func (ff *FileFilter) clearAll() {
	ff.regexPattern = ""
	ff.regexCompiled = nil
	ff.ownerEnabled = false
}

// statusText returns a short description of active filters for display.
func (ff *FileFilter) statusText() string {
	var parts []string
	if ff.regexCompiled != nil {
		parts = append(parts, "regex:"+ff.regexPattern)
	}
	if ff.ownerEnabled && len(ff.ownerIdentities) > 0 {
		parts = append(parts, "owner:"+ff.ownerIdentities[0])
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " + ")
}

// isIncluded returns true if the given file index passes all filters.
// If no filters are active, all files are included.
func (ff *FileFilter) isIncluded(idx int) bool {
	if !ff.isActive() {
		return true
	}
	return ff.includedFiles[idx]
}
