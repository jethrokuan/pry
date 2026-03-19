package github

import (
	"fmt"
	"io"

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

// Client wraps the go-gh REST and GraphQL clients.
type Client struct {
	rest    restClient
	graphql graphqlClient
	owner   string
	repo    string
	teams   userTeamsCache
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
