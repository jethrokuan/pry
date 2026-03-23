package diffview

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/jethrokuan/pry/internal/diff"
	"github.com/jethrokuan/pry/internal/ui/styles"
)

// treeNode represents a node in the file tree (either a directory or a file).
type treeNode struct {
	name     string
	path     string // full path for files, empty for directories
	dirPath  string // stable key for collapse state (set by computeDirPaths)
	children []*treeNode
	fileIdx  int // index into m.files, -1 for directories
}

// treeRow is a flattened visible row in the file tree.
type treeRow struct {
	node   *treeNode
	depth  int
	isLast bool
	prefix string
}

// buildTree groups files by directory into a tree structure.
// If included is non-nil, only files whose index is in the set are added.
func buildTree(files []diff.DiffFile, included map[int]bool) *treeNode {
	root := &treeNode{fileIdx: -1}
	for i, file := range files {
		if included != nil && !included[i] {
			continue
		}
		parts := strings.Split(file.Path, "/")
		node := root
		for j, part := range parts {
			isFile := j == len(parts)-1
			// Find existing child
			var child *treeNode
			if !isFile {
				for _, c := range node.children {
					if c.fileIdx == -1 && c.name == part {
						child = c
						break
					}
				}
			}
			if child == nil {
				child = &treeNode{name: part, fileIdx: -1}
				if isFile {
					child.fileIdx = i
					child.path = file.Path
				}
				node.children = append(node.children, child)
			}
			node = child
		}
	}
	// Collapse single-child directories
	collapseTree(root)
	// Compute stable directory paths for collapse map keys
	computeDirPaths(root, "")
	return root
}

// collapseTree collapses directories that have only one child directory.
func collapseTree(node *treeNode) {
	for _, child := range node.children {
		collapseTree(child)
	}
	// Collapse: if this dir has exactly one child and that child is also a dir
	for i, child := range node.children {
		for child.fileIdx == -1 && len(child.children) == 1 && child.children[0].fileIdx == -1 {
			// Merge child into grandchild
			grandchild := child.children[0]
			grandchild.name = child.name + "/" + grandchild.name
			node.children[i] = grandchild
			child = grandchild
		}
	}
}

// computeDirPaths sets dirPath on directory nodes for stable collapse-map keys.
func computeDirPaths(node *treeNode, parentPath string) {
	for _, child := range node.children {
		if child.fileIdx == -1 {
			if parentPath == "" {
				child.dirPath = child.name
			} else {
				child.dirPath = parentPath + "/" + child.name
			}
			computeDirPaths(child, child.dirPath)
		}
	}
}

// flattenTree walks the tree and produces a flat list of visible rows,
// skipping children of collapsed directories.
func flattenTree(root *treeNode, collapsedDirs map[string]bool) []treeRow {
	var rows []treeRow
	for i, child := range root.children {
		isLast := i == len(root.children)-1
		flattenNode(child, "", isLast, 0, collapsedDirs, &rows)
	}
	return rows
}

func flattenNode(node *treeNode, prefix string, isLast bool, depth int, collapsedDirs map[string]bool, rows *[]treeRow) {
	*rows = append(*rows, treeRow{
		node:   node,
		depth:  depth,
		isLast: isLast,
		prefix: prefix,
	})

	// If this is a directory and it's collapsed, skip children
	if node.fileIdx == -1 && collapsedDirs[node.dirPath] {
		return
	}

	// Recurse into children
	if node.fileIdx == -1 {
		childPrefix := prefix + "│   "
		if isLast {
			childPrefix = prefix + "    "
		}
		for i, child := range node.children {
			childIsLast := i == len(node.children)-1
			flattenNode(child, childPrefix, childIsLast, depth+1, collapsedDirs, rows)
		}
	}
}

// collectAllDirPaths returns all directory paths in the tree (for global fold/unfold).
func collectAllDirPaths(root *treeNode) []string {
	var paths []string
	for _, child := range root.children {
		collectAllDirPathsHelper(child, &paths)
	}
	return paths
}

func collectAllDirPathsHelper(node *treeNode, paths *[]string) {
	if node.fileIdx == -1 {
		*paths = append(*paths, node.dirPath)
		for _, child := range node.children {
			collectAllDirPathsHelper(child, paths)
		}
	}
}

// findAncestorDirs returns the chain of ancestor directory nodes for a given fileIdx.
// Returns them from root-child down to the immediate parent.
func findAncestorDirs(root *treeNode, fileIdx int) []*treeNode {
	var path []*treeNode
	findAncestorDirsHelper(root, fileIdx, &path)
	return path
}

func findAncestorDirsHelper(node *treeNode, fileIdx int, path *[]*treeNode) bool {
	for _, child := range node.children {
		if child.fileIdx == fileIdx {
			return true
		}
		if child.fileIdx == -1 {
			*path = append(*path, child)
			if findAncestorDirsHelper(child, fileIdx, path) {
				return true
			}
			*path = (*path)[:len(*path)-1]
		}
	}
	return false
}

// collectFileIndices returns all descendant file indices for a node.
func collectFileIndices(node *treeNode) []int {
	var indices []int
	collectFileIndicesHelper(node, &indices)
	return indices
}

func collectFileIndicesHelper(node *treeNode, indices *[]int) {
	if node.fileIdx >= 0 {
		*indices = append(*indices, node.fileIdx)
		return
	}
	for _, child := range node.children {
		collectFileIndicesHelper(child, indices)
	}
}

// countViewedDescendants returns (viewed, total) counts for files under a node.
func countViewedDescendants(node *treeNode, viewedFiles map[string]bool, files []diff.DiffFile) (int, int) {
	indices := collectFileIndices(node)
	viewed := 0
	for _, idx := range indices {
		if idx < len(files) && viewedFiles[files[idx].Path] {
			viewed++
		}
	}
	return viewed, len(indices)
}

func (m Model) renderFileTree() string {
	var b strings.Builder

	viewedCount := len(m.pendingReview.ViewedFiles)
	fileCount := len(m.files)
	titleStr := fmt.Sprintf("Files (%d)", fileCount)
	if m.filter.isActive() {
		titleStr = fmt.Sprintf("Files (%d/%d)", m.filter.filteredCount, m.filter.totalFiles)
	}
	title := styles.Title.Render(titleStr)
	if viewedCount > 0 {
		title += lipgloss.NewStyle().Foreground(styles.Success).
			Render(fmt.Sprintf(" %d/%d viewed", viewedCount, fileCount))
	}
	if filterStatus := m.filter.statusText(); filterStatus != "" {
		title += "\n" + lipgloss.NewStyle().Foreground(styles.Warning).Italic(true).
			Render("  ⚡ " + filterStatus)
	}
	b.WriteString(title + "\n\n")

	if m.filter.isActive() && m.filter.filteredCount == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(styles.Muted).Italic(true).
			Render("  No matching files."))
		return b.String()
	}

	for i, row := range m.nav.treeRows {
		connector := "├── "
		if row.isLast {
			connector = "└── "
		}

		isCursorRow := i == m.nav.treeCursor && m.nav.focus == FocusFileTree

		if row.node.fileIdx >= 0 {
			// File node
			fileIdx := row.node.fileIdx
			file := m.files[fileIdx]
			isActiveFile := fileIdx == m.nav.fileCursor

			cursor := "  "
			if isCursorRow {
				cursor = "> "
			}

			isViewed := m.pendingReview.ViewedFiles[file.Path]

			statusStyle := lipgloss.NewStyle()
			switch file.Status {
			case diff.StatusAdded:
				statusStyle = styles.Addition
			case diff.StatusDeleted:
				statusStyle = styles.Deletion
			case diff.StatusModified:
				statusStyle = lipgloss.NewStyle().Foreground(styles.Warning)
			case diff.StatusRenamed:
				statusStyle = lipgloss.NewStyle().Foreground(styles.Secondary)
			}

			nameStr := row.node.name
			changes := fmt.Sprintf("+%d/-%d", file.Additions, file.Deletions)

			var line string
			if isCursorRow || isActiveFile {
				// Apply background to each segment individually so inner
				// ANSI resets don't break the row background.
				var bg color.Color
				if isCursorRow {
					bg = styles.BgCursor
				} else {
					bg = styles.BgSurface
				}

				plain := lipgloss.NewStyle().Background(bg)
				if isCursorRow {
					plain = plain.Bold(true)
				}

				statusBg := statusStyle.Background(bg)
				subtitleBg := styles.Subtitle.Background(bg)

				statusStr := statusBg.Render(file.Status.String())
				changesStr := subtitleBg.Render(changes)

				var nameRendered string
				if isViewed {
					nameRendered = lipgloss.NewStyle().Foreground(styles.Success).Background(bg).Render(nameStr)
				} else {
					nameRendered = plain.Render(nameStr)
				}

				line = plain.Render(cursor+row.prefix+connector) + statusStr + plain.Render(" ") + nameRendered + plain.Render(" ") + changesStr

				// Pad to full width
				visWidth := lipgloss.Width(line)
				treeWidth := m.nav.treeViewport.Width()
				if visWidth < treeWidth {
					line += plain.Render(strings.Repeat(" ", treeWidth-visWidth))
				}
			} else {
				status := statusStyle.Render(file.Status.String())
				if isViewed {
					viewedStyle := lipgloss.NewStyle().Foreground(styles.Success)
					line = fmt.Sprintf("%s%s%s%s %s %s", cursor, row.prefix, connector, status, viewedStyle.Render(nameStr), styles.Subtitle.Render(changes))
				} else {
					line = fmt.Sprintf("%s%s%s%s %s %s", cursor, row.prefix, connector, status, nameStr, styles.Subtitle.Render(changes))
				}
			}

			b.WriteString(line + "\n")
		} else {
			// Directory node
			cursor := "  "
			if isCursorRow {
				cursor = "> "
			}

			collapsed := m.nav.collapsedDirs[row.node.dirPath]
			indicator := "▼ "
			if collapsed {
				indicator = "▶ "
			}

			// Show viewed count for directory
			viewedDesc, totalDesc := countViewedDescendants(row.node, m.pendingReview.ViewedFiles, m.files)

			dirStyle := lipgloss.NewStyle().Bold(true)
			if totalDesc > 0 && viewedDesc == totalDesc {
				dirStyle = dirStyle.Foreground(styles.Success)
			} else {
				dirStyle = dirStyle.Foreground(styles.Primary)
			}

			var line string
			if isCursorRow {
				// Apply background to each segment individually so inner
				// ANSI resets don't break the row background.
				bg := styles.BgCursor
				plain := lipgloss.NewStyle().Background(bg).Bold(true)
				dirStyleBg := dirStyle.Background(bg)

				viewedInfoStr := ""
				if totalDesc > 0 {
					viewedInfoStr = lipgloss.NewStyle().Foreground(styles.Muted).Background(bg).
						Render(fmt.Sprintf(" (%d/%d viewed)", viewedDesc, totalDesc))
				}

				line = plain.Render(cursor+row.prefix+connector+indicator) + dirStyleBg.Render(row.node.name) + plain.Render("/") + viewedInfoStr

				// Pad to full width
				visWidth := lipgloss.Width(line)
				treeWidth := m.nav.treeViewport.Width()
				if visWidth < treeWidth {
					line += plain.Render(strings.Repeat(" ", treeWidth-visWidth))
				}
			} else {
				viewedInfo := ""
				if totalDesc > 0 {
					viewedInfo = lipgloss.NewStyle().Foreground(styles.Muted).
						Render(fmt.Sprintf(" (%d/%d viewed)", viewedDesc, totalDesc))
				}

				line = fmt.Sprintf("%s%s%s%s%s/%s", cursor, row.prefix, connector, indicator, dirStyle.Render(row.node.name), viewedInfo)
			}

			b.WriteString(line + "\n")
		}
	}

	return b.String()
}
