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

type apiComment struct {
	ID   int    `json:"id"`
	Path string `json:"path"`
	Line int    `json:"line"`
	Side string `json:"side"`
	Body string `json:"body"`
	User struct {
		Login string `json:"login"`
	} `json:"user"`
	CreatedAt string `json:"created_at"`
}

// --- Pending review API ---

// FetchPendingReview finds the authenticated user's existing PENDING review, if any.
// Returns the review ID, node ID (0/"" if none found) and any pre-existing comments on it.
func (c *Client) FetchPendingReview(_ context.Context, prNumber int) (int, string, []review.Comment, error) {
	endpoint := fmt.Sprintf("repos/%s/%s/pulls/%d/reviews?per_page=100", c.owner, c.repo, prNumber)
	slog.Debug("fetching pending review", "endpoint", endpoint, "prNumber", prNumber)

	var reviews []apiReview
	err := c.rest.Get(endpoint, &reviews)
	if err != nil {
		slog.Error("failed to fetch reviews", "endpoint", endpoint, "error", err)
		return 0, "", nil, fmt.Errorf("failed to fetch reviews: %w", err)
	}
	slog.Debug("fetched reviews", "count", len(reviews), "prNumber", prNumber)

	// Find the PENDING review (there can be at most one per user)
	for _, r := range reviews {
		if r.State == "PENDING" {
			comments, err := c.fetchReviewComments(prNumber, r.ID)
			if err != nil {
				return r.ID, r.NodeID, nil, err
			}
			return r.ID, r.NodeID, comments, nil
		}
	}

	return 0, "", nil, nil
}

func (c *Client) fetchReviewComments(prNumber, reviewID int) ([]review.Comment, error) {
	endpoint := fmt.Sprintf("repos/%s/%s/pulls/%d/reviews/%d/comments?per_page=100&page=%%d",
		c.owner, c.repo, prNumber, reviewID)

	batch, err := paginateREST[apiComment](c.rest, endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch review comments: %w", err)
	}

	comments := make([]review.Comment, len(batch))
	for i, ac := range batch {
		comments[i] = review.Comment{
			ID:        ac.ID,
			Path:      ac.Path,
			Line:      ac.Line,
			Side:      ac.Side,
			Body:      ac.Body,
			Author:    ac.User.Login,
			CreatedAt: ac.CreatedAt,
			IsPending: true,
		}
	}

	return comments, nil
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

// AddReviewComment adds a comment to an existing pending review on GitHub
// using the GraphQL addPullRequestReviewThread mutation.
// Returns the database ID of the created comment.
func (c *Client) AddReviewComment(_ context.Context, reviewNodeID string, path string, line, startLine int, side, body string) (int, error) {
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
		return 0, fmt.Errorf("failed to add comment to pending review: %w", err)
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
	// If no pending review exists on GitHub yet (e.g. approving without inline
	// comments), create one so we have a valid review ID to submit.
	if rev.ReviewID == 0 {
		id, nodeID, err := c.CreatePendingReview(ctx, pr.Number)
		if err != nil {
			return fmt.Errorf("failed to create pending review before submit: %w", err)
		}
		rev.ReviewID = id
		rev.ReviewNodeID = nodeID
	}

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

// --- Comments ---

// FetchComments gets all review comments on a PR.
// Paginates automatically to retrieve all comments.
func (c *Client) FetchComments(_ context.Context, prNumber int) ([]review.Comment, error) {
	endpoint := fmt.Sprintf("repos/%s/%s/pulls/%d/comments?per_page=100&page=%%d",
		c.owner, c.repo, prNumber)
	slog.Debug("fetching comments", "prNumber", prNumber)

	batch, err := paginateREST[apiComment](c.rest, endpoint)
	if err != nil {
		slog.Error("failed to fetch comments", "prNumber", prNumber, "error", err)
		return nil, fmt.Errorf("failed to fetch comments: %w", err)
	}
	slog.Debug("fetched comments", "prNumber", prNumber, "count", len(batch))

	comments := make([]review.Comment, len(batch))
	for i, ac := range batch {
		comments[i] = review.Comment{
			ID:        ac.ID,
			Path:      ac.Path,
			Line:      ac.Line,
			Side:      ac.Side,
			Body:      ac.Body,
			Author:    ac.User.Login,
			CreatedAt: ac.CreatedAt,
		}
	}

	return comments, nil
}
