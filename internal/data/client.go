// Package data provides GitHub API functions for the application.
// All functions operate on module-level clients initialized via Init().
package data

import (
	"fmt"
	"io"
	"log/slog"
	"time"

	ghAPI "github.com/cli/go-gh/v2/pkg/api"

	"github.com/jethrokuan/pry/internal/cache"
)

// restClient abstracts the REST methods we use, enabling mock injection in tests.
type restClient interface {
	Get(path string, resp any) error
	Do(method string, path string, body io.Reader, response any) error
}

// graphqlClient abstracts the GraphQL methods we use, enabling mock injection in tests.
type graphqlClient interface {
	Do(query string, variables map[string]any, response any) error
}

// Module-level state, initialized once via Init().
var (
	rest       restClient
	graphql    graphqlClient
	repoCache  cache.Cache
	owner      string
	repo       string
	prTTL      time.Duration
	pageSize   int
	apiTimeout time.Duration
)

// Init initializes the module-level GitHub clients. Must be called once at startup.
func Init(repoOwner, repoName string, c cache.Cache, ttl time.Duration, ps int, timeout time.Duration) error {
	opts := ghAPI.ClientOptions{Timeout: timeout}

	r, err := ghAPI.NewRESTClient(opts)
	if err != nil {
		return fmt.Errorf("failed to create REST client (is gh authenticated?): %w", err)
	}
	g, err := ghAPI.NewGraphQLClient(opts)
	if err != nil {
		return fmt.Errorf("failed to create GraphQL client: %w", err)
	}

	if ps <= 0 {
		ps = 50
	}
	if ps > 100 {
		ps = 100 // GitHub GraphQL search max
	}

	rest = r
	graphql = g
	repoCache = c
	owner = repoOwner
	repo = repoName
	prTTL = ttl
	pageSize = ps
	apiTimeout = timeout

	return nil
}

// RepoOwner returns the current repo owner.
func RepoOwner() string { return owner }

// RepoName returns the current repo name.
func RepoName() string { return repo }

// InvalidateListPRs clears cached PR list and detail entries.
func InvalidateListPRs() {
	repoCache.DeleteByPrefix("listprs__")
	repoCache.DeleteByPrefix("pr__")
}

// CurrentUser returns the authenticated user's login.
// Results are cached to disk (24h TTL).
func CurrentUser() (string, error) {
	var login string
	if repoCache.Get("current_user", &login) {
		return login, nil
	}

	slog.Debug("fetching current user")
	var resp struct {
		Login string `json:"login"`
	}
	if err := rest.Get("user", &resp); err != nil {
		err = fmt.Errorf("failed to fetch current user: %w", err)
		slog.Error("failed to fetch current user", "error", err)
		return "", err
	}
	slog.Debug("fetched current user", "login", resp.Login)

	repoCache.Set("current_user", resp.Login, 24*time.Hour)
	return resp.Login, nil
}
