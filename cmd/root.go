package main

import (
	"fmt"
	"os"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jkuan/pr-review/internal/app"
	"github.com/jkuan/pr-review/internal/config"
	gitpkg "github.com/jkuan/pr-review/internal/git"
	gh "github.com/jkuan/pr-review/internal/github"
	"github.com/jkuan/pr-review/internal/ui/styles"
)

func main() {
	// Load config and apply theme
	cfg := config.Load()
	styles.Apply(config.ResolveTheme(cfg))

	// Detect repo context
	owner, repo, err := gitpkg.GetRepoInfo()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "Make sure you're in a git repo with a GitHub remote and `gh` is authenticated.\n")
		os.Exit(1)
	}

	// Create GitHub client (implements review.Service directly)
	svc, err := gh.NewClient(owner, repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Load filters and columns from config (falls back to defaults)
	filters := cfg.PRFilters()
	columns := cfg.PRColumns()

	// Create the app — optionally jump to a specific PR
	var model app.Model
	if len(os.Args) > 1 {
		prNumber, err := strconv.Atoi(os.Args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Usage: pr-review [PR_NUMBER]\n")
			os.Exit(1)
		}
		model = app.NewWithPR(svc, prNumber, filters, columns)
	} else {
		model = app.New(svc, filters, columns)
	}

	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
