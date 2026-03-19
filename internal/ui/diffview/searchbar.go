package diffview

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
}
