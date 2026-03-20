package github

import (
	"context"
	"fmt"
	"io"
	"sync"

	ghAPI "github.com/cli/go-gh/v2/pkg/api"
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

// currentUserCache holds cached user login.
type currentUserCache struct {
	once  sync.Once
	login string
	err   error
}

// Client wraps the go-gh REST and GraphQL clients.
type Client struct {
	rest    restClient
	graphql graphqlClient
	owner   string
	repo    string
	teams   userTeamsCache
	user    currentUserCache
}

// CurrentUser returns the authenticated user's login.
// Implements review.Service. Results are cached after the first call.
func (c *Client) CurrentUser(_ context.Context) (string, error) {
	c.user.once.Do(func() {
		var resp struct {
			Login string `json:"login"`
		}
		if err := c.rest.Get("user", &resp); err != nil {
			c.user.err = fmt.Errorf("failed to fetch current user: %w", err)
			return
		}
		c.user.login = resp.Login
	})
	return c.user.login, c.user.err
}

// NewClient creates a new GitHub client for the given repository.
func NewClient(owner, repo string) (*Client, error) {
	rest, err := ghAPI.DefaultRESTClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create REST client (is gh authenticated?): %w", err)
	}
	graphql, err := ghAPI.DefaultGraphQLClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create GraphQL client: %w", err)
	}

	return &Client{
		rest:    rest,
		graphql: graphql,
		owner:   owner,
		repo:    repo,
	}, nil
}

// RepoOwner returns the current repo owner.
func (c *Client) RepoOwner() string { return c.owner }

// RepoName returns the current repo name.
func (c *Client) RepoName() string { return c.repo }
