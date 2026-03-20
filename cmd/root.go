package main

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jkuan/pr-review/internal/app"
	"github.com/jkuan/pr-review/internal/config"
	gitpkg "github.com/jkuan/pr-review/internal/git"
	gh "github.com/jkuan/pr-review/internal/github"
	"github.com/jkuan/pr-review/internal/logging"
	"github.com/jkuan/pr-review/internal/ui/styles"
)

// CLI defines the command-line interface for pr-review.
type CLI struct {
	PRNumber int    `arg:"" optional:"" help:"PR number to open directly."`
	Config   string `short:"c" help:"Path to config file." type:"path"`
	Verbose  bool   `short:"v" help:"Enable verbose output."`
}

func main() {
	var cli CLI
	kong.Parse(&cli,
		kong.Name("pr-review"),
		kong.Description("Terminal UI for reviewing GitHub pull requests."),
		kong.UsageOnError(),
	)

	// Set up file-based logging (writes to ~/.config/pr-review/debug.log)
	logCleanup, err := logging.Setup(cli.Verbose)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not set up logging: %v\n", err)
	} else {
		defer logCleanup()
	}

	// Load config and apply theme
	var cfg config.Config
	if cli.Config != "" {
		cfg = config.LoadFrom(cli.Config)
	} else {
		cfg = config.Load()
	}
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
	if cli.PRNumber > 0 {
		model = app.NewWithPR(svc, cfg, cli.PRNumber, filters, columns)
	} else {
		model = app.New(svc, cfg, filters, columns)
	}

	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())

	// Force-quit handler: double Ctrl-C exits even if the TUI event loop is hung.
	// This runs in a separate goroutine independent of Bubble Tea.
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT)

		var mu sync.Mutex
		var lastInterrupt time.Time
		const window = 1 * time.Second

		for range sigCh {
			mu.Lock()
			now := time.Now()
			if now.Sub(lastInterrupt) <= window {
				mu.Unlock()
				// Second Ctrl-C within window: force exit
				os.Exit(130) // 128 + SIGINT(2) — standard exit code
			}
			lastInterrupt = now
			mu.Unlock()

			// First Ctrl-C: forward to Bubble Tea as a key event so the
			// normal quit flow (with unsaved-work confirmation) still works.
			p.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
		}
	}()

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
