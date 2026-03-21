package codeowners

import (
	"os"
	"strings"

	"github.com/hmarr/codeowners"
)

// Codeowners holds parsed CODEOWNERS rules.
type Codeowners struct {
	ruleset codeowners.Ruleset
}

// Parse reads a CODEOWNERS file and returns the parsed rules.
func Parse(path string) (*Codeowners, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	ruleset, err := codeowners.ParseFile(f)
	if err != nil {
		return nil, err
	}
	return &Codeowners{ruleset: ruleset}, nil
}

// Find locates and parses a CODEOWNERS file from standard locations.
// Returns nil if no file is found.
func Find(repoRoot string) *Codeowners {
	ruleset, err := codeowners.LoadFileFromStandardLocation()
	if err != nil {
		return nil
	}
	return &Codeowners{ruleset: ruleset}
}

// Owners returns the owners for a given file path (last matching rule wins).
// Returns nil if no rule matches.
func (c *Codeowners) Owners(path string) []string {
	if c == nil {
		return nil
	}
	rule, err := c.ruleset.Match(path)
	if err != nil || rule == nil {
		return nil
	}
	owners := make([]string, len(rule.Owners))
	for i, o := range rule.Owners {
		owners[i] = o.String()
	}
	return owners
}

// OwnedBy returns true if the file path is owned by the given owner.
// The owner string is compared case-insensitively.
func (c *Codeowners) OwnedBy(path, owner string) bool {
	owners := c.Owners(path)
	lower := strings.ToLower(owner)
	for _, o := range owners {
		if strings.ToLower(o) == lower {
			return true
		}
	}
	return false
}

// OwnedByAny returns true if the file path is owned by any of the given owners.
// Each owner is compared case-insensitively.
func (c *Codeowners) OwnedByAny(path string, candidates []string) bool {
	owners := c.Owners(path)
	if len(owners) == 0 || len(candidates) == 0 {
		return false
	}
	for _, o := range owners {
		lower := strings.ToLower(o)
		for _, candidate := range candidates {
			if strings.ToLower(candidate) == lower {
				return true
			}
		}
	}
	return false
}
