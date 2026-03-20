package diff

import (
	"bytes"
	"fmt"
	"strings"

	godiff "github.com/sourcegraph/go-diff/diff"
)

// Parse parses a unified diff string into structured DiffFiles.
func Parse(raw string) ([]DiffFile, error) {
	if raw == "" {
		return nil, nil
	}

	// Pre-process to extract binary file diffs that go-diff cannot parse.
	// Binary diffs look like:
	//   diff --git a/file b/file
	//   Binary files a/file and b/file differ
	var binaryFiles []DiffFile
	cleaned := preprocessBinaryDiffs(raw, &binaryFiles)

	var files []DiffFile
	if strings.TrimSpace(cleaned) != "" {
		fileDiffs, err := godiff.ParseMultiFileDiff([]byte(cleaned))
		if err != nil {
			return nil, fmt.Errorf("parsing diff: %w", err)
		}
		files = make([]DiffFile, 0, len(fileDiffs)+len(binaryFiles))
		for _, fd := range fileDiffs {
			files = append(files, convertFileDiff(fd))
		}
	}

	files = append(files, binaryFiles...)
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

		hunks, err := parseHunks([]byte(pf.Patch))
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

// preprocessBinaryDiffs extracts binary file diffs from the raw diff text,
// appends them to binaryFiles, and returns the remaining diff text.
func preprocessBinaryDiffs(raw string, binaryFiles *[]DiffFile) string {
	var cleaned strings.Builder
	lines := strings.Split(raw, "\n")

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if strings.HasPrefix(line, "diff --git") {
			// Check if the next non-empty line is "Binary files ... differ"
			j := i + 1
			for j < len(lines) && lines[j] == "" {
				j++
			}
			if j < len(lines) && strings.HasPrefix(lines[j], "Binary files") {
				// Extract path from diff --git header.
				parts := strings.Fields(line)
				path := ""
				if len(parts) >= 4 {
					path = strings.TrimPrefix(parts[3], "b/")
				}
				*binaryFiles = append(*binaryFiles, DiffFile{
					Path:     path,
					IsBinary: true,
				})
				i = j // skip past the "Binary files" line
				continue
			}
		}
		cleaned.WriteString(line)
		cleaned.WriteString("\n")
	}
	return cleaned.String()
}

// convertFileDiff converts a go-diff FileDiff to our DiffFile type.
func convertFileDiff(fd *godiff.FileDiff) DiffFile {
	file := DiffFile{}

	origName := strings.TrimPrefix(fd.OrigName, "a/")
	newName := strings.TrimPrefix(fd.NewName, "b/")

	// Determine status and paths from extended headers and file names.
	for _, ext := range fd.Extended {
		switch {
		case strings.HasPrefix(ext, "rename from "):
			file.OldPath = strings.TrimPrefix(ext, "rename from ")
			file.Status = StatusRenamed
		case strings.HasPrefix(ext, "new file"):
			file.Status = StatusAdded
		case strings.HasPrefix(ext, "deleted file"):
			file.Status = StatusDeleted
		case strings.Contains(ext, "Binary"):
			file.IsBinary = true
		}
	}

	// Set path from new name (or orig for deleted files).
	if newName != "" && newName != "/dev/null" {
		file.Path = newName
	} else if origName != "" && origName != "/dev/null" {
		file.Path = origName
	}

	if file.Status == 0 && !file.IsBinary {
		file.Status = StatusModified
	}

	if file.IsBinary {
		return file
	}

	// Convert hunks.
	for _, h := range fd.Hunks {
		hunk := convertHunk(h)
		for _, l := range hunk.Lines {
			switch l.Type {
			case LineAddition:
				file.Additions++
			case LineDeletion:
				file.Deletions++
			}
		}
		file.Hunks = append(file.Hunks, hunk)
	}

	return file
}

// convertHunk converts a go-diff Hunk to our Hunk type.
func convertHunk(h *godiff.Hunk) Hunk {
	hunk := Hunk{
		OldStart: int(h.OrigStartLine),
		OldLines: int(h.OrigLines),
		NewStart: int(h.NewStartLine),
		NewLines: int(h.NewLines),
		Header:   h.Section,
	}

	oldNum := int(h.OrigStartLine)
	newNum := int(h.NewStartLine)

	lines := bytes.Split(h.Body, []byte("\n"))
	// The trailing newline from Body produces an empty final element; drop it.
	if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	for _, line := range lines {
		if len(line) == 0 {
			// Empty context line (missing space prefix).
			hunk.Lines = append(hunk.Lines, DiffLine{
				Type:   LineContext,
				OldNum: oldNum,
				NewNum: newNum,
			})
			oldNum++
			newNum++
			continue
		}

		switch line[0] {
		case '+':
			hunk.Lines = append(hunk.Lines, DiffLine{
				Type:    LineAddition,
				Content: string(line[1:]),
				NewNum:  newNum,
			})
			newNum++
		case '-':
			hunk.Lines = append(hunk.Lines, DiffLine{
				Type:    LineDeletion,
				Content: string(line[1:]),
				OldNum:  oldNum,
			})
			oldNum++
		case '\\':
			// "\ No newline at end of file" — skip
		default:
			// Context line (prefixed with space).
			content := string(line)
			if len(line) > 0 && line[0] == ' ' {
				content = string(line[1:])
			}
			hunk.Lines = append(hunk.Lines, DiffLine{
				Type:    LineContext,
				Content: content,
				OldNum:  oldNum,
				NewNum:  newNum,
			})
			oldNum++
			newNum++
		}
	}

	return hunk
}

// parseHunks parses hunk content (without file headers) into our Hunk type.
func parseHunks(content []byte) ([]Hunk, error) {
	parsed, err := godiff.ParseHunks(content)
	if err != nil {
		return nil, err
	}

	hunks := make([]Hunk, 0, len(parsed))
	for _, h := range parsed {
		hunks = append(hunks, convertHunk(h))
	}
	return hunks, nil
}
