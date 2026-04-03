package review

// UserIdentity holds the authenticated user's login and team memberships.
type UserIdentity struct {
	Login string   // e.g. "@username"
	Teams []string // e.g. ["@org/team1", "@org/team2"]
}

// MentionableUser represents a user that can be @mentioned in the repo.
type MentionableUser struct {
	Login string // GitHub username (e.g. "octocat")
	Name  string // Display name (e.g. "The Octocat"), may be empty
}
