package git

import (
	"bufio"
	"bytes"
	"log/slog"
	"os/exec"
	"strings"
)

// MergeConflictFiles uses git merge-tree to detect which files have merge
// conflicts between two refs. Returns nil if the refs are unavailable or
// git merge-tree is not supported (requires git 2.38+).
func MergeConflictFiles(baseRef, headRef string) []string {
	cmd := exec.Command("git", "merge-tree", "--write-tree", "--name-only", baseRef, headRef)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		// Exit 0 means clean merge — no conflicts.
		return nil
	}

	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 1 {
		// Exit >1 or other error means merge-tree couldn't run
		// (refs not available, git too old, etc.)
		slog.Debug("git merge-tree failed", "error", err, "stderr", stderr.String())
		return nil
	}

	// Exit code 1: conflicts detected. With --name-only, output is:
	//   <tree-oid>\n
	//   <conflicting file paths, one per line>
	var files []string
	scanner := bufio.NewScanner(&stdout)
	firstLine := true
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if firstLine {
			// Skip the tree OID on the first line.
			firstLine = false
			continue
		}
		if line != "" {
			files = append(files, line)
		}
	}

	slog.Debug("detected merge conflict files", "count", len(files), "files", files)
	return files
}
