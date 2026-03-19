package github

import (
	"context"
	"fmt"
	"time"

	"github.com/jkuan/pr-review/internal/review"
)

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
}

type graphqlPRResponse struct {
	Search struct {
		Nodes    []graphqlPRNode `json:"nodes"`
		PageInfo struct {
			HasNextPage bool   `json:"hasNextPage"`
			EndCursor   string `json:"endCursor"`
		} `json:"pageInfo"`
	} `json:"search"`
}

// ListPRs fetches PRs based on the given filter.
func (c *Client) ListPRs(_ context.Context, filter review.PRFilter) ([]review.PullRequest, error) {
	query := fmt.Sprintf("is:pr is:open repo:%s/%s %s sort:updated-desc", c.owner, c.repo, filter.Qualifier)

	// Minimal fields for the list view — details fetched on demand via GetPR.
	graphqlQuery := `
	query($query: String!) {
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
				}
			}
		}
	}`

	var resp graphqlPRResponse
	err := c.graphql.Do(graphqlQuery, map[string]interface{}{
		"query": query,
	}, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PRs: %w", err)
	}

	var prs []review.PullRequest
	for _, node := range resp.Search.Nodes {
		if node.Number == 0 {
			continue
		}
		prs = append(prs, nodeToPR(node))
	}

	return prs, nil
}

func nodeToPR(node graphqlPRNode) review.PullRequest {
	labels := make([]string, 0, len(node.Labels.Nodes))
	for _, l := range node.Labels.Nodes {
		labels = append(labels, l.Name)
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

	err := c.graphql.Do(query, map[string]interface{}{
		"owner":  c.owner,
		"repo":   c.repo,
		"number": number,
	}, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PR #%d: %w", number, err)
	}

	pr := nodeToPR(resp.Repository.PullRequest)
	return &pr, nil
}
