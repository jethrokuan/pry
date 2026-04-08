package data

import (
	"crypto/sha256"
	"fmt"
	"log/slog"
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
	BaseRefOid     string    `json:"baseRefOid"`
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

type graphqlCountByState struct {
	Count int    `json:"count"`
	State string `json:"state"`
}

type graphqlCheckContext struct {
	TypeName    string     `json:"__typename"`
	DatabaseID  int64      `json:"databaseId"`
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Conclusion  string     `json:"conclusion"`
	StartedAt   *time.Time `json:"startedAt"`
	CompletedAt *time.Time `json:"completedAt"`
	DetailsURL  string     `json:"detailsUrl"`
	Context     string     `json:"context"`
	State       string     `json:"state"`
	TargetURL   string     `json:"targetUrl"`
	CreatedAt   *time.Time `json:"createdAt"`
}

type graphqlPageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

type graphqlReviewer struct {
	Slug         string `json:"slug"`
	Organization struct {
		Login string `json:"login"`
	} `json:"organization"`
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

func listPRsCacheKey(qualifier string) string {
	h := sha256.Sum256([]byte(qualifier))
	return fmt.Sprintf("listprs__%x", h[:8])
}

// FetchPullRequests fetches PRs based on the given filter.
func FetchPullRequests(filter review.PRFilter) ([]review.PullRequest, error) {
	key := listPRsCacheKey(filter.Qualifier)
	var cached []review.PullRequest
	if repoCache.Get(key, &cached) {
		slog.Debug("FetchPullRequests: cache hit", "qualifier", filter.Qualifier, "count", len(cached))
		return cached, nil
	}

	slog.Debug("FetchPullRequests: cache miss, fetching", "qualifier", filter.Qualifier)
	prs, err := searchPRs(filter.Qualifier)
	if err != nil {
		return nil, err
	}

	repoCache.Set(key, prs, prTTL)
	slog.Debug("FetchPullRequests: fetched and cached", "qualifier", filter.Qualifier, "count", len(prs))
	return prs, nil
}

func searchPRs(qualifier string) ([]review.PullRequest, error) {
	query := fmt.Sprintf("is:pr repo:%s/%s %s sort:updated-desc", owner, repo, qualifier)

	graphqlQuery := fmt.Sprintf(`
	query($query: String!) {
		viewer { login }
		search(query: $query, type: ISSUE, first: %d) {
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
				}
			}
		}
	}`, pageSize)

	slog.Debug("searching PRs", "query", query)
	var resp graphqlPRResponse
	err := graphql.Do(graphqlQuery, map[string]any{
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
		BaseSHA:        node.BaseRefOid,
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
		cr.ID = ctx.DatabaseID
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
		cr.Name = ctx.Context
		cr.DetailsURL = ctx.TargetURL
		if ctx.CreatedAt != nil {
			cr.StartedAt = *ctx.CreatedAt
		}
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

// FetchPR fetches a single PR by number with full details.
func FetchPR(number int) (*review.PullRequest, error) {
	key := fmt.Sprintf("pr__%d", number)
	var cached review.PullRequest
	if repoCache.Get(key, &cached) {
		slog.Debug("FetchPR: cache hit",
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
				baseRefOid
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
											databaseId
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

	slog.Debug("fetching PR", "owner", owner, "repo", repo, "number", number)
	err := graphql.Do(query, map[string]any{
		"owner":  owner,
		"repo":   repo,
		"number": number,
	}, &resp)
	if err != nil {
		slog.Error("failed to fetch PR", "number", number, "error", err)
		return nil, fmt.Errorf("failed to fetch PR #%d: %w", number, err)
	}

	node := resp.Repository.PullRequest
	if len(node.Commits.Nodes) > 0 {
		rollup := node.Commits.Nodes[len(node.Commits.Nodes)-1].Commit.StatusCheckRollup
		if rollup.Contexts.PageInfo.HasNextPage {
			extra, err := fetchRemainingCheckContexts(number, rollup.Contexts.PageInfo.EndCursor)
			if err != nil {
				slog.Warn("failed to paginate check contexts", "number", number, "error", err)
			} else {
				rollup.Contexts.Nodes = append(rollup.Contexts.Nodes, extra...)
				node.Commits.Nodes[len(node.Commits.Nodes)-1].Commit.StatusCheckRollup = rollup
			}
		}
	}

	pr := nodeToPR(node, "")
	pr.Enriched = true

	if pr.Mergeable == "CONFLICTING" {
		pr.ConflictFiles = git.MergeConflictFiles("origin/"+pr.Base, pr.HeadSHA)
		slog.Debug("FetchPR: conflict files",
			"number", number,
			"base", "origin/"+pr.Base,
			"headSHA", pr.HeadSHA,
			"conflictFiles", pr.ConflictFiles,
		)
	}

	repoCache.Set(key, pr, prTTL)
	slog.Debug("FetchPR: cached result", "number", number)
	return &pr, nil
}

func fetchRemainingCheckContexts(number int, after string) ([]graphqlCheckContext, error) {
	const ctxPageSize = 100
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
												databaseId
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

		err := graphql.Do(query, map[string]any{
			"owner":  owner,
			"repo":   repo,
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
func FetchCommits(number int) ([]review.Commit, error) {
	key := fmt.Sprintf("commits__%d", number)
	var cached []review.Commit
	if repoCache.Get(key, &cached) {
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

	err := graphql.Do(query, map[string]any{
		"owner":  owner,
		"repo":   repo,
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

	repoCache.Set(key, commits, prTTL)
	return commits, nil
}
