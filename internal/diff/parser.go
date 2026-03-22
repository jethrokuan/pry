package diff

import (
	"bytes"
	"log/slog"

	godiff "github.com/sourcegraph/go-diff/diff"
)

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
		} else {
			slog.Warn("failed to parse hunks from patch", "file", pf.Filename, "error", err)
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
