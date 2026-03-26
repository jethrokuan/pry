package github

import (
	"context"
	"log/slog"
	"time"

	"github.com/jethrokuan/pry/internal/review"
)

// ListMentionableUsers returns users that can be @mentioned in the repo.
// Implements review.Service. Results are cached (24h TTL).
func (c *Client) ListMentionableUsers(_ context.Context) ([]review.MentionableUser, error) {
	var users []review.MentionableUser
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

func (c *Client) fetchMentionableUsers() ([]review.MentionableUser, error) {
	slog.Debug("fetching mentionable users", "owner", c.owner, "repo", c.repo)

	var resp struct {
		Repository struct {
			MentionableUsers struct {
				Nodes []struct {
					Login string `json:"login"`
					Name  string `json:"name"`
				} `json:"nodes"`
				PageInfo struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
			} `json:"mentionableUsers"`
		} `json:"repository"`
	}

	query := `query($owner: String!, $repo: String!, $first: Int!, $after: String) {
		repository(owner: $owner, name: $repo) {
			mentionableUsers(first: $first, after: $after) {
				nodes { login name }
				pageInfo { hasNextPage endCursor }
			}
		}
	}`

	var allUsers []review.MentionableUser
	var after *string

	for {
		vars := map[string]interface{}{
			"owner": c.owner,
			"repo":  c.repo,
			"first":  100,
			"after":  after,
		}

		if err := c.graphql.Do(query, vars, &resp); err != nil {
			slog.Error("failed to fetch mentionable users", "error", err)
			return nil, err
		}

		for _, n := range resp.Repository.MentionableUsers.Nodes {
			allUsers = append(allUsers, review.MentionableUser{
				Login: n.Login,
				Name:  n.Name,
			})
		}

		if !resp.Repository.MentionableUsers.PageInfo.HasNextPage {
			break
		}
		cursor := resp.Repository.MentionableUsers.PageInfo.EndCursor
		after = &cursor
	}

	slog.Debug("fetched mentionable users", "count", len(allUsers))
	return allUsers, nil
}
