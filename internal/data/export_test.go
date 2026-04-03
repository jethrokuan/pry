package data

import (
	"github.com/jethrokuan/pry/internal/cache"
)

// ResetForTest injects mock clients and repo info for testing.
// Only available in test builds.
func ResetForTest(r restClient, g graphqlClient, c cache.Cache, o, rp string) {
	rest = r
	graphql = g
	repoCache = c
	owner = o
	repo = rp
	prTTL = 0
	pageSize = 30
}
