package github

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jethrokuan/pry/internal/review"
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
	ReviewDecision   string `json:"reviewDecision"`
	MergeStateStatus string `json:"mergeStateStatus"`
	Mergeable        string `json:"mergeable"`
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
	Comments struct {
		TotalCount int `json:"totalCount"`
	} `json:"comments"`
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
	Commits struct {
		TotalCount int `json:"totalCount"`
		Nodes      []struct {
			Commit struct {
				StatusCheckRollup struct {
					State    string `json:"state"`
					Contexts struct {
						Nodes []graphqlCheckContext `json:"nodes"`
					} `json:"contexts"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
}

// graphqlCheckContext represents a union of CheckRun | StatusContext from the API.
type graphqlCheckContext struct {
	TypeName string `json:"__typename"`
	// CheckRun fields
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Conclusion  string     `json:"conclusion"`
	StartedAt   *time.Time `json:"startedAt"`
	CompletedAt *time.Time `json:"completedAt"`
	DetailsURL  string     `json:"detailsUrl"`
	// StatusContext fields
	Context   string     `json:"context"`
	State     string     `json:"state"`
	TargetURL string     `json:"targetUrl"`
	CreatedAt *time.Time `json:"createdAt"`
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

// listPRsCacheKey returns the cache key for a ListPRs qualifier.
func listPRsCacheKey(qualifier string) string {
	h := sha256.Sum256([]byte(qualifier))
	return fmt.Sprintf("listprs__%x", h[:8])
}

// ListPRs fetches PRs based on the given filter.
// If the qualifier contains @my-teams, it expands into one query per team
// the authenticated user belongs to (in the repo's org) and deduplicates.
func (c *Client) ListPRs(_ context.Context, filter review.PRFilter) ([]review.PullRequest, error) {
	key := listPRsCacheKey(filter.Qualifier)
	var cached []review.PullRequest
	if c.cache.Get(key, &cached) {
		return cached, nil
	}

	var prs []review.PullRequest
	var err error
	if strings.Contains(filter.Qualifier, myTeamsPlaceholder) {
		prs, err = c.listPRsForTeams(filter)
	} else {
		prs, err = c.searchPRs(filter.Qualifier)
	}
	if err != nil {
		return nil, err
	}

	c.cache.Set(key, prs, c.prTTL)
	return prs, nil
}

// searchPRs runs a single GitHub search query and returns matching PRs.
func (c *Client) searchPRs(qualifier string) ([]review.PullRequest, error) {
	query := fmt.Sprintf("is:pr is:open repo:%s/%s %s sort:updated-desc", c.owner, c.repo, qualifier)

	// Lightweight list query — only fields needed for the table rows.
	// Heavy nested data (check runs, reviews, review requests) are fetched
	// lazily via GetPR when the sidebar preview is shown.
	graphqlQuery := `
	query($query: String!) {
		viewer { login }
		search(query: $query, type: ISSUE, first: 30) {
			nodes {
				... on PullRequest {
					id
					number
					title
					state
					isDraft
					url
					createdAt
					updatedAt
					additions
					deletions
					changedFiles
					headRefName
					baseRefName
					reviewDecision
					mergeStateStatus
					mergeable
					author { login }
					comments { totalCount }
					commits(last: 1) {
						totalCount
						nodes {
							commit {
								statusCheckRollup {
									state
								}
							}
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

	// Cap concurrency to avoid firing too many heavy queries in parallel.
	const maxConcurrent = 5
	sem := make(chan struct{}, maxConcurrent)

	for i, team := range teams {
		wg.Add(1)
		go func(idx int, teamSlug string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
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

	// Build per-reviewer statuses.
	// Start with completed reviews (latestReviews), then add pending requests.
	reviewerMap := make(map[string]review.Reviewer)
	var myReviewState string
	for _, rv := range node.LatestReviews.Nodes {
		login := rv.Author.Login
		if login == "" {
			continue
		}
		reviewerMap[strings.ToLower(login)] = review.Reviewer{
			Login: login,
			State: rv.State,
		}
		if viewer != "" && strings.EqualFold(login, viewer) {
			myReviewState = rv.State
		}
	}
	// Add pending user reviewers (requested but haven't reviewed yet)
	for _, rr := range node.ReviewRequests.Nodes {
		r := rr.RequestedReviewer
		if r.Login != "" {
			key := strings.ToLower(r.Login)
			if _, exists := reviewerMap[key]; !exists {
				reviewerMap[key] = review.Reviewer{Login: r.Login, State: "PENDING"}
			}
		}
		if r.Slug != "" && r.Organization.Login != "" {
			key := strings.ToLower(r.Organization.Login + "/" + r.Slug)
			if _, exists := reviewerMap[key]; !exists {
				reviewerMap[key] = review.Reviewer{
					Login:  r.Slug,
					IsTeam: true,
					State:  "PENDING",
				}
			}
		}
	}
	reviewers := make([]review.Reviewer, 0, len(reviewerMap))
	for _, r := range reviewerMap {
		reviewers = append(reviewers, r)
	}

	// Extract CI status from the last commit's status check rollup
	var checksPass *bool
	var checkRuns []review.CheckRun
	if len(node.Commits.Nodes) > 0 {
		rollup := node.Commits.Nodes[len(node.Commits.Nodes)-1].Commit.StatusCheckRollup
		switch rollup.State {
		case "SUCCESS":
			t := true
			checksPass = &t
		case "ERROR", "FAILURE":
			f := false
			checksPass = &f
		case "PENDING", "EXPECTED":
			// Leave as nil (pending)
		}

		for _, ctx := range rollup.Contexts.Nodes {
			cr := graphqlContextToCheckRun(ctx)
			checkRuns = append(checkRuns, cr)
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
		Commits:        node.Commits.TotalCount,
		CommentCount:   node.Comments.TotalCount,
		Body:           node.Body,
		URL:            node.URL,
		HeadSHA:        node.HeadRefOid,
		ChecksPass:     checksPass,
		CheckRuns:      checkRuns,
		MergeState:     node.MergeStateStatus,
		Mergeable:      node.Mergeable,
		ReviewDecision: node.ReviewDecision,
		Reviewers:      reviewers,
		PendingTeams:   pendingTeams,
		MyReviewState:  myReviewState,
	}
}

// graphqlContextToCheckRun converts a GraphQL check context (CheckRun or StatusContext) to a domain CheckRun.
func graphqlContextToCheckRun(ctx graphqlCheckContext) review.CheckRun {
	cr := review.CheckRun{}

	if ctx.TypeName == "CheckRun" {
		cr.Name = ctx.Name
		cr.Status = review.CheckRunStatus(ctx.Status)
		cr.Conclusion = review.CheckRunConclusion(ctx.Conclusion)
		cr.DetailsURL = ctx.DetailsURL
		if ctx.StartedAt != nil {
			cr.StartedAt = *ctx.StartedAt
		}
		if ctx.CompletedAt != nil {
			cr.CompletedAt = *ctx.CompletedAt
		}
	} else {
		// StatusContext
		cr.Name = ctx.Context
		cr.DetailsURL = ctx.TargetURL
		if ctx.CreatedAt != nil {
			cr.StartedAt = *ctx.CreatedAt
		}
		// Map StatusContext states to CheckRun equivalents
		switch ctx.State {
		case "SUCCESS":
			cr.Status = review.CheckRunCompleted
			cr.Conclusion = review.ConclusionSuccess
		case "PENDING", "EXPECTED":
			cr.Status = review.CheckRunInProgress
		case "ERROR", "FAILURE":
			cr.Status = review.CheckRunCompleted
			cr.Conclusion = review.ConclusionFailure
		}
	}

	return cr
}

// GetPR fetches a single PR by number, including the full body.
func (c *Client) GetPR(_ context.Context, number int) (*review.PullRequest, error) {
	key := fmt.Sprintf("pr__%d", number)
	var cached review.PullRequest
	if c.cache.Get(key, &cached) {
		return &cached, nil
	}

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
				mergeStateStatus
				mergeable
				author { login }
				comments { totalCount }
				labels(first: 10) { nodes { name } }
				commits(last: 1) {
					totalCount
					nodes {
						commit {
							statusCheckRollup {
								state
								contexts(first: 25) {
									nodes {
										__typename
										... on CheckRun {
											name
											status
											conclusion
											startedAt
											completedAt
											detailsUrl
										}
										... on StatusContext {
											context
											state
											targetUrl
											createdAt
										}
									}
								}
							}
						}
					}
				}
				reviewRequests(first: 20) {
					nodes {
						requestedReviewer {
							... on Team { slug organization { login } }
							... on User { login }
						}
					}
				}
				latestReviews(first: 10) {
					nodes {
						author { login }
						state
					}
				}
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

	c.cache.Set(key, pr, c.prTTL)
	return &pr, nil
}
