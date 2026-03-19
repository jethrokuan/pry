package git

import (
	"fmt"
	"os/exec"
	"strings"
)

// CheckoutPR checks out a PR locally using gh pr checkout.
func CheckoutPR(number int) error {
	cmd := exec.Command("gh", "pr", "checkout", fmt.Sprintf("%d", number))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to checkout PR #%d: %s", number, string(out))
	}
	return nil
}

// GetRepoInfo returns the owner and repo name from the current git directory.
func GetRepoInfo() (owner, repo string, err error) {
	cmd := exec.Command("gh", "repo", "view", "--json", "owner,name", "-q", ".owner.login + \"/\" + .name")
	out, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("failed to get repo info (are you in a git repo with a GitHub remote?): %w", err)
	}
	parts := strings.SplitN(strings.TrimSpace(string(out)), "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("unexpected repo info format: %s", string(out))
	}
	return parts[0], parts[1], nil
}
