package github

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// team represents a GitHub team from the /user/teams API.
type team struct {
	Slug string `json:"slug"`
	Org  struct {
		Login string `json:"login"`
	} `json:"organization"`
}

// UserTeams returns the authenticated user's team slugs for the repo's org.
// Implements review.Service. Results are cached (1h TTL).
func (c *Client) UserTeams(_ context.Context) ([]string, error) {
	return c.getUserTeams()
}

// getUserTeams returns the authenticated user's team slugs for the repo's org.
// Results are cached (1h TTL).
func (c *Client) getUserTeams() ([]string, error) {
	var teams []string
	if c.cache.Get("user_teams", &teams) {
		slog.Debug("getUserTeams: cache hit", "teams", teams)
		return teams, nil
	}

	teams, err := c.fetchUserTeams()
	if err != nil {
		return nil, err
	}

	c.cache.Set("user_teams", teams, time.Hour)
	return teams, nil
}

// fetchUserTeams fetches all teams the authenticated user belongs to,
// filtered to the current repo's org.
func (c *Client) fetchUserTeams() ([]string, error) {
	slog.Debug("fetching user teams", "org", c.owner)
	allTeams, err := paginateREST[team](c.rest, "user/teams?per_page=100&page=%d")
	if err != nil {
		slog.Error("failed to fetch user teams", "error", err)
		return nil, fmt.Errorf("failed to fetch user teams: %w", err)
	}

	var filtered []string
	for _, t := range allTeams {
		if strings.EqualFold(t.Org.Login, c.owner) {
			filtered = append(filtered, fmt.Sprintf("%s/%s", t.Org.Login, t.Slug))
		}
	}

	slog.Debug("fetched user teams", "total", len(allTeams), "orgFiltered", len(filtered), "teams", filtered)
	return filtered, nil
}
