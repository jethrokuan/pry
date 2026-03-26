package jj

import (
	"fmt"
	"os/exec"
	"strings"
)

// IsRepo returns true if the current working directory is inside a
// Jujutsu-managed repository (detected via `jj root`).
func IsRepo() bool {
	cmd := exec.Command("jj", "root")
	return cmd.Run() == nil
}

// Checkout fetches the latest remote state and creates a new jj working copy
// change on top of the given branch from origin.
func Checkout(branch string) error {
	fetch := exec.Command("jj", "git", "fetch")
	if out, err := fetch.CombinedOutput(); err != nil {
		return fmt.Errorf("jj git fetch: %s", strings.TrimSpace(string(out)))
	}

	newCmd := exec.Command("jj", "new", branch+"@origin")
	if out, err := newCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("jj new %s@origin: %s", branch, strings.TrimSpace(string(out)))
	}
	return nil
}
