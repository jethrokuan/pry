package github

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// collaborator represents a GitHub user from the collaborators API.
type collaborator struct {
	Login string `json:"login"`
}

// ListMentionableUsers returns usernames that can be @mentioned in the repo.
// Implements review.Service. Results are cached (24h TTL).
func (c *Client) ListMentionableUsers(_ context.Context) ([]string, error) {
	var users []string
	if c.cache.Get("mentionable_users", &users) {
		return users, nil
	}

	users, err := c.fetchMentionableUsers()
	if err != nil {
		return nil, err
	}

	c.cache.Set("mentionable_users", users, 24*time.Hour)
	return users, nil
}

func (c *Client) fetchMentionableUsers() ([]string, error) {
	slog.Debug("fetching mentionable users", "owner", c.owner, "repo", c.repo)
	endpoint := fmt.Sprintf("repos/%s/%s/collaborators?per_page=100&page=%%d", c.owner, c.repo)
	collabs, err := paginateREST[collaborator](c.rest, endpoint)
	if err != nil {
		slog.Error("failed to fetch collaborators", "error", err)
		return nil, fmt.Errorf("failed to fetch collaborators: %w", err)
	}

	users := make([]string, len(collabs))
	for i, u := range collabs {
		users[i] = u.Login
	}
	slog.Debug("fetched mentionable users", "count", len(users))
	return users, nil
}
