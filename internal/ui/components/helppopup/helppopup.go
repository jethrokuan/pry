// Package helppopup renders a keybinding help overlay popup.
package helppopup

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"

	"github.com/jethrokuan/pry/internal/ui/styles"
)

// Entry is a single key+description pair for the help popup.
type Entry struct {
	Key  string
	Desc string
}

// FromBinding creates an Entry from a key.Binding's help text.
func FromBinding(b key.Binding) Entry {
	h := b.Help()
	return Entry{Key: h.Key, Desc: h.Desc}
}

// Section groups related key bindings under a title.
type Section struct {
	Title   string
	Entries []Entry
}

// Bind is a convenience to create a Section from key.Bindings.
func Bind(title string, bindings ...key.Binding) Section {
	entries := make([]Entry, len(bindings))
	for i, b := range bindings {
		entries[i] = FromBinding(b)
	}
	return Section{Title: title, Entries: entries}
}

// Render builds the help popup box from the given sections.
// Sections are laid out in multiple columns when there's enough width.
func Render(sections []Section, termWidth int) string {
	bg := styles.BgOverlay
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.BrightWhite).Background(bg)
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.BrightCyan).Background(bg)
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.White).Background(bg)
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.BrightYellow).Background(bg)
	footerStyle := lipgloss.NewStyle().Foreground(lipgloss.BrightBlack).Background(bg)
	bgStyle := lipgloss.NewStyle().Background(bg)

	// Render each section as lines and measure width
	type block struct {
		lines []string
		width int
	}

	var blocks []block
	for _, section := range sections {
		var lines []string
		lines = append(lines, sectionStyle.Render(section.Title))
		for _, e := range section.Entries {
			line := keyStyle.Render(fmt.Sprintf("%-16s", e.Key)) + descStyle.Render(" "+e.Desc)
			lines = append(lines, line)
		}
		w := 0
		for _, l := range lines {
			if lw := lipgloss.Width(l); lw > w {
				w = lw
			}
		}
		blocks = append(blocks, block{lines: lines, width: w})
	}

	// Determine column layout
	colGap := 4
	padding := 8 // border + padding
	availWidth := termWidth - padding
	if availWidth < 40 {
		availWidth = 40
	}

	// Calculate total height for single column
	totalHeight := 0
	for _, blk := range blocks {
		totalHeight += len(blk.lines) + 1 // +1 spacing between sections
	}

	// Try 2 columns if tall enough
	type column struct {
		blocks []block
		width  int
		height int
	}

	buildColumns := func(splitAt int) []column {
		col1 := column{}
		col2 := column{}
		for i, blk := range blocks {
			if i < splitAt {
				col1.blocks = append(col1.blocks, blk)
				if blk.width > col1.width {
					col1.width = blk.width
				}
				col1.height += len(blk.lines) + 1
			} else {
				col2.blocks = append(col2.blocks, blk)
				if blk.width > col2.width {
					col2.width = blk.width
				}
				col2.height += len(blk.lines) + 1
			}
		}
		return []column{col1, col2}
	}

	var columns []column
	if totalHeight > 16 && len(blocks) >= 4 {
		// Try splitting at different points to find the most balanced split
		bestSplit := len(blocks) / 2
		bestDiff := totalHeight
		for i := 1; i < len(blocks); i++ {
			cols := buildColumns(i)
			if cols[0].width+colGap+cols[1].width > availWidth {
				continue
			}
			diff := cols[0].height - cols[1].height
			if diff < 0 {
				diff = -diff
			}
			if diff < bestDiff {
				bestDiff = diff
				bestSplit = i
			}
		}
		cols := buildColumns(bestSplit)
		if cols[0].width+colGap+cols[1].width <= availWidth {
			columns = cols
		}
	}

	// Fall back to single column
	if len(columns) == 0 {
		col := column{}
		for _, blk := range blocks {
			col.blocks = append(col.blocks, blk)
			if blk.width > col.width {
				col.width = blk.width
			}
			col.height += len(blk.lines) + 1
		}
		columns = []column{col}
	}

	// Collect lines per column
	colLines := func(col column) []string {
		var allLines []string
		for i, blk := range col.blocks {
			if i > 0 {
				allLines = append(allLines, "")
			}
			allLines = append(allLines, blk.lines...)
		}
		return allLines
	}

	var body string
	if len(columns) == 1 {
		body = strings.Join(colLines(columns[0]), "\n")
	} else {
		// Build line by line so background is consistent
		leftLines := colLines(columns[0])
		rightLines := colLines(columns[1])
		maxLines := len(leftLines)
		if len(rightLines) > maxLines {
			maxLines = len(rightLines)
		}
		lw := columns[0].width
		gap := strings.Repeat(" ", colGap)
		var rows []string
		for i := range maxLines {
			left := ""
			if i < len(leftLines) {
				left = leftLines[i]
			}
			right := ""
			if i < len(rightLines) {
				right = rightLines[i]
			}
			// Pad left column to fixed width
			leftPad := lw - lipgloss.Width(left)
			if leftPad < 0 {
				leftPad = 0
			}
			row := left + bgStyle.Render(strings.Repeat(" ", leftPad)+gap) + right
			rows = append(rows, row)
		}
		body = strings.Join(rows, "\n")
	}

	content := titleStyle.Render("Keybindings") + "\n\n" +
		body + "\n\n" +
		footerStyle.Render("Press any key to close")

	contentWidth := 0
	for _, line := range strings.Split(content, "\n") {
		if w := lipgloss.Width(line); w > contentWidth {
			contentWidth = w
		}
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.BrightBlack).
		Background(bg).
		Padding(1, 2).
		Width(contentWidth + 4)

	return boxStyle.Render(content)
}

// Overlay places the popup centered over the base string using Canvas compositing.
func Overlay(base string, popup string, width, height int) string {
	popupW := lipgloss.Width(popup)
	popupH := lipgloss.Height(popup)

	x := (width - popupW) / 2
	if x < 0 {
		x = 0
	}
	y := (height - popupH) / 2
	if y < 0 {
		y = 0
	}

	baseLayer := lipgloss.NewLayer(base)
	popupLayer := lipgloss.NewLayer(popup).X(x).Y(y).Z(1)
	return lipgloss.NewCompositor(baseLayer, popupLayer).Render()
}
