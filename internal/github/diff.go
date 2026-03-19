package github

import (
	"context"
	"fmt"

	"github.com/jkuan/pr-review/internal/diff"
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

	allFiles, err := paginateREST[prFile](c.rest, endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PR files: %w", err)
	}

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
