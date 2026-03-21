package github

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jethrokuan/pry/internal/diff"
)

// prFile represents a file changed in a PR from the API.
type prFile struct {
	SHA              string `json:"sha"`
	Filename         string `json:"filename"`
	Status           string `json:"status"` // added, removed, modified, renamed
	Additions        int    `json:"additions"`
	Deletions        int    `json:"deletions"`
	Changes          int    `json:"changes"`
	Patch            string `json:"patch"`
	PreviousFilename string `json:"previous_filename"`
}

// FetchDiffFiles fetches and parses the changed files for a PR.
// Paginates automatically to retrieve all files.
func (c *Client) FetchDiffFiles(_ context.Context, number int) ([]diff.DiffFile, error) {
	endpoint := fmt.Sprintf("repos/%s/%s/pulls/%d/files?per_page=100&page=%%d",
		c.owner, c.repo, number)

	slog.Debug("fetching diff files", "prNumber", number, "endpoint", endpoint)
	allFiles, err := paginateREST[prFile](c.rest, endpoint)
	if err != nil {
		slog.Error("failed to fetch PR files", "prNumber", number, "error", err)
		return nil, fmt.Errorf("failed to fetch PR files: %w", err)
	}
	slog.Debug("fetched diff files", "prNumber", number, "fileCount", len(allFiles))

	patches := make([]diff.FilePatch, len(allFiles))
	for i, f := range allFiles {
		patches[i] = diff.FilePatch{
			Filename:         f.Filename,
			PreviousFilename: f.PreviousFilename,
			Status:           f.Status,
			Additions:        f.Additions,
			Deletions:        f.Deletions,
			Patch:            f.Patch,
		}
	}
	return diff.ParseFromPatches(patches), nil
}
