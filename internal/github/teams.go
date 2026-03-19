package github

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// team represents a GitHub team from the /user/teams API.
type team struct {
	Slug string `json:"slug"`
	Org  struct {
		Login string `json:"login"`
	} `json:"organization"`
}

// userTeamsCache holds cached team slugs for the repo's org.
type userTeamsCache struct {
	once  sync.Once
	teams []string // "org/team-slug" format
	err   error
}

// UserTeams returns the authenticated user's team slugs for the repo's org.
// Implements review.Service. Results are cached after the first call.
func (c *Client) UserTeams(_ context.Context) ([]string, error) {
	return c.getUserTeams()
}

// getUserTeams returns the authenticated user's team slugs for the repo's org.
// Results are cached after the first call.
func (c *Client) getUserTeams() ([]string, error) {
	c.teams.once.Do(func() {
		c.teams.teams, c.teams.err = c.fetchUserTeams()
	})
	return c.teams.teams, c.teams.err
}

// fetchUserTeams fetches all teams the authenticated user belongs to,
// filtered to the current repo's org.
func (c *Client) fetchUserTeams() ([]string, error) {
	allTeams, err := paginateREST[team](c.rest, "user/teams")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch user teams: %w", err)
	}

	var filtered []string
	for _, t := range allTeams {
		if strings.EqualFold(t.Org.Login, c.owner) {
			filtered = append(filtered, fmt.Sprintf("%s/%s", t.Org.Login, t.Slug))
		}
	}

	return filtered, nil
}
