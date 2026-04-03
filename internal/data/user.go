package data

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jethrokuan/pry/internal/review"
)

// UserTeams returns the authenticated user's team slugs for the repo's org.
// Results are cached (1h TTL).
func UserTeams() ([]string, error) {
	var teams []string
	if repoCache.Get("user_teams", &teams) {
		slog.Debug("UserTeams: cache hit", "teams", teams)
		return teams, nil
	}

	teams, err := fetchUserTeams()
	if err != nil {
		return nil, err
	}

	repoCache.Set("user_teams", teams, time.Hour)
	return teams, nil
}

func fetchUserTeams() ([]string, error) {
	type team struct {
		Slug string `json:"slug"`
		Org  struct {
			Login string `json:"login"`
		} `json:"organization"`
	}

	slog.Debug("fetching user teams", "org", owner)
	allTeams, err := paginateREST[team]("user/teams?per_page=100&page=%d")
	if err != nil {
		slog.Error("failed to fetch user teams", "error", err)
		return nil, fmt.Errorf("failed to fetch user teams: %w", err)
	}

	var filtered []string
	for _, t := range allTeams {
		if strings.EqualFold(t.Org.Login, owner) {
			filtered = append(filtered, fmt.Sprintf("%s/%s", t.Org.Login, t.Slug))
		}
	}

	slog.Debug("fetched user teams", "total", len(allTeams), "orgFiltered", len(filtered), "teams", filtered)
	return filtered, nil
}

// ListMentionableUsers returns users that can be @mentioned in the repo.
// Results are cached (24h TTL).
func ListMentionableUsers() ([]review.MentionableUser, error) {
	var users []review.MentionableUser
	if repoCache.Get("mentionable_users", &users) {
		return users, nil
	}

	users, err := fetchMentionableUsers()
	if err != nil {
		return nil, err
	}

	repoCache.Set("mentionable_users", users, 24*time.Hour)
	return users, nil
}

func fetchMentionableUsers() ([]review.MentionableUser, error) {
	slog.Debug("fetching mentionable users", "owner", owner, "repo", repo)

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
		vars := map[string]any{
			"owner": owner,
			"repo":  repo,
			"first": 100,
			"after": after,
		}

		if err := graphql.Do(query, vars, &resp); err != nil {
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
