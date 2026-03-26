package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/jethrokuan/pry/internal/review"
)

// --- API response types (GitHub-specific) ---

type apiReview struct {
	ID          int    `json:"id"`
	NodeID      string `json:"node_id"`
	State       string `json:"state"`
	Body        string `json:"body"`
	SubmittedAt string `json:"submitted_at"`
	User        struct {
		Login string `json:"login"`
	} `json:"user"`
}

// --- Pending review helpers (internal) ---

// findPendingReview finds the authenticated user's PENDING review via REST.
// Returns the review ID and node ID (0/"" if none found).
// Used internally by ensurePendingReview for add/submit operations.
func (c *Client) findPendingReview(prNumber int) (int, string, error) {
	endpoint := fmt.Sprintf("repos/%s/%s/pulls/%d/reviews?per_page=100", c.owner, c.repo, prNumber)
	slog.Debug("finding pending review", "endpoint", endpoint, "prNumber", prNumber)

	var reviews []apiReview
	err := c.rest.Get(endpoint, &reviews)
	if err != nil {
		return 0, "", fmt.Errorf("failed to fetch reviews: %w", err)
	}

	for _, r := range reviews {
		if r.State == "PENDING" {
			return r.ID, r.NodeID, nil
		}
	}
	return 0, "", nil
}

// --- FetchCommentsAndReview (GraphQL, single query) ---

// FetchCommentsAndReview fetches all review threads (including pending) and
// the user's pending review in a single GraphQL query. Threads include proper
// positioning (line, side) for both submitted and pending comments.
func (c *Client) FetchCommentsAndReview(_ context.Context, prNumber int) ([]review.Thread, int, string, error) {
	query := `
	query($owner: String!, $repo: String!, $number: Int!, $threadCursor: String) {
		repository(owner: $owner, name: $repo) {
			pullRequest(number: $number) {
				reviews(first: 1, states: [PENDING]) {
					nodes {
						databaseId
						id
					}
				}
				reviewThreads(first: 100, after: $threadCursor) {
					nodes {
						path
						line
						originalLine
						startLine
						originalStartLine
						diffSide
						isResolved
						isOutdated
						comments(first: 100) {
							nodes {
								databaseId
								body
								author { login }
								createdAt
								pullRequestReview { databaseId }
							}
						}
					}
					pageInfo {
						hasNextPage
						endCursor
					}
				}
			}
		}
	}`

	type gqlComment struct {
		DatabaseId int    `json:"databaseId"`
		Body       string `json:"body"`
		Author     struct {
			Login string `json:"login"`
		} `json:"author"`
		CreatedAt         string `json:"createdAt"`
		PullRequestReview struct {
			DatabaseId int `json:"databaseId"`
		} `json:"pullRequestReview"`
	}

	type gqlThread struct {
		Path              string `json:"path"`
		Line              *int   `json:"line"`
		OriginalLine      int    `json:"originalLine"`
		StartLine         *int   `json:"startLine"`
		OriginalStartLine *int   `json:"originalStartLine"`
		DiffSide          string `json:"diffSide"`
		IsResolved        bool   `json:"isResolved"`
		IsOutdated        bool   `json:"isOutdated"`
		Comments          struct {
			Nodes []gqlComment `json:"nodes"`
		} `json:"comments"`
	}

	type gqlResp struct {
		Repository struct {
			PullRequest struct {
				Reviews struct {
					Nodes []struct {
						DatabaseId int    `json:"databaseId"`
						Id         string `json:"id"`
					} `json:"nodes"`
				} `json:"reviews"`
				ReviewThreads struct {
					Nodes    []gqlThread `json:"nodes"`
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
				} `json:"reviewThreads"`
			} `json:"pullRequest"`
		} `json:"repository"`
	}

	slog.Debug("fetching comments and review via GraphQL", "prNumber", prNumber)

	var pendingReviewID int
	var pendingNodeID string
	var allThreads []gqlThread
	var cursor *string
	firstPage := true

	for {
		vars := map[string]interface{}{
			"owner":        c.owner,
			"repo":         c.repo,
			"number":       prNumber,
			"threadCursor": cursor,
		}

		var resp gqlResp
		if err := c.graphql.Do(query, vars, &resp); err != nil {
			return nil, 0, "", fmt.Errorf("failed to fetch comments and review: %w", err)
		}

		// Extract pending review from first page only
		if firstPage {
			if nodes := resp.Repository.PullRequest.Reviews.Nodes; len(nodes) > 0 {
				pendingReviewID = nodes[0].DatabaseId
				pendingNodeID = nodes[0].Id
			}
			firstPage = false
		}

		allThreads = append(allThreads, resp.Repository.PullRequest.ReviewThreads.Nodes...)

		if !resp.Repository.PullRequest.ReviewThreads.PageInfo.HasNextPage {
			break
		}
		cursor = &resp.Repository.PullRequest.ReviewThreads.PageInfo.EndCursor
	}

	// Convert to domain threads
	var threads []review.Thread
	for _, gt := range allThreads {
		line := gt.OriginalLine
		if gt.Line != nil {
			line = *gt.Line
		}
		var startLine int
		if gt.StartLine != nil {
			startLine = *gt.StartLine
		} else if gt.OriginalStartLine != nil {
			startLine = *gt.OriginalStartLine
		}

		t := review.Thread{
			Path:       gt.Path,
			Line:       line,
			StartLine:  startLine,
			Side:       gt.DiffSide,
			IsResolved: gt.IsResolved,
			IsOutdated: gt.IsOutdated,
		}
		for _, gc := range gt.Comments.Nodes {
			t.Comments = append(t.Comments, review.Comment{
				ID:        gc.DatabaseId,
				Body:      gc.Body,
				Author:    gc.Author.Login,
				CreatedAt: gc.CreatedAt,
				IsPending: pendingReviewID != 0 && gc.PullRequestReview.DatabaseId == pendingReviewID,
			})
		}
		threads = append(threads, t)
	}

	slog.Debug("fetched comments and review",
		"prNumber", prNumber,
		"threads", len(threads),
		"pendingReviewID", pendingReviewID,
	)

	return threads, pendingReviewID, pendingNodeID, nil
}

// CreatePendingReview creates a new PENDING review on GitHub (no event = pending).
// Returns the review ID and the GraphQL node ID.
func (c *Client) CreatePendingReview(_ context.Context, prNumber int) (int, string, error) {
	endpoint := fmt.Sprintf("repos/%s/%s/pulls/%d/reviews", c.owner, c.repo, prNumber)

	payload := map[string]interface{}{}
	body, err := json.Marshal(payload)
	if err != nil {
		slog.Error("failed to marshal pending review payload", "error", err)
		return 0, "", fmt.Errorf("failed to marshal pending review payload: %w", err)
	}

	var result apiReview
	err = c.rest.Do(http.MethodPost, endpoint, bytes.NewReader(body), &result)
	if err != nil {
		slog.Error("failed to create pending review", "endpoint", endpoint, "error", err)
		return 0, "", fmt.Errorf("failed to create pending review: %w", err)
	}
	slog.Debug("created pending review", "reviewID", result.ID, "nodeID", result.NodeID)
	return result.ID, result.NodeID, nil
}

// AddReviewComment adds a comment to the user's pending review. If
// reviewNodeID is non-empty it is tried first as a fast path (single GraphQL
// call). If the hint is empty or stale, the method falls back to
// ensurePendingReview (fetch-or-create) before retrying.
func (c *Client) AddReviewComment(ctx context.Context, prNumber int, reviewNodeID string, path string, line, startLine int, side, body string) (int, int, string, error) {
	// Fast path: try the cached review node ID directly.
	if reviewNodeID != "" {
		commentID, err := c.addReviewThread(reviewNodeID, path, line, startLine, side, body)
		if err == nil {
			return commentID, 0, reviewNodeID, nil
		}
		slog.Info("fast-path addReviewThread failed, falling back to ensurePendingReview",
			"reviewNodeID", reviewNodeID, "err", err)
	}

	// Slow path: fetch or create a valid pending review.
	reviewID, nodeID, err := c.ensurePendingReview(ctx, prNumber)
	if err != nil {
		return 0, 0, "", err
	}

	commentID, err := c.addReviewThread(nodeID, path, line, startLine, side, body)
	if err != nil {
		return 0, reviewID, nodeID, fmt.Errorf("failed to add comment to pending review: %w", err)
	}
	return commentID, reviewID, nodeID, nil
}

// ensurePendingReview returns the user's existing pending review, creating
// one if none exists. This is the single source of truth for obtaining a
// valid review to attach comments to.
func (c *Client) ensurePendingReview(ctx context.Context, prNumber int) (int, string, error) {
	id, nodeID, err := c.findPendingReview(prNumber)
	if err != nil {
		return 0, "", fmt.Errorf("ensure pending review: fetch failed: %w", err)
	}
	if id != 0 {
		return id, nodeID, nil
	}
	id, nodeID, err = c.CreatePendingReview(ctx, prNumber)
	if err != nil {
		return 0, "", fmt.Errorf("ensure pending review: create failed: %w", err)
	}
	return id, nodeID, nil
}

// addReviewThread performs the GraphQL addPullRequestReviewThread mutation.
func (c *Client) addReviewThread(reviewNodeID string, path string, line, startLine int, side, body string) (int, error) {
	mutation := `
	mutation($reviewID: ID!, $body: String!, $path: String!, $line: Int!, $side: DiffSide!, $startLine: Int, $startSide: DiffSide) {
		addPullRequestReviewThread(input: {
			pullRequestReviewId: $reviewID
			body: $body
			path: $path
			line: $line
			side: $side
			startLine: $startLine
			startSide: $startSide
		}) {
			thread {
				comments(first: 1) {
					nodes {
						databaseId
					}
				}
			}
		}
	}`

	vars := map[string]interface{}{
		"reviewID": reviewNodeID,
		"body":     body,
		"path":     path,
		"line":     line,
		"side":     side,
	}
	if startLine > 0 {
		vars["startLine"] = startLine
		vars["startSide"] = side
	}

	var resp struct {
		AddPullRequestReviewThread struct {
			Thread struct {
				Comments struct {
					Nodes []struct {
						DatabaseId int `json:"databaseId"`
					} `json:"nodes"`
				} `json:"comments"`
			} `json:"thread"`
		} `json:"addPullRequestReviewThread"`
	}

	slog.Debug("adding review comment", "reviewNodeID", reviewNodeID, "path", path, "line", line, "side", side)
	err := c.graphql.Do(mutation, vars, &resp)
	if err != nil {
		slog.Error("failed to add review comment", "reviewNodeID", reviewNodeID, "path", path, "line", line, "error", err)
		return 0, fmt.Errorf("graphql addPullRequestReviewThread: %w", err)
	}

	nodes := resp.AddPullRequestReviewThread.Thread.Comments.Nodes
	if len(nodes) == 0 {
		slog.Error("no comment returned from addPullRequestReviewThread", "reviewNodeID", reviewNodeID)
		return 0, fmt.Errorf("no comment returned from addPullRequestReviewThread")
	}
	slog.Debug("added review comment", "commentID", nodes[0].DatabaseId)
	return nodes[0].DatabaseId, nil
}

// DeleteReviewComment deletes a pending review comment by its database ID.
func (c *Client) DeleteReviewComment(_ context.Context, prNumber, commentID int) error {
	endpoint := fmt.Sprintf("repos/%s/%s/pulls/comments/%d", c.owner, c.repo, commentID)
	slog.Debug("deleting review comment", "commentID", commentID, "endpoint", endpoint)
	err := c.rest.Do(http.MethodDelete, endpoint, nil, nil)
	if err != nil {
		slog.Error("failed to delete review comment", "commentID", commentID, "error", err)
		return fmt.Errorf("failed to delete review comment: %w", err)
	}
	return nil
}

// EditReviewComment updates the body of a review comment by its database ID.
func (c *Client) EditReviewComment(_ context.Context, prNumber, commentID int, body string) error {
	endpoint := fmt.Sprintf("repos/%s/%s/pulls/comments/%d", c.owner, c.repo, commentID)
	payload, err := json.Marshal(map[string]string{"body": body})
	if err != nil {
		slog.Error("failed to marshal edit comment payload", "commentID", commentID, "error", err)
		return fmt.Errorf("failed to marshal edit comment payload: %w", err)
	}
	err = c.rest.Do(http.MethodPatch, endpoint, bytes.NewReader(payload), nil)
	if err != nil {
		slog.Error("failed to edit review comment", "commentID", commentID, "error", err)
		return fmt.Errorf("failed to edit review comment: %w", err)
	}
	slog.Debug("edited review comment", "commentID", commentID)
	return nil
}

// SubmitReview submits the pending review to GitHub.
// Comments are already on the server; this just finalizes the review with an event.
func (c *Client) SubmitReview(ctx context.Context, pr *review.PullRequest, rev *review.PendingReview) error {
	// Always ensure a valid pending review exists — the locally cached ID may
	// be stale (e.g. GitHub auto-deleted it when all comments were removed).
	id, nodeID, err := c.ensurePendingReview(ctx, pr.Number)
	if err != nil {
		return fmt.Errorf("failed to ensure pending review before submit: %w", err)
	}
	rev.ReviewID = id
	rev.ReviewNodeID = nodeID

	endpoint := fmt.Sprintf("repos/%s/%s/pulls/%d/reviews/%d/events",
		c.owner, c.repo, pr.Number, rev.ReviewID)

	payload := map[string]string{
		"event": string(rev.Event),
	}
	if rev.Body != "" {
		payload["body"] = rev.Body
	}

	body, err := json.Marshal(payload)
	if err != nil {
		slog.Error("failed to marshal review submission payload", "reviewID", rev.ReviewID, "error", err)
		return fmt.Errorf("failed to marshal review submission payload: %w", err)
	}
	slog.Debug("submitting review", "reviewID", rev.ReviewID, "event", rev.Event)
	err = c.rest.Do(http.MethodPost, endpoint, bytes.NewReader(body), nil)
	if err != nil {
		slog.Error("failed to submit review", "reviewID", rev.ReviewID, "event", rev.Event, "error", err)
		return fmt.Errorf("failed to submit review: %w", err)
	}
	slog.Info("review submitted", "reviewID", rev.ReviewID, "event", rev.Event)
	return nil
}

// --- Viewed files ---

// FetchViewedFiles returns the set of file paths already marked as viewed on a PR.
func (c *Client) FetchViewedFiles(_ context.Context, prNodeID string) (map[string]bool, error) {
	query := `
	query($prID: ID!, $cursor: String) {
		node(id: $prID) {
			... on PullRequest {
				files(first: 100, after: $cursor) {
					nodes {
						path
						viewerViewedState
					}
					pageInfo {
						hasNextPage
						endCursor
					}
				}
			}
		}
	}`

	viewed := make(map[string]bool)
	var cursor *string

	for {
		vars := map[string]interface{}{
			"prID":   prNodeID,
			"cursor": cursor,
		}

		var resp struct {
			Node struct {
				Files struct {
					Nodes []struct {
						Path              string `json:"path"`
						ViewerViewedState string `json:"viewerViewedState"`
					} `json:"nodes"`
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
				} `json:"files"`
			} `json:"node"`
		}

		err := c.graphql.Do(query, vars, &resp)
		if err != nil {
			slog.Error("failed to fetch viewed files", "prNodeID", prNodeID, "error", err)
			return nil, fmt.Errorf("failed to fetch viewed files: %w", err)
		}

		for _, f := range resp.Node.Files.Nodes {
			if f.ViewerViewedState == "VIEWED" {
				viewed[f.Path] = true
			}
		}

		if !resp.Node.Files.PageInfo.HasNextPage {
			break
		}
		cursor = &resp.Node.Files.PageInfo.EndCursor
	}

	return viewed, nil
}

// MarkFileAsViewed marks a file as viewed on a PR using GraphQL.
func (c *Client) MarkFileAsViewed(_ context.Context, prNodeID, path string) error {
	mutation := `
	mutation($prID: ID!, $path: String!) {
		markFileAsViewed(input: {pullRequestId: $prID, path: $path}) {
			clientMutationId
		}
	}`

	slog.Debug("marking file as viewed", "prNodeID", prNodeID, "path", path)
	err := c.graphql.Do(mutation, map[string]interface{}{
		"prID": prNodeID,
		"path": path,
	}, nil)
	if err != nil {
		slog.Error("failed to mark file as viewed", "prNodeID", prNodeID, "path", path, "error", err)
		return fmt.Errorf("failed to mark file as viewed: %w", err)
	}
	return nil
}

// UnmarkFileAsViewed unmarks a file as viewed on a PR.
func (c *Client) UnmarkFileAsViewed(_ context.Context, prNodeID, path string) error {
	mutation := `
	mutation($prID: ID!, $path: String!) {
		unmarkFileAsViewed(input: {pullRequestId: $prID, path: $path}) {
			clientMutationId
		}
	}`

	err := c.graphql.Do(mutation, map[string]interface{}{
		"prID": prNodeID,
		"path": path,
	}, nil)
	if err != nil {
		return fmt.Errorf("failed to unmark file as viewed: %w", err)
	}
	return nil
}

// --- PR Actions ---

// ClosePR closes an open pull request via REST.
func (c *Client) ClosePR(_ context.Context, prNumber int) error {
	endpoint := fmt.Sprintf("repos/%s/%s/pulls/%d", c.owner, c.repo, prNumber)
	payload, err := json.Marshal(map[string]string{"state": "closed"})
	if err != nil {
		return fmt.Errorf("failed to marshal close payload: %w", err)
	}
	slog.Debug("closing PR", "number", prNumber)
	err = c.rest.Do(http.MethodPatch, endpoint, bytes.NewReader(payload), nil)
	if err != nil {
		return fmt.Errorf("failed to close PR #%d: %w", prNumber, err)
	}
	return nil
}

// ReopenPR reopens a closed pull request via REST.
func (c *Client) ReopenPR(_ context.Context, prNumber int) error {
	endpoint := fmt.Sprintf("repos/%s/%s/pulls/%d", c.owner, c.repo, prNumber)
	payload, err := json.Marshal(map[string]string{"state": "open"})
	if err != nil {
		return fmt.Errorf("failed to marshal reopen payload: %w", err)
	}
	slog.Debug("reopening PR", "number", prNumber)
	err = c.rest.Do(http.MethodPatch, endpoint, bytes.NewReader(payload), nil)
	if err != nil {
		return fmt.Errorf("failed to reopen PR #%d: %w", prNumber, err)
	}
	return nil
}

// MergePR merges a pull request via REST using the default merge method.
func (c *Client) MergePR(_ context.Context, prNumber int) error {
	endpoint := fmt.Sprintf("repos/%s/%s/pulls/%d/merge", c.owner, c.repo, prNumber)
	slog.Debug("merging PR", "number", prNumber)
	err := c.rest.Do(http.MethodPut, endpoint, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to merge PR #%d: %w", prNumber, err)
	}
	return nil
}

// MarkReadyForReview converts a draft PR to ready for review via GraphQL.
func (c *Client) MarkReadyForReview(_ context.Context, prNodeID string) error {
	mutation := `
	mutation($prID: ID!) {
		markPullRequestReadyForReview(input: {pullRequestId: $prID}) {
			clientMutationId
		}
	}`
	slog.Debug("marking PR ready for review", "prNodeID", prNodeID)
	err := c.graphql.Do(mutation, map[string]interface{}{
		"prID": prNodeID,
	}, nil)
	if err != nil {
		return fmt.Errorf("failed to mark PR ready for review: %w", err)
	}
	return nil
}

// AssignPR adds a user as an assignee on a PR via REST.
func (c *Client) AssignPR(_ context.Context, prNumber int, login string) error {
	endpoint := fmt.Sprintf("repos/%s/%s/issues/%d/assignees", c.owner, c.repo, prNumber)
	payload, err := json.Marshal(map[string][]string{"assignees": {login}})
	if err != nil {
		return fmt.Errorf("failed to marshal assign payload: %w", err)
	}
	slog.Debug("assigning PR", "number", prNumber, "login", login)
	err = c.rest.Do(http.MethodPost, endpoint, bytes.NewReader(payload), nil)
	if err != nil {
		return fmt.Errorf("failed to assign %s to PR #%d: %w", login, prNumber, err)
	}
	return nil
}

// UnassignPR removes a user as an assignee from a PR via REST.
func (c *Client) UnassignPR(_ context.Context, prNumber int, login string) error {
	endpoint := fmt.Sprintf("repos/%s/%s/issues/%d/assignees", c.owner, c.repo, prNumber)
	payload, err := json.Marshal(map[string][]string{"assignees": {login}})
	if err != nil {
		return fmt.Errorf("failed to marshal unassign payload: %w", err)
	}
	slog.Debug("unassigning PR", "number", prNumber, "login", login)
	err = c.rest.Do(http.MethodDelete, endpoint, bytes.NewReader(payload), nil)
	if err != nil {
		return fmt.Errorf("failed to unassign %s from PR #%d: %w", login, prNumber, err)
	}
	return nil
}

