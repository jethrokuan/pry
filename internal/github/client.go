package github

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	ghAPI "github.com/cli/go-gh/v2/pkg/api"

	"github.com/jethrokuan/pry/internal/cache"
)

// restClient abstracts the REST methods used by Client, enabling mock injection in tests.
type restClient interface {
	Get(path string, resp interface{}) error
	Do(method string, path string, body io.Reader, response interface{}) error
}

// graphqlClient abstracts the GraphQL methods used by Client, enabling mock injection in tests.
type graphqlClient interface {
	Do(query string, variables map[string]interface{}, response interface{}) error
}

// Client wraps the go-gh REST and GraphQL clients.
type Client struct {
	rest     restClient
	graphql  graphqlClient
	owner    string
	repo     string
	cache    cache.Cache
	prTTL    time.Duration // TTL for ListPRs/GetPR cache entries
	pageSize int           // Number of PRs to fetch per ListPRs call
}

// CurrentUser returns the authenticated user's login.
// Implements review.Service. Results are cached to disk (24h TTL).
func (c *Client) CurrentUser(_ context.Context) (string, error) {
	var login string
	if c.cache.Get("current_user", &login) {
		return login, nil
	}

	slog.Debug("fetching current user")
	var resp struct {
		Login string `json:"login"`
	}
	if err := c.rest.Get("user", &resp); err != nil {
		err = fmt.Errorf("failed to fetch current user: %w", err)
		slog.Error("failed to fetch current user", "error", err)
		return "", err
	}
	slog.Debug("fetched current user", "login", resp.Login)

	c.cache.Set("current_user", resp.Login, 24*time.Hour)
	return resp.Login, nil
}

// NewClient creates a new GitHub client for the given repository.
func NewClient(owner, repo string, c cache.Cache, prTTL time.Duration, pageSize int) (*Client, error) {
	rest, err := ghAPI.DefaultRESTClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create REST client (is gh authenticated?): %w", err)
	}
	graphql, err := ghAPI.DefaultGraphQLClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create GraphQL client: %w", err)
	}

	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 100 {
		pageSize = 100 // GitHub GraphQL search max
	}

	return &Client{
		rest:     rest,
		graphql:  graphql,
		owner:    owner,
		repo:     repo,
		cache:    c,
		prTTL:    prTTL,
		pageSize: pageSize,
	}, nil
}

// InvalidateListPRs clears cached PR list and detail entries.
// Implements review.CacheInvalidator.
func (c *Client) InvalidateListPRs() {
	c.cache.DeleteByPrefix("listprs__")
	c.cache.DeleteByPrefix("pr__")
}

// RepoOwner returns the current repo owner.
func (c *Client) RepoOwner() string { return c.owner }

// RepoName returns the current repo name.
func (c *Client) RepoName() string { return c.repo }
