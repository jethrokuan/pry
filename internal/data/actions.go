package data

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
)

// ClosePR closes an open pull request via REST.
func ClosePR(prNumber int) error {
	endpoint := fmt.Sprintf("repos/%s/%s/pulls/%d", owner, repo, prNumber)
	payload, err := json.Marshal(map[string]string{"state": "closed"})
	if err != nil {
		return fmt.Errorf("failed to marshal close payload: %w", err)
	}
	slog.Debug("closing PR", "number", prNumber)
	err = rest.Do(http.MethodPatch, endpoint, bytes.NewReader(payload), nil)
	if err != nil {
		return fmt.Errorf("failed to close PR #%d: %w", prNumber, err)
	}
	return nil
}

// ReopenPR reopens a closed pull request via REST.
func ReopenPR(prNumber int) error {
	endpoint := fmt.Sprintf("repos/%s/%s/pulls/%d", owner, repo, prNumber)
	payload, err := json.Marshal(map[string]string{"state": "open"})
	if err != nil {
		return fmt.Errorf("failed to marshal reopen payload: %w", err)
	}
	slog.Debug("reopening PR", "number", prNumber)
	err = rest.Do(http.MethodPatch, endpoint, bytes.NewReader(payload), nil)
	if err != nil {
		return fmt.Errorf("failed to reopen PR #%d: %w", prNumber, err)
	}
	return nil
}

// MergePR merges a pull request using the repository's preferred merge method.
func MergePR(prNumber int) error {
	method, err := preferredMergeMethod()
	if err != nil {
		slog.Warn("failed to detect merge method, trying default", "error", err)
		method = "merge"
	}

	endpoint := fmt.Sprintf("repos/%s/%s/pulls/%d/merge", owner, repo, prNumber)
	payload, err := json.Marshal(map[string]string{"merge_method": method})
	if err != nil {
		return fmt.Errorf("failed to marshal merge payload: %w", err)
	}
	slog.Debug("merging PR", "number", prNumber, "method", method)
	err = rest.Do(http.MethodPut, endpoint, bytes.NewReader(payload), nil)
	if err != nil {
		return fmt.Errorf("failed to merge PR #%d: %w", prNumber, err)
	}
	return nil
}

func preferredMergeMethod() (string, error) {
	var repoInfo struct {
		AllowSquashMerge bool `json:"allow_squash_merge"`
		AllowMergeCommit bool `json:"allow_merge_commit"`
		AllowRebaseMerge bool `json:"allow_rebase_merge"`
	}
	err := rest.Get(fmt.Sprintf("repos/%s/%s", owner, repo), &repoInfo)
	if err != nil {
		return "", err
	}
	switch {
	case repoInfo.AllowSquashMerge:
		return "squash", nil
	case repoInfo.AllowMergeCommit:
		return "merge", nil
	case repoInfo.AllowRebaseMerge:
		return "rebase", nil
	default:
		return "merge", nil
	}
}

// MarkReadyForReview converts a draft PR to ready for review via GraphQL.
func MarkReadyForReview(prNodeID string) error {
	mutation := `
	mutation($prID: ID!) {
		markPullRequestReadyForReview(input: {pullRequestId: $prID}) {
			clientMutationId
		}
	}`
	slog.Debug("marking PR ready for review", "prNodeID", prNodeID)
	err := graphql.Do(mutation, map[string]any{
		"prID": prNodeID,
	}, nil)
	if err != nil {
		return fmt.Errorf("failed to mark PR ready for review: %w", err)
	}
	return nil
}

// AssignPR adds a user as an assignee on a PR via REST.
func AssignPR(prNumber int, login string) error {
	endpoint := fmt.Sprintf("repos/%s/%s/issues/%d/assignees", owner, repo, prNumber)
	payload, err := json.Marshal(map[string][]string{"assignees": {login}})
	if err != nil {
		return fmt.Errorf("failed to marshal assign payload: %w", err)
	}
	slog.Debug("assigning PR", "number", prNumber, "login", login)
	err = rest.Do(http.MethodPost, endpoint, bytes.NewReader(payload), nil)
	if err != nil {
		return fmt.Errorf("failed to assign %s to PR #%d: %w", login, prNumber, err)
	}
	return nil
}

// UnassignPR removes a user as an assignee from a PR via REST.
func UnassignPR(prNumber int, login string) error {
	endpoint := fmt.Sprintf("repos/%s/%s/issues/%d/assignees", owner, repo, prNumber)
	payload, err := json.Marshal(map[string][]string{"assignees": {login}})
	if err != nil {
		return fmt.Errorf("failed to marshal unassign payload: %w", err)
	}
	slog.Debug("unassigning PR", "number", prNumber, "login", login)
	err = rest.Do(http.MethodDelete, endpoint, bytes.NewReader(payload), nil)
	if err != nil {
		return fmt.Errorf("failed to unassign %s from PR #%d: %w", login, prNumber, err)
	}
	return nil
}
