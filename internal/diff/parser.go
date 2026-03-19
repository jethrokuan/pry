package diff

import (
	"fmt"
	"strconv"
	"strings"
)

// Parse parses a unified diff string into structured DiffFiles.
func Parse(raw string) ([]DiffFile, error) {
	var files []DiffFile
	chunks := splitDiffByFile(raw)

	for _, chunk := range chunks {
		file, err := parseFileChunk(chunk)
		if err != nil {
			continue // skip unparseable chunks
		}
		files = append(files, *file)
	}

	return files, nil
}

// ParseFromPatches creates DiffFiles from GitHub API file patches.
func ParseFromPatches(prFiles []FilePatch) []DiffFile {
	files := make([]DiffFile, 0, len(prFiles))
	for _, pf := range prFiles {
		df := DiffFile{
			Path:      pf.Filename,
			OldPath:   pf.PreviousFilename,
			Additions: pf.Additions,
			Deletions: pf.Deletions,
		}

		switch pf.Status {
		case "added":
			df.Status = StatusAdded
		case "removed":
			df.Status = StatusDeleted
		case "renamed":
			df.Status = StatusRenamed
		default:
			df.Status = StatusModified
		}

		if pf.Patch == "" {
			df.IsBinary = true
			files = append(files, df)
			continue
		}

		hunks, err := parseHunks(pf.Patch)
		if err == nil {
			df.Hunks = hunks
		}

		files = append(files, df)
	}
	return files
}

// FilePatch is the input structure for ParseFromPatches.
type FilePatch struct {
	Filename         string
	PreviousFilename string
	Status           string
	Additions        int
	Deletions        int
	Patch            string
}

// BuildPositionMap builds a map from (path, newLineNumber) -> diff position.
// This is needed for the GitHub review comment API.
func BuildPositionMap(files []DiffFile) map[string]map[int]int {
	result := make(map[string]map[int]int)

	for _, f := range files {
		lineMap := make(map[int]int)
		position := 0

		for _, hunk := range f.Hunks {
			position++ // hunk header counts as a position
			for _, line := range hunk.Lines {
				position++
				if line.Type != LineDeletion && line.NewNum > 0 {
					lineMap[line.NewNum] = position
				}
			}
		}

		result[f.Path] = lineMap
	}

	return result
}

func splitDiffByFile(raw string) []string {
	lines := strings.Split(raw, "\n")
	var chunks []string
	var current []string

	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git") {
			if len(current) > 0 {
				chunks = append(chunks, strings.Join(current, "\n"))
			}
			current = []string{line}
		} else {
			current = append(current, line)
		}
	}
	if len(current) > 0 {
		chunks = append(chunks, strings.Join(current, "\n"))
	}

	return chunks
}

func parseFileChunk(chunk string) (*DiffFile, error) {
	lines := strings.Split(chunk, "\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("empty chunk")
	}

	file := &DiffFile{}

	// Parse header
	for i, line := range lines {
		if strings.HasPrefix(line, "diff --git") {
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				file.Path = strings.TrimPrefix(parts[3], "b/")
			}
		} else if strings.HasPrefix(line, "rename from") {
			file.OldPath = strings.TrimPrefix(line, "rename from ")
			file.Status = StatusRenamed
		} else if strings.HasPrefix(line, "new file") {
			file.Status = StatusAdded
		} else if strings.HasPrefix(line, "deleted file") {
			file.Status = StatusDeleted
		} else if strings.HasPrefix(line, "Binary files") {
			file.IsBinary = true
			return file, nil
		} else if strings.HasPrefix(line, "--- ") {
			// Start of actual diff content
			if file.Status == 0 && !strings.HasPrefix(line, "--- /dev/null") {
				file.Status = StatusModified
			}
		} else if strings.HasPrefix(line, "+++ ") {
			newPath := strings.TrimPrefix(line, "+++ ")
			newPath = strings.TrimPrefix(newPath, "b/")
			if newPath != "/dev/null" {
				file.Path = newPath
			}
		} else if strings.HasPrefix(line, "@@") {
			// Parse hunks from here
			hunkContent := strings.Join(lines[i:], "\n")
			hunks, err := parseHunks(hunkContent)
			if err == nil {
				file.Hunks = hunks
				for _, h := range hunks {
					for _, l := range h.Lines {
						switch l.Type {
						case LineAddition:
							file.Additions++
						case LineDeletion:
							file.Deletions++
						}
					}
				}
			}
			break
		}
	}

	return file, nil
}

func parseHunks(content string) ([]Hunk, error) {
	lines := strings.Split(content, "\n")
	var hunks []Hunk
	var currentHunk *Hunk

	for _, line := range lines {
		if strings.HasPrefix(line, "@@") {
			if currentHunk != nil {
				hunks = append(hunks, *currentHunk)
			}
			hunk, err := parseHunkHeader(line)
			if err != nil {
				continue
			}
			currentHunk = hunk
			continue
		}

		if currentHunk == nil {
			continue
		}

		dl := parseDiffLine(line, currentHunk)
		if dl != nil {
			currentHunk.Lines = append(currentHunk.Lines, *dl)
		}
	}

	if currentHunk != nil {
		hunks = append(hunks, *currentHunk)
	}

	return hunks, nil
}

func parseHunkHeader(line string) (*Hunk, error) {
	// Format: @@ -oldStart,oldLines +newStart,newLines @@ optional header
	hunk := &Hunk{Header: line}

	// Find the range info between @@ markers
	parts := strings.SplitN(line, "@@", 3)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid hunk header: %s", line)
	}

	rangePart := strings.TrimSpace(parts[1])
	ranges := strings.Fields(rangePart)

	for _, r := range ranges {
		if strings.HasPrefix(r, "-") {
			nums := strings.SplitN(strings.TrimPrefix(r, "-"), ",", 2)
			hunk.OldStart, _ = strconv.Atoi(nums[0])
			if len(nums) > 1 {
				hunk.OldLines, _ = strconv.Atoi(nums[1])
			} else {
				hunk.OldLines = 1
			}
		} else if strings.HasPrefix(r, "+") {
			nums := strings.SplitN(strings.TrimPrefix(r, "+"), ",", 2)
			hunk.NewStart, _ = strconv.Atoi(nums[0])
			if len(nums) > 1 {
				hunk.NewLines, _ = strconv.Atoi(nums[1])
			} else {
				hunk.NewLines = 1
			}
		}
	}

	if len(parts) >= 3 {
		hunk.Header = strings.TrimSpace(parts[2])
	}

	return hunk, nil
}

func parseDiffLine(line string, hunk *Hunk) *DiffLine {
	if len(line) == 0 {
		// Empty context line
		dl := &DiffLine{Type: LineContext, Content: ""}
		dl.OldNum = nextOldLine(hunk)
		dl.NewNum = nextNewLine(hunk)
		return dl
	}

	switch line[0] {
	case '+':
		dl := &DiffLine{
			Type:    LineAddition,
			Content: line[1:],
			NewNum:  nextNewLine(hunk),
		}
		return dl
	case '-':
		dl := &DiffLine{
			Type:    LineDeletion,
			Content: line[1:],
			OldNum:  nextOldLine(hunk),
		}
		return dl
	case '\\':
		// "\ No newline at end of file"
		return nil
	default:
		// Context line (starts with space)
		content := line
		if len(line) > 0 && line[0] == ' ' {
			content = line[1:]
		}
		dl := &DiffLine{
			Type:    LineContext,
			Content: content,
			OldNum:  nextOldLine(hunk),
			NewNum:  nextNewLine(hunk),
		}
		return dl
	}
}

func nextOldLine(hunk *Hunk) int {
	num := hunk.OldStart
	for _, l := range hunk.Lines {
		if l.Type != LineAddition {
			num = l.OldNum + 1
		}
	}
	if len(hunk.Lines) == 0 {
		return hunk.OldStart
	}
	return num
}

func nextNewLine(hunk *Hunk) int {
	num := hunk.NewStart
	for _, l := range hunk.Lines {
		if l.Type != LineDeletion {
			num = l.NewNum + 1
		}
	}
	if len(hunk.Lines) == 0 {
		return hunk.NewStart
	}
	return num
}
