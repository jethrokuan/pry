package github

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/jethrokuan/pry/internal/git"
	"github.com/jethrokuan/pry/internal/review"
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
	ReviewDecision   string `json:"reviewDecision"`
	MergeStateStatus string `json:"mergeStateStatus"`
	Mergeable        string `json:"mergeable"`
	Assignees struct {
		Nodes []struct {
			Login string `json:"login"`
		} `json:"nodes"`
	} `json:"assignees"`
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
						TotalCount                 int                    `json:"totalCount"`
						CheckRunCount              int                    `json:"checkRunCount"`
						CheckRunCountsByState      []graphqlCountByState  `json:"checkRunCountsByState"`
						StatusContextCount         int                    `json:"statusContextCount"`
						StatusContextCountsByState []graphqlCountByState  `json:"statusContextCountsByState"`
						Nodes                      []graphqlCheckContext  `json:"nodes"`
						PageInfo                   graphqlPageInfo        `json:"pageInfo"`
					} `json:"contexts"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
}

// graphqlCountByState represents a count + state pair from checkRunCountsByState / statusContextCountsByState.
type graphqlCountByState struct {
	Count int    `json:"count"`
	State string `json:"state"`
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

// graphqlPageInfo holds cursor-based pagination state from GraphQL connections.
type graphqlPageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
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
		slog.Debug("ListPRs: cache hit", "qualifier", filter.Qualifier, "count", len(cached))
		return cached, nil
	}

	slog.Debug("ListPRs: cache miss, fetching", "qualifier", filter.Qualifier)
	var prs []review.PullRequest
	var err error
	if strings.Contains(filter.Qualifier, "@my-teams") {
		prs, err = c.searchPRsForMyTeams(filter.Qualifier)
	} else {
		prs, err = c.searchPRs(filter.Qualifier)
	}
	if err != nil {
		return nil, err
	}

	c.cache.Set(key, prs, c.prTTL)
	slog.Debug("ListPRs: fetched and cached", "qualifier", filter.Qualifier, "count", len(prs))
	return prs, nil
}

// searchPRsForMyTeams expands @my-teams in the qualifier into one search per
// team the authenticated user belongs to, then deduplicates and sorts results.
func (c *Client) searchPRsForMyTeams(qualifier string) ([]review.PullRequest, error) {
	teams, err := c.getUserTeams()
	if err != nil {
		return nil, err
	}
	if len(teams) == 0 {
		slog.Debug("searchPRsForMyTeams: no teams, returning empty")
		return nil, nil
	}

	seen := make(map[int]bool)
	var result []review.PullRequest
	for _, team := range teams {
		// team is "org/slug"; GitHub qualifier expects "@org/slug"
		expanded := strings.ReplaceAll(qualifier, "@my-teams", "@"+team)
		prs, err := c.searchPRs(expanded)
		if err != nil {
			return nil, err
		}
		for _, pr := range prs {
			if !seen[pr.Number] {
				seen[pr.Number] = true
				result = append(result, pr)
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].UpdatedAt.After(result[j].UpdatedAt)
	})
	return result, nil
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
					reviewRequests(first: 100) {
						nodes {
							requestedReviewer {
								... on Team {
									slug
									organization { login }
								}
								... on User {
									login
								}
							}
						}
					}
					latestReviews(first: 20) {
						nodes {
							author { login }
							state
						}
					}
					commits(last: 1) {
						totalCount
						nodes {
							commit {
								statusCheckRollup {
									state
									contexts {
										totalCount
										checkRunCount
										checkRunCountsByState { count state }
										statusContextCount
										statusContextCountsByState { count state }
									}
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

// nodeToPR converts a GraphQL node to a domain PullRequest.
// viewer is the authenticated user's login (used to extract their review state);
// pass "" if unknown.
func nodeToPR(node graphqlPRNode, viewer string) review.PullRequest {
	labels := make([]string, 0, len(node.Labels.Nodes))
	for _, l := range node.Labels.Nodes {
		labels = append(labels, l.Name)
	}

	assignees := make([]string, 0, len(node.Assignees.Nodes))
	for _, a := range node.Assignees.Nodes {
		assignees = append(assignees, a.Login)
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
	var checksTotal int
	var checkCounts review.CheckCounts
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

		checksTotal = rollup.Contexts.TotalCount
		for _, ctx := range rollup.Contexts.Nodes {
			cr := graphqlContextToCheckRun(ctx)
			checkRuns = append(checkRuns, cr)
		}

		checkCounts = aggregateCountsByState(
			rollup.Contexts.CheckRunCountsByState,
			rollup.Contexts.StatusContextCountsByState,
			checksTotal,
		)
	}

	pr := review.PullRequest{
		NodeID:         node.ID,
		Number:         node.Number,
		Title:          node.Title,
		Author:         node.Author.Login,
		Branch:         node.HeadRefName,
		Base:           node.BaseRefName,
		State:          node.State,
		Draft:          node.IsDraft,
		Labels:         labels,
		Assignees:      assignees,
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
		ChecksTotal:    checksTotal,
		CheckCounts:    checkCounts,
		MergeState:     node.MergeStateStatus,
		Mergeable:      node.Mergeable,
		ReviewDecision: node.ReviewDecision,
		Reviewers:      reviewers,
		PendingTeams:   pendingTeams,
		MyReviewState:  myReviewState,
	}

	// Log all merge-status-related fields for debugging.
	slog.Debug("nodeToPR: merge status",
		"number", pr.Number,
		"title", pr.Title,
		"mergeState", pr.MergeState,
		"mergeable", pr.Mergeable,
		"reviewDecision", pr.ReviewDecision,
		"draft", pr.Draft,
		"checksPass", pr.ChecksPass,
		"checkRunCount", len(pr.CheckRuns),
		"reviewerCount", len(pr.Reviewers),
		"pendingTeams", pr.PendingTeams,
		"myReviewState", pr.MyReviewState,
		"viewer", viewer,
	)
	if len(node.Commits.Nodes) > 0 {
		rollupState := node.Commits.Nodes[len(node.Commits.Nodes)-1].Commit.StatusCheckRollup.State
		slog.Debug("nodeToPR: rollup state",
			"number", pr.Number,
			"rollupState", rollupState,
		)
	}
	for _, cr := range pr.CheckRuns {
		slog.Debug("nodeToPR: check run",
			"number", pr.Number,
			"name", cr.Name,
			"status", cr.Status,
			"conclusion", cr.Conclusion,
		)
	}
	for _, rv := range pr.Reviewers {
		slog.Debug("nodeToPR: reviewer",
			"number", pr.Number,
			"login", rv.Login,
			"state", rv.State,
			"isTeam", rv.IsTeam,
		)
	}

	return pr
}

// graphqlContextToCheckRun converts a GraphQL check context (CheckRun or StatusContext) to a domain CheckRun.
func graphqlContextToCheckRun(ctx graphqlCheckContext) review.CheckRun {
	slog.Debug("graphqlContextToCheckRun: raw context",
		"typename", ctx.TypeName,
		"name", ctx.Name,
		"status", ctx.Status,
		"conclusion", ctx.Conclusion,
		"context", ctx.Context,
		"state", ctx.State,
	)
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

// aggregateCountsByState combines checkRunCountsByState and statusContextCountsByState
// into a single CheckCounts summary.
func aggregateCountsByState(crCounts, scCounts []graphqlCountByState, total int) review.CheckCounts {
	cc := review.CheckCounts{Total: total}
	for _, c := range crCounts {
		switch c.State {
		case "SUCCESS":
			cc.Passing += c.Count
		case "FAILURE", "TIMED_OUT", "STARTUP_FAILURE", "ACTION_REQUIRED", "CANCELLED":
			cc.Failing += c.Count
		case "SKIPPED", "NEUTRAL":
			cc.Skipped += c.Count
		case "IN_PROGRESS", "QUEUED", "PENDING", "WAITING":
			cc.Pending += c.Count
		}
	}
	for _, c := range scCounts {
		switch c.State {
		case "SUCCESS":
			cc.Passing += c.Count
		case "ERROR", "FAILURE":
			cc.Failing += c.Count
		case "PENDING", "EXPECTED":
			cc.Pending += c.Count
		}
	}
	return cc
}

// GetPR fetches a single PR by number, including the full body.
func (c *Client) GetPR(_ context.Context, number int) (*review.PullRequest, error) {
	key := fmt.Sprintf("pr__%d", number)
	var cached review.PullRequest
	if c.cache.Get(key, &cached) {
		slog.Debug("GetPR: cache hit",
			"number", number,
			"mergeState", cached.MergeState,
			"mergeable", cached.Mergeable,
			"reviewDecision", cached.ReviewDecision,
		)
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
				assignees(first: 20) { nodes { login } }
				comments { totalCount }
				labels(first: 10) { nodes { name } }
				commits(last: 1) {
					totalCount
					nodes {
						commit {
							statusCheckRollup {
								state
								contexts(first: 100) {
									totalCount
									checkRunCount
									checkRunCountsByState { count state }
									statusContextCount
									statusContextCountsByState { count state }
									pageInfo { hasNextPage endCursor }
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

	// Paginate remaining check contexts if there are more than the first page.
	node := resp.Repository.PullRequest
	if len(node.Commits.Nodes) > 0 {
		rollup := node.Commits.Nodes[len(node.Commits.Nodes)-1].Commit.StatusCheckRollup
		if rollup.Contexts.PageInfo.HasNextPage {
			extra, err := c.fetchRemainingCheckContexts(number, rollup.Contexts.PageInfo.EndCursor)
			if err != nil {
				slog.Warn("failed to paginate check contexts", "number", number, "error", err)
			} else {
				rollup.Contexts.Nodes = append(rollup.Contexts.Nodes, extra...)
				node.Commits.Nodes[len(node.Commits.Nodes)-1].Commit.StatusCheckRollup = rollup
			}
		}
	}

	pr := nodeToPR(node, "")

	// Best-effort: detect which files have merge conflicts using local git.
	if pr.Mergeable == "CONFLICTING" {
		pr.ConflictFiles = git.MergeConflictFiles("origin/"+pr.Base, pr.HeadSHA)
		slog.Debug("GetPR: conflict files",
			"number", number,
			"base", "origin/"+pr.Base,
			"headSHA", pr.HeadSHA,
			"conflictFiles", pr.ConflictFiles,
		)
	}

	c.cache.Set(key, pr, c.prTTL)
	slog.Debug("GetPR: cached result", "number", number)
	return &pr, nil
}

// fetchRemainingCheckContexts paginates through all remaining check contexts
// for a PR's last commit, starting after the given cursor.
func (c *Client) fetchRemainingCheckContexts(number int, after string) ([]graphqlCheckContext, error) {
	const pageSize = 100
	var all []graphqlCheckContext
	cursor := after

	for {
		var resp struct {
			Repository struct {
				PullRequest struct {
					Commits struct {
						Nodes []struct {
							Commit struct {
								StatusCheckRollup struct {
									Contexts struct {
										Nodes    []graphqlCheckContext `json:"nodes"`
										PageInfo graphqlPageInfo       `json:"pageInfo"`
									} `json:"contexts"`
								} `json:"statusCheckRollup"`
							} `json:"commit"`
						} `json:"nodes"`
					} `json:"commits"`
				} `json:"pullRequest"`
			} `json:"repository"`
		}

		query := `query($owner: String!, $repo: String!, $number: Int!, $after: String!) {
			repository(owner: $owner, name: $repo) {
				pullRequest(number: $number) {
					commits(last: 1) {
						nodes {
							commit {
								statusCheckRollup {
									contexts(first: 100, after: $after) {
										pageInfo { hasNextPage endCursor }
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
				}
			}
		}`

		err := c.graphql.Do(query, map[string]interface{}{
			"owner":  c.owner,
			"repo":   c.repo,
			"number": number,
			"after":  cursor,
		}, &resp)
		if err != nil {
			return all, fmt.Errorf("paginating check contexts: %w", err)
		}

		nodes := resp.Repository.PullRequest.Commits.Nodes
		if len(nodes) == 0 {
			break
		}
		contexts := nodes[len(nodes)-1].Commit.StatusCheckRollup.Contexts
		all = append(all, contexts.Nodes...)
		slog.Debug("paginated check contexts",
			"number", number,
			"fetched", len(contexts.Nodes),
			"total", len(all),
		)

		if !contexts.PageInfo.HasNextPage {
			break
		}
		cursor = contexts.PageInfo.EndCursor
	}

	return all, nil
}

// FetchCommits fetches individual commits for a PR.
func (c *Client) FetchCommits(_ context.Context, number int) ([]review.Commit, error) {
	key := fmt.Sprintf("commits__%d", number)
	var cached []review.Commit
	if c.cache.Get(key, &cached) {
		return cached, nil
	}

	query := `
	query($owner: String!, $repo: String!, $number: Int!) {
		repository(owner: $owner, name: $repo) {
			pullRequest(number: $number) {
				commits(last: 30) {
					nodes {
						commit {
							oid
							abbreviatedOid
							messageHeadline
							committedDate
							additions
							deletions
							authors(first: 1) {
								nodes {
									user { login }
									name
								}
							}
							statusCheckRollup {
								state
								contexts { totalCount }
							}
						}
					}
				}
			}
		}
	}`

	var resp struct {
		Repository struct {
			PullRequest struct {
				Commits struct {
					Nodes []struct {
						Commit struct {
							OID              string    `json:"oid"`
							AbbreviatedOID   string    `json:"abbreviatedOid"`
							MessageHeadline  string    `json:"messageHeadline"`
							CommittedDate    time.Time `json:"committedDate"`
							Additions        int       `json:"additions"`
							Deletions        int       `json:"deletions"`
							Authors          struct {
								Nodes []struct {
									User struct {
										Login string `json:"login"`
									} `json:"user"`
									Name string `json:"name"`
								} `json:"nodes"`
							} `json:"authors"`
							StatusCheckRollup struct {
								State    string `json:"state"`
								Contexts struct {
									TotalCount int `json:"totalCount"`
								} `json:"contexts"`
							} `json:"statusCheckRollup"`
						} `json:"commit"`
					} `json:"nodes"`
				} `json:"commits"`
			} `json:"pullRequest"`
		} `json:"repository"`
	}

	err := c.graphql.Do(query, map[string]interface{}{
		"owner":  c.owner,
		"repo":   c.repo,
		"number": number,
	}, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch commits for PR #%d: %w", number, err)
	}

	var commits []review.Commit
	for _, n := range resp.Repository.PullRequest.Commits.Nodes {
		c := n.Commit
		author := c.Authors.Nodes[0].User.Login
		if author == "" {
			author = c.Authors.Nodes[0].Name
		}

		var checksPass *bool
		switch c.StatusCheckRollup.State {
		case "SUCCESS":
			t := true
			checksPass = &t
		case "ERROR", "FAILURE":
			f := false
			checksPass = &f
		}

		commits = append(commits, review.Commit{
			SHA:         c.OID,
			ShortSHA:    c.AbbreviatedOID,
			Message:     c.MessageHeadline,
			Author:      author,
			CommittedAt: c.CommittedDate,
			Additions:   c.Additions,
			Deletions:   c.Deletions,
			ChecksPass:  checksPass,
			ChecksTotal: c.StatusCheckRollup.Contexts.TotalCount,
		})
	}

	c.cache.Set(key, commits, c.prTTL)
	return commits, nil
}
