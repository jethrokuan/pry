package github

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// mentionableUsersCache holds cached mentionable usernames.
type mentionableUsersCache struct {
	once  sync.Once
	users []string
	err   error
}

// collaborator represents a GitHub user from the collaborators API.
type collaborator struct {
	Login string `json:"login"`
}

// ListMentionableUsers returns usernames that can be @mentioned in the repo.
// Implements review.Service. Results are cached after the first call.
func (c *Client) ListMentionableUsers(_ context.Context) ([]string, error) {
	c.mentionable.once.Do(func() {
		c.mentionable.users, c.mentionable.err = c.fetchMentionableUsers()
	})
	return c.mentionable.users, c.mentionable.err
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
