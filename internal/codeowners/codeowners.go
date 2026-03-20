package codeowners

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Rule is a single CODEOWNERS rule: a pattern and its owners.
type Rule struct {
	Pattern string
	Owners  []string
}

// Codeowners holds parsed CODEOWNERS rules.
type Codeowners struct {
	Rules []Rule
}

// Find locates and parses a CODEOWNERS file relative to repoRoot.
// Checks standard locations: .github/CODEOWNERS, CODEOWNERS, docs/CODEOWNERS.
// Returns nil if no file is found.
func Find(repoRoot string) *Codeowners {
	candidates := []string{
		filepath.Join(repoRoot, ".github", "CODEOWNERS"),
		filepath.Join(repoRoot, "CODEOWNERS"),
		filepath.Join(repoRoot, "docs", "CODEOWNERS"),
	}
	for _, path := range candidates {
		co, err := Parse(path)
		if err == nil {
			return co
		}
	}
	return nil
}

// Parse reads a CODEOWNERS file and returns the parsed rules.
func Parse(path string) (*Codeowners, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var rules []Rule
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		rules = append(rules, Rule{
			Pattern: parts[0],
			Owners:  parts[1:],
		})
	}

	return &Codeowners{Rules: rules}, scanner.Err()
}

// Owners returns the owners for a given file path (last matching rule wins).
// Returns nil if no rule matches.
func (c *Codeowners) Owners(path string) []string {
	if c == nil {
		return nil
	}
	var owners []string
	for _, rule := range c.Rules {
		if matchPattern(rule.Pattern, path) {
			owners = rule.Owners
		}
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

// matchPattern matches a CODEOWNERS pattern against a file path.
// Supports: *, ?, **, directory patterns (trailing /).
func matchPattern(pattern, path string) bool {
	// Normalize: remove leading /
	if strings.HasPrefix(pattern, "/") {
		pattern = pattern[1:]
	}

	// Directory pattern: "dir/" matches everything under dir
	if strings.HasSuffix(pattern, "/") {
		dir := strings.TrimSuffix(pattern, "/")
		return path == dir || strings.HasPrefix(path, dir+"/")
	}

	// If pattern contains no slash, match against filename only
	if !strings.Contains(pattern, "/") {
		_, file := filepath.Split(path)
		return matchGlob(pattern, file)
	}

	// Pattern contains /, match against full path
	return matchGlob(pattern, path)
}

// matchGlob matches a pattern with *, ?, and ** support against a string.
func matchGlob(pattern, name string) bool {
	return doMatch(pattern, name)
}

func doMatch(pattern, name string) bool {
	for len(pattern) > 0 {
		switch pattern[0] {
		case '*':
			if len(pattern) > 1 && pattern[1] == '*' {
				// ** matches any number of path components
				rest := pattern[2:]
				if len(rest) > 0 && rest[0] == '/' {
					rest = rest[1:]
				}
				// Try matching rest against every suffix of name
				if doMatch(rest, name) {
					return true
				}
				for i := 0; i < len(name); i++ {
					if name[i] == '/' {
						if doMatch(rest, name[i+1:]) {
							return true
						}
					}
				}
				return false
			}
			// Single * matches everything except /
			rest := pattern[1:]
			for i := 0; i <= len(name); i++ {
				if i > 0 && name[i-1] == '/' {
					break
				}
				if doMatch(rest, name[i:]) {
					return true
				}
			}
			return false
		case '?':
			if len(name) == 0 || name[0] == '/' {
				return false
			}
			pattern = pattern[1:]
			name = name[1:]
		default:
			if len(name) == 0 || pattern[0] != name[0] {
				return false
			}
			pattern = pattern[1:]
			name = name[1:]
		}
	}
	return len(name) == 0
}
