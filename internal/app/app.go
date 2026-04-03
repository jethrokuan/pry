package app

import (
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jethrokuan/pry/internal/ai"
	"github.com/jethrokuan/pry/internal/config"
	"github.com/jethrokuan/pry/internal/data"
	"github.com/jethrokuan/pry/internal/jj"
	"github.com/jethrokuan/pry/internal/review"
	"github.com/jethrokuan/pry/internal/ui/components/flash"
	"github.com/jethrokuan/pry/internal/ui/diffview"
	"github.com/jethrokuan/pry/internal/ui/prlist"
	"github.com/jethrokuan/pry/internal/ui/styles"
	"github.com/jethrokuan/pry/internal/ui/submit"
)

// Screen represents the current active screen.
type Screen int

const (
	ScreenPRList Screen = iota
	ScreenDiffView
	ScreenSubmit
)

// Model is the top-level application model.
type Model struct {
	cfg              config.Config
	filters          []review.PRFilter
	screen           Screen
	width            int
	height           int
	userIdentity     *review.UserIdentity
	mentionableUsers []review.MentionableUser

	// Screen models
	prList   prlist.Model
	diffView diffview.Model
	submit   submit.Model

	// Flash messages (stacked, newer on top)
	flash flash.Model

	// State
	selectedPR *review.PullRequest
	initialPR  int  // PR number passed via CLI argument (0 = none)
	useJJ      bool // true when repo is Jujutsu-managed
	aiAgent    *ai.Agent // nil when AI is disabled or claude not available
}

// New creates the application model.
func New(cfg config.Config, filters []review.PRFilter) Model {
	useJJ := jj.IsRepo()
	pl := prlist.New(filters)
	pl.SetJujutsu(useJJ)
	return Model{
		cfg:     cfg,
		filters: filters,
		screen:  ScreenPRList,
		prList:  pl,
		flash:   flash.New(),
		useJJ:   useJJ,
		aiAgent: createAIAgent(cfg),
	}
}

// NewWithPR creates the application model starting at a specific PR.
func NewWithPR(cfg config.Config, prNumber int, filters []review.PRFilter) Model {
	pr := &review.PullRequest{Number: prNumber}
	pr.StartReview()
	useJJ := jj.IsRepo()
	pl := prlist.New(filters)
	pl.SetJujutsu(useJJ)
	m := Model{
		cfg:        cfg,
		useJJ:      useJJ,
		filters:    filters,
		screen:     ScreenDiffView,
		prList:     pl,
		flash:      flash.New(),
		selectedPR: pr,
		initialPR:  prNumber,
		aiAgent:    createAIAgent(cfg),
	}
	m.diffView = diffview.New(pr, m.diffviewOpts()...)
	return m
}

// createAIAgent creates the AI agent if enabled and claude is available.
func createAIAgent(cfg config.Config) *ai.Agent {
	if cfg.AI.Enabled != nil && !*cfg.AI.Enabled {
		return nil
	}
	if !ai.Available() {
		slog.Info("AI assistant disabled: claude CLI not found on PATH")
		return nil
	}
	repoRoot, err := gitRepoRoot()
	if err != nil {
		slog.Warn("AI assistant disabled: could not determine repo root", "error", err)
		return nil
	}
	slog.Info("AI assistant enabled", "model", cfg.AI.Model, "max_turns", cfg.AI.MaxTurns)
	return ai.NewAgent(repoRoot, cfg.AI.Model, cfg.AI.MaxTurns)
}

// gitRepoRoot returns the root directory of the current git repository.
func gitRepoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}


// diffviewOpts converts config to diffview options.
func (m Model) diffviewOpts() []diffview.Option {
	var opts []diffview.Option
	if m.userIdentity != nil {
		opts = append(opts, diffview.WithUserIdentity(m.userIdentity))
		opts = append(opts, diffview.WithCurrentUser(m.userIdentity.Login))
	}
	if len(m.mentionableUsers) > 0 {
		opts = append(opts, diffview.WithMentionableUsers(m.mentionableUsers))
	}
	if m.cfg.FileTree.OwnerFilter != nil && *m.cfg.FileTree.OwnerFilter {
		opts = append(opts, diffview.WithOwnerFilterEnabled())
	}
	if m.useJJ {
		opts = append(opts, diffview.WithJujutsu())
	}
	if m.aiAgent != nil {
		opts = append(opts, diffview.WithAI(m.aiAgent))
	}
	return opts
}

// userIdentityMsg carries the result of the async user identity fetch.
type userIdentityMsg struct {
	identity *review.UserIdentity
	err      error
}

// mentionableUsersMsg carries the result of the async mentionable users fetch.
type mentionableUsersMsg struct {
	users []review.MentionableUser
	err   error
}

// loadUserIdentity fetches the current user's login and teams.
func (m Model) loadUserIdentity() tea.Cmd {
	return safeCmd(func() tea.Msg {
		login, err := data.CurrentUser()
		if err != nil {
			return userIdentityMsg{err: err}
		}
		teams, err := data.UserTeams()
		if err != nil {
			return userIdentityMsg{err: err}
		}
		return userIdentityMsg{
			identity: &review.UserIdentity{
				Login: login,
				Teams: teams,
			},
		}
	})
}

// loadMentionableUsers fetches @-mentionable usernames in the background at startup.
func (m Model) loadMentionableUsers() tea.Cmd {
	return safeCmd(func() tea.Msg {
		users, err := data.ListMentionableUsers()
		return mentionableUsersMsg{users: users, err: err}
	})
}

// Init starts the application.
func (m Model) Init() tea.Cmd {
	if m.initialPR > 0 {
		prNumber := m.initialPR
		return tea.Batch(
			tea.RequestBackgroundColor,
			m.diffView.Init(),
			m.loadUserIdentity(),
			m.loadMentionableUsers(),
			safeCmd(func() tea.Msg {
				full, err := data.FetchPR(prNumber)
				return prBodyLoadedMsg{pr: full, err: err}
			}),
		)
	}
	return tea.Batch(
		tea.RequestBackgroundColor,
		m.prList.Init(),
		m.loadUserIdentity(),
		m.loadMentionableUsers(),
	)
}

// windowSizeMsg returns a command that re-sends the current window dimensions.
// This ensures sub-models get sized correctly on screen transitions.
func (m Model) windowSizeCmd() tea.Cmd {
	w, h := m.width, m.height
	if w == 0 {
		return nil
	}
	return func() tea.Msg {
		return tea.WindowSizeMsg{Width: w, Height: h}
	}
}

// Update handles all messages, routing to the active screen.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Log all messages for debugging (visible with -v flag in ~/.config/pry/debug.log)
	if k, ok := msg.(tea.KeyPressMsg); ok {
		slog.Debug("msg", "type", "KeyPressMsg", "key", k.String())
	} else {
		slog.Debug("msg", "type", fmt.Sprintf("%T", msg))
	}

	// Flash messages from any screen — forward to flash component.
	var flashCmd tea.Cmd
	m.flash, flashCmd = m.flash.Update(msg)

	// Global messages
	switch msg := msg.(type) {
	case CmdPanicMsg:
		slog.Error("command panic recovered", "error", msg.Err)
		return m, flashCmd
	case tea.BackgroundColorMsg:
		styles.Apply(msg.IsDark(), msg)
		return m, flashCmd
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case userIdentityMsg:
		if msg.err == nil && msg.identity != nil {
			m.userIdentity = msg.identity
			switch m.screen {
			case ScreenPRList:
				var cmd tea.Cmd
				m.prList, cmd = m.prList.Update(prlist.UserIdentityMsg{Identity: msg.identity})
				return m, tea.Batch(flashCmd, cmd)
			case ScreenDiffView:
				var cmd tea.Cmd
				m.diffView, cmd = m.diffView.Update(diffview.UserIdentityMsg{Identity: msg.identity})
				return m, tea.Batch(flashCmd, cmd)
			}
		}
		return m, flashCmd
	case mentionableUsersMsg:
		if msg.err == nil {
			m.mentionableUsers = msg.users
			// Forward to prlist (always, for author: autocomplete)
			var prlistCmd tea.Cmd
			m.prList, prlistCmd = m.prList.Update(prlist.MentionableUsersMsg{Users: msg.users})
			cmds := []tea.Cmd{flashCmd, prlistCmd}
			// Forward to diffview if it's active
			if m.screen == ScreenDiffView {
				var cmd tea.Cmd
				m.diffView, cmd = m.diffView.Update(diffview.MentionableUsersMsg{Users: msg.users})
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)
		}
		return m, flashCmd
	}

	var model tea.Model
	var screenCmd tea.Cmd
	switch m.screen {
	case ScreenPRList:
		model, screenCmd = m.updatePRList(msg)
	case ScreenDiffView:
		model, screenCmd = m.updateDiffView(msg)
	case ScreenSubmit:
		model, screenCmd = m.updateSubmit(msg)
	default:
		return m, flashCmd
	}
	return model, tea.Batch(flashCmd, screenCmd)
}

// prBodyLoadedMsg carries the full PR data after fetching the body.
type prBodyLoadedMsg struct {
	pr  *review.PullRequest
	err error
}

func (m Model) updatePRList(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case prlist.PRSelectedMsg:
		pr := msg.PR
		m.selectedPR = pr
		pr.StartReview()
		m.diffView = diffview.New(pr, m.diffviewOpts()...)
		m.screen = ScreenDiffView
		prNumber := pr.Number
		return m, tea.Batch(
			m.diffView.Init(),
			m.windowSizeCmd(),
			safeCmd(func() tea.Msg {
				full, err := data.FetchPR(prNumber)
				return prBodyLoadedMsg{pr: full, err: err}
			}),
		)
	case prlist.GoToPRMsg:
		pr := &review.PullRequest{Number: msg.Number}
		pr.StartReview()
		m.selectedPR = pr
		m.diffView = diffview.New(pr, m.diffviewOpts()...)
		m.screen = ScreenDiffView
		prNumber := msg.Number
		return m, tea.Batch(
			m.diffView.Init(),
			m.windowSizeCmd(),
			safeCmd(func() tea.Msg {
				full, err := data.FetchPR(prNumber)
				return prBodyLoadedMsg{pr: full, err: err}
			}),
		)
	}

	var cmd tea.Cmd
	m.prList, cmd = m.prList.Update(msg)
	return m, cmd
}

func (m Model) updateDiffView(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case diffview.SubmitReviewMsg:
		m.selectedPR.PendingReview = m.diffView.PendingReview()
		currentUser := ""
		if m.userIdentity != nil {
			currentUser = m.userIdentity.Login
		}
		m.submit = submit.New(m.selectedPR, currentUser)
		m.screen = ScreenSubmit
		return m, tea.Batch(m.submit.Init(), m.windowSizeCmd())
	case diffview.BackMsg:
		m.selectedPR = nil
		m.screen = ScreenPRList
		return m, m.windowSizeCmd()
	case prBodyLoadedMsg:
		// Forward to diffview — it owns the merge via MergeEnriched.
		// app.go sees the result through the shared pointer.
		var dvCmd tea.Cmd
		m.diffView, dvCmd = m.diffView.Update(diffview.PRBodyLoadedMsg{PR: msg.pr, Err: msg.err})
		return m, dvCmd
	}

	var cmd tea.Cmd
	m.diffView, cmd = m.diffView.Update(msg)
	return m, cmd
}

func (m Model) updateSubmit(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case submit.SubmittedMsg:
		m.selectedPR = nil
		m.screen = ScreenPRList
		m.prList = prlist.New(m.filters)
		return m, tea.Batch(m.prList.Init(), m.windowSizeCmd())
	case submit.CancelledMsg:
		m.screen = ScreenDiffView
		return m, m.windowSizeCmd()
	}

	var cmd tea.Cmd
	m.submit, cmd = m.submit.Update(msg)
	return m, cmd
}

// View renders the active screen.
func (m Model) View() tea.View {
	var content string
	switch m.screen {
	case ScreenPRList:
		content = m.prList.View()
	case ScreenDiffView:
		content = m.diffView.View()
	case ScreenSubmit:
		content = m.submit.View()
	}

	// Overlay flash messages using Canvas compositing.
	if !m.flash.Empty() {
		flashView := m.flash.View()
		flashW := lipgloss.Width(flashView)

		// Top-right corner with 1 cell padding from edge.
		x := m.width - flashW - 1
		if x < 0 {
			x = 0
		}
		y := 1
		if y < 0 {
			y = 0
		}

		base := lipgloss.NewLayer(content)
		overlay := lipgloss.NewLayer(flashView).X(x).Y(y).Z(1)
		content = lipgloss.NewCompositor(base, overlay).Render()
	}

	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}
