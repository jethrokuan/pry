package github

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jethrokuan/pry/internal/cache"
	"github.com/jethrokuan/pry/internal/review"
)

// --- Mock REST client ---

type restCall struct {
	Method string
	Path   string
	Body   []byte // captured from io.Reader for Do calls
}

type mockREST struct {
	// getHandler is called for Get requests; it receives the path and should
	// populate resp via json.Unmarshal or return an error.
	getHandler func(path string, resp interface{}) error

	// doHandler is called for Do requests.
	doHandler func(method, path string, body io.Reader, resp interface{}) error

	mu    sync.Mutex
	calls []restCall
}

func (m *mockREST) Get(path string, resp interface{}) error {
	m.mu.Lock()
	m.calls = append(m.calls, restCall{Method: "GET", Path: path})
	m.mu.Unlock()
	if m.getHandler != nil {
		return m.getHandler(path, resp)
	}
	return nil
}

func (m *mockREST) Do(method, path string, body io.Reader, resp interface{}) error {
	var bodyBytes []byte
	if body != nil {
		bodyBytes, _ = io.ReadAll(body)
	}
	m.mu.Lock()
	m.calls = append(m.calls, restCall{Method: method, Path: path, Body: bodyBytes})
	m.mu.Unlock()
	if m.doHandler != nil {
		return m.doHandler(method, path, nil, resp)
	}
	return nil
}

// --- Mock GraphQL client ---

type gqlCall struct {
	Query     string
	Variables map[string]interface{}
}

type mockGraphQL struct {
	handler func(query string, vars map[string]interface{}, resp interface{}) error

	mu    sync.Mutex
	calls []gqlCall
}

func (m *mockGraphQL) Do(query string, vars map[string]interface{}, resp interface{}) error {
	m.mu.Lock()
	m.calls = append(m.calls, gqlCall{Query: query, Variables: vars})
	m.mu.Unlock()
	if m.handler != nil {
		return m.handler(query, vars, resp)
	}
	return nil
}

// newTestClient creates a Client with mock REST and GraphQL clients and a noop cache.
func newTestClient(rest *mockREST, gql *mockGraphQL) *Client {
	return &Client{
		rest:    rest,
		graphql: gql,
		owner:   "testowner",
		repo:    "testrepo",
		cache:   cache.Noop{},
	}
}

// jsonInto marshals src to JSON and unmarshals into dst (simulates API response population).
func jsonInto(src interface{}, dst interface{}) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

var _ = Describe("ListPRs", func() {
	var (
		rest *mockREST
		gql  *mockGraphQL
		c    *Client
		ctx  context.Context
	)

	BeforeEach(func() {
		rest = &mockREST{}
		gql = &mockGraphQL{}
		c = newTestClient(rest, gql)
		ctx = context.Background()
	})

	It("returns PRs from a simple qualifier", func() {
		gql.handler = func(query string, vars map[string]interface{}, resp interface{}) error {
			return jsonInto(graphqlPRResponse{
				Viewer: struct {
					Login string `json:"login"`
				}{Login: "me"},
				Search: struct {
					Nodes    []graphqlPRNode `json:"nodes"`
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
				}{
					Nodes: []graphqlPRNode{
						{Number: 1, Title: "First PR", Author: struct {
							Login string `json:"login"`
						}{Login: "alice"}},
						{Number: 2, Title: "Second PR", Author: struct {
							Login string `json:"login"`
						}{Login: "bob"}},
					},
				},
			}, resp)
		}

		prs, err := c.ListPRs(ctx, review.PRFilter{Name: "test", Qualifier: "review-requested:me"})
		Expect(err).NotTo(HaveOccurred())
		Expect(prs).To(HaveLen(2))
		Expect(prs[0].Number).To(Equal(1))
		Expect(prs[0].Title).To(Equal("First PR"))
		Expect(prs[0].Author).To(Equal("alice"))
		Expect(prs[1].Number).To(Equal(2))
	})

	It("skips nodes with Number == 0", func() {
		gql.handler = func(query string, vars map[string]interface{}, resp interface{}) error {
			return jsonInto(graphqlPRResponse{
				Viewer: struct {
					Login string `json:"login"`
				}{Login: "me"},
				Search: struct {
					Nodes    []graphqlPRNode `json:"nodes"`
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
				}{
					Nodes: []graphqlPRNode{
						{Number: 0}, // empty/invalid node
						{Number: 5, Title: "Valid PR"},
					},
				},
			}, resp)
		}

		prs, err := c.ListPRs(ctx, review.PRFilter{Qualifier: "is:open"})
		Expect(err).NotTo(HaveOccurred())
		Expect(prs).To(HaveLen(1))
		Expect(prs[0].Number).To(Equal(5))
	})

	It("returns an error when GraphQL fails", func() {
		gql.handler = func(query string, vars map[string]interface{}, resp interface{}) error {
			return errors.New("network error")
		}

		_, err := c.ListPRs(ctx, review.PRFilter{Qualifier: "is:open"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to fetch PRs"))
	})

	It("builds the correct search query string", func() {
		gql.handler = func(query string, vars map[string]interface{}, resp interface{}) error {
			return jsonInto(graphqlPRResponse{}, resp)
		}

		_, _ = c.ListPRs(ctx, review.PRFilter{Qualifier: "review-requested:me"})
		Expect(gql.calls).To(HaveLen(1))
		q := gql.calls[0].Variables["query"].(string)
		Expect(q).To(ContainSubstring("is:pr"))
		Expect(q).To(ContainSubstring("is:open"))
		Expect(q).To(ContainSubstring("repo:testowner/testrepo"))
		Expect(q).To(ContainSubstring("review-requested:me"))
	})
})

var _ = Describe("nodeToPR", func() {
	It("converts labels correctly", func() {
		node := graphqlPRNode{
			Number: 1,
			Labels: struct {
				Nodes []struct {
					Name string `json:"name"`
				} `json:"nodes"`
			}{
				Nodes: []struct {
					Name string `json:"name"`
				}{
					{Name: "bug"},
					{Name: "urgent"},
				},
			},
		}
		pr := nodeToPR(node, "")
		Expect(pr.Labels).To(Equal([]string{"bug", "urgent"}))
	})

	It("extracts pending team review requests", func() {
		node := graphqlPRNode{
			Number: 1,
			ReviewRequests: struct {
				Nodes []struct {
					RequestedReviewer graphqlReviewer `json:"requestedReviewer"`
				} `json:"nodes"`
			}{
				Nodes: []struct {
					RequestedReviewer graphqlReviewer `json:"requestedReviewer"`
				}{
					{RequestedReviewer: graphqlReviewer{
						Slug:         "backend",
						Organization: struct{ Login string `json:"login"` }{Login: "acme"},
					}},
					// User reviewer (no slug/org) should be ignored
					{RequestedReviewer: graphqlReviewer{Login: "someuser"}},
				},
			},
		}
		pr := nodeToPR(node, "")
		Expect(pr.PendingTeams).To(Equal([]string{"acme/backend"}))
	})

	It("extracts the viewer's review state", func() {
		node := graphqlPRNode{
			Number: 1,
			LatestReviews: struct {
				Nodes []struct {
					Author struct {
						Login string `json:"login"`
					} `json:"author"`
					State string `json:"state"`
				} `json:"nodes"`
			}{
				Nodes: []struct {
					Author struct {
						Login string `json:"login"`
					} `json:"author"`
					State string `json:"state"`
				}{
					{Author: struct {
						Login string `json:"login"`
					}{Login: "other"}, State: "COMMENTED"},
					{Author: struct {
						Login string `json:"login"`
					}{Login: "ME"}, State: "APPROVED"},
				},
			},
		}
		pr := nodeToPR(node, "me") // case-insensitive match
		Expect(pr.MyReviewState).To(Equal("APPROVED"))
	})

	It("leaves MyReviewState empty when viewer is not specified", func() {
		node := graphqlPRNode{Number: 1}
		pr := nodeToPR(node, "")
		Expect(pr.MyReviewState).To(BeEmpty())
	})
})

var _ = Describe("FetchDiffFiles", func() {
	var (
		rest *mockREST
		gql  *mockGraphQL
		c    *Client
		ctx  context.Context
	)

	BeforeEach(func() {
		rest = &mockREST{}
		gql = &mockGraphQL{}
		c = newTestClient(rest, gql)
		ctx = context.Background()
	})

	It("fetches files from a single page", func() {
		rest.getHandler = func(path string, resp interface{}) error {
			Expect(path).To(ContainSubstring("pulls/42/files"))
			return jsonInto([]prFile{
				{Filename: "main.go", Status: "modified", Additions: 5, Deletions: 2, Patch: "@@ -1,3 +1,6 @@\n context\n+added\n context"},
			}, resp)
		}

		files, err := c.FetchDiffFiles(ctx, 42)
		Expect(err).NotTo(HaveOccurred())
		Expect(files).To(HaveLen(1))
		Expect(files[0].Path).To(Equal("main.go"))
	})

	It("paginates when a page returns exactly 100 files", func() {
		callCount := 0
		rest.getHandler = func(path string, resp interface{}) error {
			callCount++
			if callCount == 1 {
				// First page: 100 files
				files := make([]prFile, 100)
				for i := range files {
					files[i] = prFile{Filename: "file" + string(rune('a'+i%26)), Status: "modified"}
				}
				return jsonInto(files, resp)
			}
			// Second page: fewer than 100 (end of pagination)
			return jsonInto([]prFile{
				{Filename: "last.go", Status: "added"},
			}, resp)
		}

		files, err := c.FetchDiffFiles(ctx, 42)
		Expect(err).NotTo(HaveOccurred())
		Expect(files).To(HaveLen(101))
		Expect(callCount).To(Equal(2))
	})

	It("returns an error on REST failure", func() {
		rest.getHandler = func(path string, resp interface{}) error {
			return errors.New("api error")
		}

		_, err := c.FetchDiffFiles(ctx, 42)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to fetch PR files"))
	})
})

var _ = Describe("FetchComments", func() {
	var (
		rest *mockREST
		gql  *mockGraphQL
		c    *Client
		ctx  context.Context
	)

	BeforeEach(func() {
		rest = &mockREST{}
		gql = &mockGraphQL{}
		c = newTestClient(rest, gql)
		ctx = context.Background()
	})

	It("fetches comments from a single page", func() {
		rest.getHandler = func(path string, resp interface{}) error {
			return jsonInto([]apiComment{
				{ID: 1, Path: "main.go", Line: 10, Side: "RIGHT", Body: "looks good", User: struct {
					Login string `json:"login"`
				}{Login: "alice"}, CreatedAt: "2024-01-01T00:00:00Z"},
			}, resp)
		}

		comments, err := c.FetchComments(ctx, 42)
		Expect(err).NotTo(HaveOccurred())
		Expect(comments).To(HaveLen(1))
		Expect(comments[0].ID).To(Equal(1))
		Expect(comments[0].Path).To(Equal("main.go"))
		Expect(comments[0].Author).To(Equal("alice"))
		Expect(comments[0].IsPending).To(BeFalse())
	})

	It("paginates when a page returns exactly 100 comments", func() {
		callCount := 0
		rest.getHandler = func(path string, resp interface{}) error {
			callCount++
			if callCount == 1 {
				batch := make([]apiComment, 100)
				for i := range batch {
					batch[i] = apiComment{ID: i + 1, Path: "f.go", Body: "comment"}
				}
				return jsonInto(batch, resp)
			}
			return jsonInto([]apiComment{
				{ID: 101, Path: "f.go", Body: "last"},
			}, resp)
		}

		comments, err := c.FetchComments(ctx, 42)
		Expect(err).NotTo(HaveOccurred())
		Expect(comments).To(HaveLen(101))
		Expect(callCount).To(Equal(2))
	})

	It("returns an error on REST failure", func() {
		rest.getHandler = func(path string, resp interface{}) error {
			return errors.New("500 error")
		}

		_, err := c.FetchComments(ctx, 42)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to fetch comments"))
	})
})

var _ = Describe("FetchPendingReview", func() {
	var (
		rest *mockREST
		gql  *mockGraphQL
		c    *Client
		ctx  context.Context
	)

	BeforeEach(func() {
		rest = &mockREST{}
		gql = &mockGraphQL{}
		c = newTestClient(rest, gql)
		ctx = context.Background()
	})

	It("returns zero values when no pending review exists", func() {
		rest.getHandler = func(path string, resp interface{}) error {
			return jsonInto([]apiReview{
				{ID: 1, State: "APPROVED"},
				{ID: 2, State: "COMMENTED"},
			}, resp)
		}

		id, nodeID, comments, err := c.FetchPendingReview(ctx, 42)
		Expect(err).NotTo(HaveOccurred())
		Expect(id).To(Equal(0))
		Expect(nodeID).To(BeEmpty())
		Expect(comments).To(BeNil())
	})

	It("returns the pending review with its comments", func() {
		callCount := 0
		rest.getHandler = func(path string, resp interface{}) error {
			callCount++
			if callCount == 1 {
				// Reviews list
				return jsonInto([]apiReview{
					{ID: 99, NodeID: "PRR_abc", State: "PENDING"},
				}, resp)
			}
			// Review comments
			return jsonInto([]apiComment{
				{ID: 10, Path: "file.go", Line: 5, Body: "pending comment", User: struct {
					Login string `json:"login"`
				}{Login: "me"}},
			}, resp)
		}

		id, nodeID, comments, err := c.FetchPendingReview(ctx, 42)
		Expect(err).NotTo(HaveOccurred())
		Expect(id).To(Equal(99))
		Expect(nodeID).To(Equal("PRR_abc"))
		Expect(comments).To(HaveLen(1))
		Expect(comments[0].IsPending).To(BeTrue())
	})
})

var _ = Describe("GetPR", func() {
	var (
		rest *mockREST
		gql  *mockGraphQL
		c    *Client
		ctx  context.Context
	)

	BeforeEach(func() {
		rest = &mockREST{}
		gql = &mockGraphQL{}
		c = newTestClient(rest, gql)
		ctx = context.Background()
	})

	It("fetches a single PR by number", func() {
		gql.handler = func(query string, vars map[string]interface{}, resp interface{}) error {
			Expect(vars["number"]).To(Equal(42))
			Expect(vars["owner"]).To(Equal("testowner"))
			Expect(vars["repo"]).To(Equal("testrepo"))

			return jsonInto(map[string]interface{}{
				"repository": map[string]interface{}{
					"pullRequest": map[string]interface{}{
						"number":      42,
						"title":       "Fix bug",
						"headRefName": "fix-bug",
						"baseRefName": "main",
						"author":      map[string]interface{}{"login": "alice"},
					},
				},
			}, resp)
		}

		pr, err := c.GetPR(ctx, 42)
		Expect(err).NotTo(HaveOccurred())
		Expect(pr.Number).To(Equal(42))
		Expect(pr.Title).To(Equal("Fix bug"))
		Expect(pr.Author).To(Equal("alice"))
		Expect(pr.Branch).To(Equal("fix-bug"))
	})

	It("returns an error on GraphQL failure", func() {
		gql.handler = func(query string, vars map[string]interface{}, resp interface{}) error {
			return errors.New("not found")
		}

		_, err := c.GetPR(ctx, 999)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to fetch PR #999"))
	})
})

var _ = Describe("SubmitReview", func() {
	var (
		rest *mockREST
		gql  *mockGraphQL
		c    *Client
		ctx  context.Context
	)

	BeforeEach(func() {
		rest = &mockREST{}
		gql = &mockGraphQL{}
		c = newTestClient(rest, gql)
		ctx = context.Background()
	})

	It("submits an existing pending review", func() {
		// ensurePendingReview fetches existing review via GET
		rest.getHandler = func(path string, resp interface{}) error {
			return jsonInto([]map[string]interface{}{
				{"id": 99, "node_id": "PRR_99", "state": "PENDING", "user": map[string]string{"login": "me"}},
			}, resp)
		}
		rest.doHandler = func(method, path string, body io.Reader, resp interface{}) error {
			Expect(method).To(Equal("POST"))
			Expect(path).To(ContainSubstring("reviews/99/events"))
			return nil
		}

		pr := &review.PullRequest{Number: 42, HeadSHA: "abc123"}
		rev := &review.PendingReview{
			ReviewID: 99,
			Event:    review.ReviewEventApprove,
			Body:     "LGTM",
		}
		err := c.SubmitReview(ctx, pr, rev)
		Expect(err).NotTo(HaveOccurred())
	})

	It("creates a pending review first when none exists", func() {
		// ensurePendingReview: no existing review
		rest.getHandler = func(path string, resp interface{}) error {
			return jsonInto([]map[string]interface{}{}, resp)
		}
		doCallCount := 0
		rest.doHandler = func(method, path string, body io.Reader, resp interface{}) error {
			doCallCount++
			Expect(method).To(Equal("POST"))
			if doCallCount == 1 {
				// CreatePendingReview
				Expect(path).To(Equal("repos/testowner/testrepo/pulls/42/reviews"))
				if resp != nil {
					raw, _ := json.Marshal(map[string]interface{}{"id": 77, "node_id": "PRR_77"})
					json.Unmarshal(raw, resp)
				}
			} else {
				// Submit the review
				Expect(path).To(ContainSubstring("reviews/77/events"))
			}
			return nil
		}

		pr := &review.PullRequest{Number: 42, HeadSHA: "abc123"}
		rev := &review.PendingReview{
			ReviewID: 0,
			Event:    review.ReviewEventComment,
		}
		err := c.SubmitReview(ctx, pr, rev)
		Expect(err).NotTo(HaveOccurred())
		Expect(doCallCount).To(Equal(2))
		Expect(rev.ReviewID).To(Equal(77))
	})

	It("returns an error on submission failure", func() {
		// ensurePendingReview succeeds
		rest.getHandler = func(path string, resp interface{}) error {
			return jsonInto([]map[string]interface{}{
				{"id": 99, "node_id": "PRR_99", "state": "PENDING", "user": map[string]string{"login": "me"}},
			}, resp)
		}
		rest.doHandler = func(method, path string, body io.Reader, resp interface{}) error {
			return errors.New("forbidden")
		}

		pr := &review.PullRequest{Number: 42}
		rev := &review.PendingReview{ReviewID: 99, Event: review.ReviewEventComment}
		err := c.SubmitReview(ctx, pr, rev)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to submit review"))
	})
})

var _ = Describe("UserTeams", func() {
	var (
		rest *mockREST
		gql  *mockGraphQL
		c    *Client
		ctx  context.Context
	)

	BeforeEach(func() {
		rest = &mockREST{}
		gql = &mockGraphQL{}
		c = newTestClient(rest, gql)
		ctx = context.Background()
	})

	It("returns teams filtered to the repo org", func() {
		rest.getHandler = func(path string, resp interface{}) error {
			return jsonInto([]team{
				{Slug: "backend", Org: struct {
					Login string `json:"login"`
				}{Login: "testowner"}},
				{Slug: "frontend", Org: struct {
					Login string `json:"login"`
				}{Login: "testowner"}},
				{Slug: "other", Org: struct {
					Login string `json:"login"`
				}{Login: "differentorg"}},
			}, resp)
		}

		teams, err := c.UserTeams(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(teams).To(Equal([]string{"testowner/backend", "testowner/frontend"}))
	})

	It("caches results after the first call", func() {
		dir := GinkgoT().TempDir()
		c = &Client{
			rest:    rest,
			graphql: gql,
			owner:   "testowner",
			repo:    "testrepo",
			cache:   cache.NewDisk(dir),
		}

		callCount := 0
		rest.getHandler = func(path string, resp interface{}) error {
			callCount++
			return jsonInto([]team{
				{Slug: "t1", Org: struct {
					Login string `json:"login"`
				}{Login: "testowner"}},
			}, resp)
		}

		teams1, err1 := c.UserTeams(ctx)
		teams2, err2 := c.UserTeams(ctx)
		Expect(err1).NotTo(HaveOccurred())
		Expect(err2).NotTo(HaveOccurred())
		Expect(teams1).To(Equal(teams2))
		Expect(callCount).To(Equal(1)) // Only called once due to disk cache
	})

	It("paginates teams", func() {
		callCount := 0
		rest.getHandler = func(path string, resp interface{}) error {
			callCount++
			if callCount == 1 {
				teams := make([]team, 100)
				for i := range teams {
					teams[i] = team{Slug: "other", Org: struct {
						Login string `json:"login"`
					}{Login: "differentorg"}}
				}
				// Put one matching team on page 1
				teams[0] = team{Slug: "page1-team", Org: struct {
					Login string `json:"login"`
				}{Login: "testowner"}}
				return jsonInto(teams, resp)
			}
			// Page 2: fewer than 100, ends pagination
			return jsonInto([]team{
				{Slug: "page2-team", Org: struct {
					Login string `json:"login"`
				}{Login: "testowner"}},
			}, resp)
		}

		teams, err := c.UserTeams(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(teams).To(Equal([]string{"testowner/page1-team", "testowner/page2-team"}))
		Expect(callCount).To(Equal(2))
	})

	It("returns an error on REST failure", func() {
		rest.getHandler = func(path string, resp interface{}) error {
			return errors.New("auth failed")
		}

		_, err := c.UserTeams(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to fetch user teams"))
	})
})

var _ = Describe("AddReviewComment", func() {
	var (
		rest *mockREST
		gql  *mockGraphQL
		c    *Client
		ctx  context.Context
	)

	// stubEnsurePendingReview sets up REST to return an existing PENDING review
	// so AddReviewComment can proceed to the GraphQL thread mutation.
	stubEnsurePendingReview := func() {
		rest.getHandler = func(path string, resp interface{}) error {
			return jsonInto([]map[string]interface{}{
				{"id": 42, "node_id": "PRR_42", "state": "PENDING", "user": map[string]string{"login": "me"}},
			}, resp)
		}
	}

	BeforeEach(func() {
		rest = &mockREST{}
		gql = &mockGraphQL{}
		c = newTestClient(rest, gql)
		ctx = context.Background()
	})

	It("uses cached reviewNodeID as fast path (no REST calls)", func() {
		gql.handler = func(query string, vars map[string]interface{}, resp interface{}) error {
			Expect(vars["reviewID"]).To(Equal("PRR_cached"))
			Expect(vars["path"]).To(Equal("main.go"))
			Expect(vars["line"]).To(Equal(10))
			Expect(vars["side"]).To(Equal("RIGHT"))

			return jsonInto(map[string]interface{}{
				"addPullRequestReviewThread": map[string]interface{}{
					"thread": map[string]interface{}{
						"comments": map[string]interface{}{
							"nodes": []map[string]interface{}{
								{"databaseId": 555},
							},
						},
					},
				},
			}, resp)
		}

		commentID, _, nodeID, err := c.AddReviewComment(ctx, 1, "PRR_cached", "main.go", 10, 0, "RIGHT", "nit: typo")
		Expect(err).NotTo(HaveOccurred())
		Expect(commentID).To(Equal(555))
		Expect(nodeID).To(Equal("PRR_cached"))
		// No REST calls should have been made (fast path)
		Expect(rest.calls).To(BeEmpty())
	})

	It("falls back to ensurePendingReview when no hint provided", func() {
		stubEnsurePendingReview()
		gql.handler = func(query string, vars map[string]interface{}, resp interface{}) error {
			Expect(vars["reviewID"]).To(Equal("PRR_42"))
			return jsonInto(map[string]interface{}{
				"addPullRequestReviewThread": map[string]interface{}{
					"thread": map[string]interface{}{
						"comments": map[string]interface{}{
							"nodes": []map[string]interface{}{
								{"databaseId": 556},
							},
						},
					},
				},
			}, resp)
		}

		commentID, reviewID, nodeID, err := c.AddReviewComment(ctx, 1, "", "main.go", 10, 0, "RIGHT", "nit: typo")
		Expect(err).NotTo(HaveOccurred())
		Expect(commentID).To(Equal(556))
		Expect(reviewID).To(Equal(42))
		Expect(nodeID).To(Equal("PRR_42"))
	})

	It("sets startLine for multi-line comments", func() {
		gql.handler = func(query string, vars map[string]interface{}, resp interface{}) error {
			Expect(vars["startLine"]).To(Equal(5))
			Expect(vars["startSide"]).To(Equal("RIGHT"))
			return jsonInto(map[string]interface{}{
				"addPullRequestReviewThread": map[string]interface{}{
					"thread": map[string]interface{}{
						"comments": map[string]interface{}{
							"nodes": []map[string]interface{}{
								{"databaseId": 556},
							},
						},
					},
				},
			}, resp)
		}

		_, _, _, err := c.AddReviewComment(ctx, 1, "PRR_hint", "main.go", 10, 5, "RIGHT", "refactor this block")
		Expect(err).NotTo(HaveOccurred())
	})

	It("returns an error when no comment node is returned", func() {
		gql.handler = func(query string, vars map[string]interface{}, resp interface{}) error {
			return jsonInto(map[string]interface{}{
				"addPullRequestReviewThread": map[string]interface{}{
					"thread": map[string]interface{}{
						"comments": map[string]interface{}{
							"nodes": []map[string]interface{}{},
						},
					},
				},
			}, resp)
		}

		// With hint, fast path returns "no comment" error; slow path also fails.
		stubEnsurePendingReview()
		_, _, _, err := c.AddReviewComment(ctx, 1, "PRR_hint", "main.go", 10, 0, "RIGHT", "test")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no comment returned"))
	})

	It("falls back to ensurePendingReview when hint is stale", func() {
		gqlCallCount := 0
		gql.handler = func(query string, vars map[string]interface{}, resp interface{}) error {
			gqlCallCount++
			if gqlCallCount == 1 {
				// First call with stale hint fails
				return errors.New("review not found")
			}
			// Second call with fresh node ID succeeds
			Expect(vars["reviewID"]).To(Equal("PRR_fresh"))
			return jsonInto(map[string]interface{}{
				"addPullRequestReviewThread": map[string]interface{}{
					"thread": map[string]interface{}{
						"comments": map[string]interface{}{
							"nodes": []map[string]interface{}{
								{"databaseId": 888},
							},
						},
					},
				},
			}, resp)
		}
		rest.getHandler = func(path string, resp interface{}) error {
			return jsonInto([]map[string]interface{}{
				{"id": 77, "node_id": "PRR_fresh", "state": "PENDING", "user": map[string]string{"login": "me"}},
			}, resp)
		}

		commentID, reviewID, nodeID, err := c.AddReviewComment(ctx, 1, "PRR_stale", "main.go", 5, 0, "RIGHT", "hello")
		Expect(err).NotTo(HaveOccurred())
		Expect(commentID).To(Equal(888))
		Expect(reviewID).To(Equal(77))
		Expect(nodeID).To(Equal("PRR_fresh"))
	})

	It("creates a new review when none exists and no hint provided", func() {
		// FetchPendingReview: return empty list (no PENDING review)
		rest.getHandler = func(path string, resp interface{}) error {
			return jsonInto([]map[string]interface{}{}, resp)
		}
		// CreatePendingReview: return new review
		rest.doHandler = func(method, path string, body io.Reader, resp interface{}) error {
			return jsonInto(map[string]interface{}{
				"id": 99, "node_id": "PRR_99",
			}, resp)
		}
		gql.handler = func(query string, vars map[string]interface{}, resp interface{}) error {
			Expect(vars["reviewID"]).To(Equal("PRR_99"))
			return jsonInto(map[string]interface{}{
				"addPullRequestReviewThread": map[string]interface{}{
					"thread": map[string]interface{}{
						"comments": map[string]interface{}{
							"nodes": []map[string]interface{}{
								{"databaseId": 777},
							},
						},
					},
				},
			}, resp)
		}

		commentID, reviewID, _, err := c.AddReviewComment(ctx, 1, "", "main.go", 5, 0, "RIGHT", "hello")
		Expect(err).NotTo(HaveOccurred())
		Expect(commentID).To(Equal(777))
		Expect(reviewID).To(Equal(99))
	})
})

var _ = Describe("DeleteReviewComment", func() {
	It("calls DELETE on the correct endpoint", func() {
		rest := &mockREST{}
		gql := &mockGraphQL{}
		c := newTestClient(rest, gql)

		rest.doHandler = func(method, path string, body io.Reader, resp interface{}) error {
			Expect(method).To(Equal("DELETE"))
			Expect(path).To(Equal("repos/testowner/testrepo/pulls/comments/123"))
			return nil
		}

		err := c.DeleteReviewComment(context.Background(), 42, 123)
		Expect(err).NotTo(HaveOccurred())
	})
})

var _ = Describe("EditReviewComment", func() {
	It("calls PATCH on the correct endpoint", func() {
		rest := &mockREST{}
		gql := &mockGraphQL{}
		c := newTestClient(rest, gql)

		rest.doHandler = func(method, path string, body io.Reader, resp interface{}) error {
			Expect(method).To(Equal("PATCH"))
			Expect(path).To(Equal("repos/testowner/testrepo/pulls/comments/456"))
			return nil
		}

		err := c.EditReviewComment(context.Background(), 42, 456, "updated body")
		Expect(err).NotTo(HaveOccurred())
	})
})
