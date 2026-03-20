package diff

import (
	"fmt"
	"os/exec"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/jkuan/pr-review/internal/ui/styles"
)

// HasDelta checks if the delta binary is available.
func HasDelta() bool {
	_, err := exec.LookPath("delta")
	return err == nil
}

// RenderWithDelta pipes a raw diff through delta for syntax-highlighted output.
func RenderWithDelta(rawDiff string, width int) (string, error) {
	cmd := exec.Command("delta", "--paging=never", fmt.Sprintf("--width=%d", width))
	cmd.Stdin = strings.NewReader(rawDiff)

	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return string(out), nil
}

// RenderFile renders a DiffFile with built-in ANSI coloring.
func RenderFile(file *DiffFile, width int) string {
	if file.IsBinary {
		return "Binary file changed"
	}

	var b strings.Builder

	for i, hunk := range file.Hunks {
		if i > 0 {
			b.WriteString("\n")
		}

		// Hunk header
		header := fmt.Sprintf("@@ -%d,%d +%d,%d @@", hunk.OldStart, hunk.OldLines, hunk.NewStart, hunk.NewLines)
		if hunk.Header != "" {
			header += " " + hunk.Header
		}
		b.WriteString(styles.HunkHeader.Render(header))
		b.WriteString("\n")

		// Lines
		lineNumStyle := lipgloss.NewStyle().Foreground(styles.Muted)
		for _, line := range hunk.Lines {
			var num int
			switch line.Type {
			case LineDeletion:
				num = line.OldNum
			default:
				num = line.NewNum
			}

			numStr := "    "
			if num > 0 {
				numStr = fmt.Sprintf("%4d", num)
			}

			lineNums := lineNumStyle.Render(fmt.Sprintf("%s │", numStr))

			switch line.Type {
			case LineAddition:
				b.WriteString(fmt.Sprintf("%s %s\n", lineNums, styles.Addition.Render("+"+line.Content)))
			case LineDeletion:
				b.WriteString(fmt.Sprintf("%s %s\n", lineNums, styles.Deletion.Render("-"+line.Content)))
			case LineContext:
				b.WriteString(fmt.Sprintf("%s  %s\n", lineNums, line.Content))
			}
		}
	}

	return b.String()
}

// RenderFileCompact renders just the change indicators for the file tree.
func RenderFileCompact(file *DiffFile) string {
	status := file.Status.String()
	changes := fmt.Sprintf("+%d/-%d", file.Additions, file.Deletions)

	statusStyle := lipgloss.NewStyle()
	switch file.Status {
	case StatusAdded:
		statusStyle = styles.Addition
	case StatusDeleted:
		statusStyle = styles.Deletion
	case StatusModified:
		statusStyle = lipgloss.NewStyle().Foreground(styles.Warning)
	case StatusRenamed:
		statusStyle = lipgloss.NewStyle().Foreground(styles.Secondary)
	}

	return fmt.Sprintf("%s %s", statusStyle.Render(status), changes)
}
