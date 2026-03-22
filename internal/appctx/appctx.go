// Package appctx provides shared application context passed to all screens.
package appctx

import "github.com/jethrokuan/pry/internal/review"

// Context holds state shared across all screens.
// It is allocated once and passed by pointer, so updates are visible everywhere.
type Context struct {
	Svc              review.Service
	UserIdentity     *review.UserIdentity // populated async, may be nil initially
	MentionableUsers []string             // populated async at startup, may be nil initially
}
