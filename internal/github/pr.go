package github

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jkuan/pr-review/internal/review"
)

const myTeamsPlaceholder = "@my-teams"

type graphqlPRNode struct {
	ID             string    `json:"id"`
	Number         int       `json:"number"`
	Title          string    `json:"title"`
	State          string    `json:"state"`
	IsDraft        bool      `json:"isDraft"`
	URL            string    `json:"url"`
	Body           string    `json:"body"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
	Additions      int       `json:"additions"`
	Deletions      int       `json:"deletions"`
	ChangedFiles   int       `json:"changedFiles"`
	HeadRefName    string    `json:"headRefName"`
	BaseRefName    string    `json:"baseRefName"`
	HeadRefOid     string    `json:"headRefOid"`
	ReviewDecision string    `json:"reviewDecision"`
	Author         struct {
		Login string `json:"login"`
	} `json:"author"`
	Labels struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	} `json:"labels"`
	ReviewRequests struct {
		Nodes []struct {
			RequestedReviewer graphqlReviewer `json:"requestedReviewer"`
		} `json:"nodes"`
	} `json:"reviewRequests"`
	LatestReviews struct {
		Nodes []struct {
			Author struct {
				Login string `json:"login"`
			} `json:"author"`
			State string `json:"state"`
		} `json:"nodes"`
	} `json:"latestReviews"`
}

// graphqlReviewer handles the union type (User | Team) in requestedReviewer.
type graphqlReviewer struct {
	// Team fields
	Slug         string `json:"slug"`
	Organization struct {
		Login string `json:"login"`
	} `json:"organization"`
	// User fields (login is present on both User and Team via typename)
	Login string `json:"login"`
}

type graphqlPRResponse struct {
	Viewer struct {
		Login string `json:"login"`
	} `json:"viewer"`
	Search struct {
		Nodes    []graphqlPRNode `json:"nodes"`
		PageInfo struct {
			HasNextPage bool   `json:"hasNextPage"`
			EndCursor   string `json:"endCursor"`
		} `json:"pageInfo"`
	} `json:"search"`
}

// ListPRs fetches PRs based on the given filter.
// If the qualifier contains @my-teams, it expands into one query per team
// the authenticated user belongs to (in the repo's org) and deduplicates.
func (c *Client) ListPRs(_ context.Context, filter review.PRFilter) ([]review.PullRequest, error) {
	if strings.Contains(filter.Qualifier, myTeamsPlaceholder) {
		return c.listPRsForTeams(filter)
	}
	return c.searchPRs(filter.Qualifier)
}

// searchPRs runs a single GitHub search query and returns matching PRs.
func (c *Client) searchPRs(qualifier string) ([]review.PullRequest, error) {
	query := fmt.Sprintf("is:pr is:open repo:%s/%s %s sort:updated-desc", c.owner, c.repo, qualifier)

	// Minimal fields for the list view — details fetched on demand via GetPR.
	graphqlQuery := `
	query($query: String!) {
		viewer { login }
		search(query: $query, type: ISSUE, first: 30) {
			nodes {
				... on PullRequest {
					id
					number
					title
					isDraft
					updatedAt
					additions
					deletions
					author { login }
					reviewRequests(first: 20) {
						nodes {
							requestedReviewer {
								... on Team { slug organization { login } }
								... on User { login }
							}
						}
					}
					latestReviews(first: 30) {
						nodes {
							author { login }
							state
						}
					}
				}
			}
		}
	}`

	slog.Debug("searching PRs", "query", query)
	var resp graphqlPRResponse
	err := c.graphql.Do(graphqlQuery, map[string]interface{}{
		"query": query,
	}, &resp)
	if err != nil {
		slog.Error("failed to fetch PRs", "query", query, "error", err)
		return nil, fmt.Errorf("failed to fetch PRs: %w", err)
	}
	slog.Debug("fetched PRs", "count", len(resp.Search.Nodes))

	viewer := resp.Viewer.Login
	var prs []review.PullRequest
	for _, node := range resp.Search.Nodes {
		if node.Number == 0 {
			continue
		}
		prs = append(prs, nodeToPR(node, viewer))
	}

	return prs, nil
}

// listPRsForTeams expands @my-teams into parallel per-team queries and deduplicates.
func (c *Client) listPRsForTeams(filter review.PRFilter) ([]review.PullRequest, error) {
	teams, err := c.getUserTeams()
	if err != nil {
		return nil, err
	}
	if len(teams) == 0 {
		return nil, nil
	}

	type result struct {
		prs []review.PullRequest
		err error
	}

	results := make([]result, len(teams))
	var wg sync.WaitGroup

	for i, team := range teams {
		wg.Add(1)
		go func(idx int, teamSlug string) {
			defer wg.Done()
			qualifier := strings.ReplaceAll(filter.Qualifier, myTeamsPlaceholder, teamSlug)
			prs, err := c.searchPRs(qualifier)
			results[idx] = result{prs: prs, err: err}
		}(i, team)
	}
	wg.Wait()

	// Merge and deduplicate by PR number, preserving order.
	seen := make(map[int]bool)
	var merged []review.PullRequest
	for _, r := range results {
		if r.err != nil {
			return nil, r.err
		}
		for _, pr := range r.prs {
			if !seen[pr.Number] {
				seen[pr.Number] = true
				merged = append(merged, pr)
			}
		}
	}

	return merged, nil
}

// nodeToPR converts a GraphQL node to a domain PullRequest.
// viewer is the authenticated user's login (used to extract their review state);
// pass "" if unknown.
func nodeToPR(node graphqlPRNode, viewer string) review.PullRequest {
	labels := make([]string, 0, len(node.Labels.Nodes))
	for _, l := range node.Labels.Nodes {
		labels = append(labels, l.Name)
	}

	var pendingTeams []string
	for _, rr := range node.ReviewRequests.Nodes {
		r := rr.RequestedReviewer
		if r.Slug != "" && r.Organization.Login != "" {
			pendingTeams = append(pendingTeams, r.Organization.Login+"/"+r.Slug)
		}
	}

	var myReviewState string
	if viewer != "" {
		for _, rv := range node.LatestReviews.Nodes {
			if strings.EqualFold(rv.Author.Login, viewer) {
				myReviewState = rv.State
				break
			}
		}
	}

	return review.PullRequest{
		NodeID:         node.ID,
		Number:         node.Number,
		Title:          node.Title,
		Author:         node.Author.Login,
		Branch:         node.HeadRefName,
		Base:           node.BaseRefName,
		State:          node.State,
		Draft:          node.IsDraft,
		Labels:         labels,
		CreatedAt:      node.CreatedAt,
		UpdatedAt:      node.UpdatedAt,
		Additions:      node.Additions,
		Deletions:      node.Deletions,
		Files:          node.ChangedFiles,
		Body:           node.Body,
		URL:            node.URL,
		HeadSHA:        node.HeadRefOid,
		ReviewDecision: node.ReviewDecision,
		PendingTeams:   pendingTeams,
		MyReviewState:  myReviewState,
	}
}

// GetPR fetches a single PR by number, including the full body.
func (c *Client) GetPR(_ context.Context, number int) (*review.PullRequest, error) {
	var resp struct {
		Repository struct {
			PullRequest graphqlPRNode `json:"pullRequest"`
		} `json:"repository"`
	}

	query := `
	query($owner: String!, $repo: String!, $number: Int!) {
		repository(owner: $owner, name: $repo) {
			pullRequest(number: $number) {
				id
				number
				title
				state
				isDraft
				url
				body
				createdAt
				updatedAt
				additions
				deletions
				changedFiles
				headRefName
				baseRefName
				headRefOid
				reviewDecision
				author { login }
				labels(first: 10) { nodes { name } }
			}
		}
	}`

	slog.Debug("fetching PR", "owner", c.owner, "repo", c.repo, "number", number)
	err := c.graphql.Do(query, map[string]interface{}{
		"owner":  c.owner,
		"repo":   c.repo,
		"number": number,
	}, &resp)
	if err != nil {
		slog.Error("failed to fetch PR", "number", number, "error", err)
		return nil, fmt.Errorf("failed to fetch PR #%d: %w", number, err)
	}

	pr := nodeToPR(resp.Repository.PullRequest, "")
	slog.Debug("fetched PR", "number", pr.Number, "nodeID", pr.NodeID, "title", pr.Title)
	return &pr, nil
}
