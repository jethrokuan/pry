package github

import (
	"fmt"

	ghAPI "github.com/cli/go-gh/v2/pkg/api"
)

// Client wraps the go-gh REST and GraphQL clients.
type Client struct {
	rest    *ghAPI.RESTClient
	graphql *ghAPI.GraphQLClient
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

// paginateREST fetches all pages of a paginated REST endpoint.
// endpoint is the base URL without pagination params (e.g. "repos/o/r/pulls/1/files").
// The helper appends ?per_page=100&page=N automatically.
func paginateREST[T any](rest *ghAPI.RESTClient, endpoint string) ([]T, error) {
	var all []T
	page := 1

	for {
		url := fmt.Sprintf("%s?per_page=100&page=%d", endpoint, page)
		var batch []T
		if err := rest.Get(url, &batch); err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(batch) < 100 {
			break
		}
		page++
	}

	return all, nil
}
