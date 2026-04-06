package data

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/jethrokuan/pry/internal/review"
)

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

func findPendingReview(prNumber int) (int, string, error) {
	endpoint := fmt.Sprintf("repos/%s/%s/pulls/%d/reviews?per_page=100", owner, repo, prNumber)
	slog.Debug("finding pending review", "endpoint", endpoint, "prNumber", prNumber)

	var reviews []apiReview
	err := rest.Get(endpoint, &reviews)
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

func ensurePendingReview(prNumber int) (int, string, error) {
	id, nodeID, err := findPendingReview(prNumber)
	if err != nil {
		return 0, "", fmt.Errorf("ensure pending review: fetch failed: %w", err)
	}
	if id != 0 {
		return id, nodeID, nil
	}
	id, nodeID, err = CreatePendingReview(prNumber)
	if err != nil {
		return 0, "", fmt.Errorf("ensure pending review: create failed: %w", err)
	}
	return id, nodeID, nil
}

// FetchCommentsAndReview fetches all review threads and the user's pending review.
func FetchCommentsAndReview(prNumber int) ([]review.Thread, int, string, error) {
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
						id
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
								id
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
		Id         string `json:"id"`
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
		Id                string `json:"id"`
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
		vars := map[string]any{
			"owner":        owner,
			"repo":         repo,
			"number":       prNumber,
			"threadCursor": cursor,
		}

		var resp gqlResp
		if err := graphql.Do(query, vars, &resp); err != nil {
			return nil, 0, "", fmt.Errorf("failed to fetch comments and review: %w", err)
		}

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
			NodeID:     gt.Id,
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
				NodeID:    gc.Id,
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

// FetchIssueComments fetches top-level conversation comments on a PR.
func FetchIssueComments(prNumber int) ([]review.IssueComment, error) {
	query := `
	query($owner: String!, $repo: String!, $number: Int!, $cursor: String) {
		repository(owner: $owner, name: $repo) {
			pullRequest(number: $number) {
				comments(first: 100, after: $cursor) {
					nodes {
						databaseId
						author { login }
						body
						createdAt
					}
					pageInfo {
						hasNextPage
						endCursor
					}
				}
			}
		}
	}`

	type gqlNode struct {
		DatabaseId int    `json:"databaseId"`
		Author     struct {
			Login string `json:"login"`
		} `json:"author"`
		Body      string `json:"body"`
		CreatedAt string `json:"createdAt"`
	}

	type gqlResp struct {
		Repository struct {
			PullRequest struct {
				Comments struct {
					Nodes    []gqlNode `json:"nodes"`
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
				} `json:"comments"`
			} `json:"pullRequest"`
		} `json:"repository"`
	}

	var all []review.IssueComment
	var cursor *string

	for {
		vars := map[string]any{
			"owner":  owner,
			"repo":   repo,
			"number": prNumber,
			"cursor": cursor,
		}

		var resp gqlResp
		if err := graphql.Do(query, vars, &resp); err != nil {
			return nil, fmt.Errorf("failed to fetch issue comments: %w", err)
		}

		for _, n := range resp.Repository.PullRequest.Comments.Nodes {
			all = append(all, review.IssueComment{
				ID:        n.DatabaseId,
				Author:    n.Author.Login,
				Body:      n.Body,
				CreatedAt: n.CreatedAt,
			})
		}

		if !resp.Repository.PullRequest.Comments.PageInfo.HasNextPage {
			break
		}
		cursor = &resp.Repository.PullRequest.Comments.PageInfo.EndCursor
	}

	return all, nil
}

// CreatePendingReview creates a new PENDING review on GitHub.
func CreatePendingReview(prNumber int) (int, string, error) {
	endpoint := fmt.Sprintf("repos/%s/%s/pulls/%d/reviews", owner, repo, prNumber)

	payload := map[string]any{}
	body, err := json.Marshal(payload)
	if err != nil {
		slog.Error("failed to marshal pending review payload", "error", err)
		return 0, "", fmt.Errorf("failed to marshal pending review payload: %w", err)
	}

	var result apiReview
	err = rest.Do(http.MethodPost, endpoint, bytes.NewReader(body), &result)
	if err != nil {
		slog.Error("failed to create pending review", "endpoint", endpoint, "error", err)
		return 0, "", fmt.Errorf("failed to create pending review: %w", err)
	}
	slog.Debug("created pending review", "reviewID", result.ID, "nodeID", result.NodeID)
	return result.ID, result.NodeID, nil
}

// AddReviewComment adds a comment to the user's pending review.
func AddReviewComment(prNumber int, reviewNodeID string, path string, line, startLine int, side, body string) (int, string, int, string, error) {
	if reviewNodeID != "" {
		commentID, commentNodeID, err := addReviewThread(reviewNodeID, path, line, startLine, side, body)
		if err == nil {
			return commentID, commentNodeID, 0, reviewNodeID, nil
		}
		slog.Info("fast-path addReviewThread failed, falling back to ensurePendingReview",
			"reviewNodeID", reviewNodeID, "err", err)
	}

	reviewID, nodeID, err := ensurePendingReview(prNumber)
	if err != nil {
		return 0, "", 0, "", err
	}

	commentID, commentNodeID, err := addReviewThread(nodeID, path, line, startLine, side, body)
	if err != nil {
		return 0, "", reviewID, nodeID, fmt.Errorf("failed to add comment to pending review: %w", err)
	}
	return commentID, commentNodeID, reviewID, nodeID, nil
}

func addReviewThread(reviewNodeID string, path string, line, startLine int, side, body string) (int, string, error) {
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
						id
					}
				}
			}
		}
	}`

	vars := map[string]any{
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
						DatabaseId int    `json:"databaseId"`
						Id         string `json:"id"`
					} `json:"nodes"`
				} `json:"comments"`
			} `json:"thread"`
		} `json:"addPullRequestReviewThread"`
	}

	slog.Debug("adding review comment", "reviewNodeID", reviewNodeID, "path", path, "line", line, "side", side)
	err := graphql.Do(mutation, vars, &resp)
	if err != nil {
		slog.Error("failed to add review comment", "reviewNodeID", reviewNodeID, "path", path, "line", line, "error", err)
		return 0, "", fmt.Errorf("graphql addPullRequestReviewThread: %w", err)
	}

	nodes := resp.AddPullRequestReviewThread.Thread.Comments.Nodes
	if len(nodes) == 0 {
		slog.Error("no comment returned from addPullRequestReviewThread", "reviewNodeID", reviewNodeID)
		return 0, "", fmt.Errorf("no comment returned from addPullRequestReviewThread")
	}
	slog.Debug("added review comment", "commentID", nodes[0].DatabaseId)
	return nodes[0].DatabaseId, nodes[0].Id, nil
}

// ReplyToReviewComment adds a reply to an existing review thread.
func ReplyToReviewComment(prNumber int, reviewNodeID string, commentNodeID, body string) (int, string, int, string, error) {
	if reviewNodeID == "" {
		_, nodeID, err := ensurePendingReview(prNumber)
		if err != nil {
			return 0, "", 0, "", err
		}
		reviewNodeID = nodeID
	}

	mutation := `
	mutation($reviewID: ID!, $body: String!, $inReplyTo: ID!) {
		addPullRequestReviewComment(input: {
			pullRequestReviewId: $reviewID
			body: $body
			inReplyTo: $inReplyTo
		}) {
			comment {
				databaseId
				id
			}
		}
	}`

	vars := map[string]any{
		"reviewID":  reviewNodeID,
		"body":      body,
		"inReplyTo": commentNodeID,
	}

	var resp struct {
		AddPullRequestReviewComment struct {
			Comment struct {
				DatabaseId int    `json:"databaseId"`
				Id         string `json:"id"`
			} `json:"comment"`
		} `json:"addPullRequestReviewComment"`
	}

	slog.Debug("replying to review comment", "reviewNodeID", reviewNodeID, "inReplyTo", commentNodeID)
	err := graphql.Do(mutation, vars, &resp)
	if err != nil {
		slog.Error("failed to reply to review comment", "inReplyTo", commentNodeID, "error", err)
		return 0, "", 0, reviewNodeID, fmt.Errorf("graphql addPullRequestReviewComment: %w", err)
	}

	comment := resp.AddPullRequestReviewComment.Comment
	slog.Debug("replied to review comment", "commentID", comment.DatabaseId)
	return comment.DatabaseId, comment.Id, 0, reviewNodeID, nil
}

// DeleteReviewComment deletes a pending review comment by its database ID.
func DeleteReviewComment(prNumber, commentID int) error {
	endpoint := fmt.Sprintf("repos/%s/%s/pulls/comments/%d", owner, repo, commentID)
	slog.Debug("deleting review comment", "commentID", commentID, "endpoint", endpoint)
	err := rest.Do(http.MethodDelete, endpoint, nil, nil)
	if err != nil {
		slog.Error("failed to delete review comment", "commentID", commentID, "error", err)
		return fmt.Errorf("failed to delete review comment: %w", err)
	}
	return nil
}

// EditReviewComment updates the body of a review comment.
func EditReviewComment(commentNodeID, body string) error {
	mutation := `
	mutation($commentID: ID!, $body: String!) {
		updatePullRequestReviewComment(input: {
			pullRequestReviewCommentId: $commentID
			body: $body
		}) {
			pullRequestReviewComment {
				databaseId
			}
		}
	}`

	vars := map[string]any{
		"commentID": commentNodeID,
		"body":      body,
	}

	slog.Debug("editing review comment", "commentNodeID", commentNodeID)
	err := graphql.Do(mutation, vars, nil)
	if err != nil {
		slog.Error("failed to edit review comment", "commentNodeID", commentNodeID, "error", err)
		return fmt.Errorf("graphql updatePullRequestReviewComment: %w", err)
	}
	slog.Debug("edited review comment", "commentNodeID", commentNodeID)
	return nil
}

// SubmitReview submits the pending review to GitHub.
func SubmitReview(pr *review.PullRequest, rev *review.PendingReview) error {
	id, nodeID, err := ensurePendingReview(pr.Number)
	if err != nil {
		return fmt.Errorf("failed to ensure pending review before submit: %w", err)
	}
	rev.ReviewID = id
	rev.ReviewNodeID = nodeID

	endpoint := fmt.Sprintf("repos/%s/%s/pulls/%d/reviews/%d/events",
		owner, repo, pr.Number, rev.ReviewID)

	payload := map[string]string{
		"event": string(rev.Event),
	}
	if rev.Body != "" {
		payload["body"] = rev.Body
	}

	b, err := json.Marshal(payload)
	if err != nil {
		slog.Error("failed to marshal review submission payload", "reviewID", rev.ReviewID, "error", err)
		return fmt.Errorf("failed to marshal review submission payload: %w", err)
	}
	slog.Debug("submitting review", "reviewID", rev.ReviewID, "event", rev.Event)
	err = rest.Do(http.MethodPost, endpoint, bytes.NewReader(b), nil)
	if err != nil {
		slog.Error("failed to submit review", "reviewID", rev.ReviewID, "event", rev.Event, "error", err)
		return fmt.Errorf("failed to submit review: %w", err)
	}
	slog.Info("review submitted", "reviewID", rev.ReviewID, "event", rev.Event)
	return nil
}
