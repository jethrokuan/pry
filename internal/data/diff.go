package data

import (
	"fmt"
	"log/slog"

	"github.com/jethrokuan/pry/internal/diff"
)

// prFile represents a file changed in a PR from the API.
type prFile struct {
	SHA              string `json:"sha"`
	Filename         string `json:"filename"`
	Status           string `json:"status"`
	Additions        int    `json:"additions"`
	Deletions        int    `json:"deletions"`
	Changes          int    `json:"changes"`
	Patch            string `json:"patch"`
	PreviousFilename string `json:"previous_filename"`
}

// FetchDiffFiles fetches and parses the changed files for a PR.
func FetchDiffFiles(number int) ([]diff.DiffFile, error) {
	endpoint := fmt.Sprintf("repos/%s/%s/pulls/%d/files?per_page=100&page=%%d",
		owner, repo, number)

	slog.Debug("fetching diff files", "prNumber", number, "endpoint", endpoint)
	allFiles, err := paginateREST[prFile](endpoint)
	if err != nil {
		slog.Error("failed to fetch PR files", "prNumber", number, "error", err)
		return nil, fmt.Errorf("failed to fetch PR files: %w", err)
	}
	slog.Debug("fetched diff files", "prNumber", number, "fileCount", len(allFiles))

	return filesToDiffFiles(allFiles), nil
}

// compareResponse is the envelope returned by the GitHub compare API.
type compareResponse struct {
	Files []prFile `json:"files"`
}

// FetchCommitDiff fetches the diff between two commits using the compare API.
func FetchCommitDiff(baseSHA, headSHA string) ([]diff.DiffFile, error) {
	endpoint := fmt.Sprintf("repos/%s/%s/compare/%s...%s?per_page=100",
		owner, repo, baseSHA, headSHA)

	slog.Debug("fetching commit diff", "base", baseSHA, "head", headSHA)

	var resp compareResponse
	if err := rest.Get(endpoint, &resp); err != nil {
		slog.Error("failed to fetch commit diff", "base", baseSHA, "head", headSHA, "error", err)
		return nil, fmt.Errorf("failed to fetch commit diff: %w", err)
	}
	slog.Debug("fetched commit diff", "base", baseSHA, "head", headSHA, "fileCount", len(resp.Files))

	return filesToDiffFiles(resp.Files), nil
}

func filesToDiffFiles(files []prFile) []diff.DiffFile {
	patches := make([]diff.FilePatch, len(files))
	for i, f := range files {
		patches[i] = diff.FilePatch{
			Filename:         f.Filename,
			PreviousFilename: f.PreviousFilename,
			Status:           f.Status,
			Additions:        f.Additions,
			Deletions:        f.Deletions,
			Patch:            f.Patch,
		}
	}
	return diff.ParseFromPatches(patches)
}
