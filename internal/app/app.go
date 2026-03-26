package app

import (
	"context"
	"fmt"
	"log/slog"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jethrokuan/pry/internal/config"
	"github.com/jethrokuan/pry/internal/jj"
	"github.com/jethrokuan/pry/internal/review"
	"github.com/jethrokuan/pry/internal/ui/components/flash"
	"github.com/jethrokuan/pry/internal/ui/diffview"
	"github.com/jethrokuan/pry/internal/ui/prlist"
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
	svc              review.Service
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
}

// New creates the application model.
func New(svc review.Service, cfg config.Config, filters []review.PRFilter) Model {
	useJJ := jj.IsRepo()
	pl := prlist.New(svc, filters)
	pl.SetJujutsu(useJJ)
	return Model{
		svc:     svc,
		cfg:     cfg,
		filters: filters,
		screen:  ScreenPRList,
		prList:  pl,
		flash:   flash.New(),
		useJJ:   useJJ,
	}
}

// NewWithPR creates the application model starting at a specific PR.
func NewWithPR(svc review.Service, cfg config.Config, prNumber int, filters []review.PRFilter) Model {
	pr := &review.PullRequest{Number: prNumber}
	pr.StartReview()
	useJJ := jj.IsRepo()
	pl := prlist.New(svc, filters)
	pl.SetJujutsu(useJJ)
	m := Model{
		svc:        svc,
		cfg:        cfg,
		useJJ:      useJJ,
		filters:    filters,
		screen:     ScreenDiffView,
		prList:     pl,
		flash:      flash.New(),
		selectedPR: pr,
		initialPR:  prNumber,
	}
	m.diffView = diffview.New(svc, pr, m.diffviewOpts()...)
	return m
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
	if m.cfg.FileTree.OwnerFilter != nil && !*m.cfg.FileTree.OwnerFilter {
		opts = append(opts, diffview.WithOwnerFilterDisabled())
	}
	if m.useJJ {
		opts = append(opts, diffview.WithJujutsu())
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
	svc := m.svc
	return safeCmd(func() tea.Msg {
		ctx := context.Background()
		login, err := svc.CurrentUser(ctx)
		if err != nil {
			return userIdentityMsg{err: err}
		}
		teams, err := svc.UserTeams(ctx)
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
	svc := m.svc
	return safeCmd(func() tea.Msg {
		users, err := svc.ListMentionableUsers(context.Background())
		return mentionableUsersMsg{users: users, err: err}
	})
}

// Init starts the application.
func (m Model) Init() tea.Cmd {
	if m.initialPR > 0 {
		prNumber := m.initialPR
		svc := m.svc
		return tea.Batch(
			m.diffView.Init(),
			m.loadUserIdentity(),
			m.loadMentionableUsers(),
			safeCmd(func() tea.Msg {
				full, err := svc.GetPR(context.Background(), prNumber)
				return prBodyLoadedMsg{pr: full, err: err}
			}),
		)
	}
	return tea.Batch(
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
			// Forward to diffview if it's active
			if m.screen == ScreenDiffView {
				var cmd tea.Cmd
				m.diffView, cmd = m.diffView.Update(diffview.MentionableUsersMsg{Users: msg.users})
				return m, tea.Batch(flashCmd, cmd)
			}
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
		m.diffView = diffview.New(m.svc, pr, m.diffviewOpts()...)
		m.screen = ScreenDiffView
		prNumber := pr.Number
		svc := m.svc
		return m, tea.Batch(
			m.diffView.Init(),
			m.windowSizeCmd(),
			safeCmd(func() tea.Msg {
				full, err := svc.GetPR(context.Background(), prNumber)
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
		m.submit = submit.New(m.svc, m.selectedPR, currentUser)
		m.screen = ScreenSubmit
		return m, tea.Batch(m.submit.Init(), m.windowSizeCmd())
	case diffview.BackMsg:
		m.selectedPR = nil
		m.screen = ScreenPRList
		return m, m.windowSizeCmd()
	case prBodyLoadedMsg:
		if msg.err == nil && msg.pr != nil {
			// Update the shared PR in-place so diffview sees the change.
			// Preserve review state that the new PR data doesn't carry.
			pendingReview := m.selectedPR.PendingReview
			threads := m.selectedPR.Threads
			*m.selectedPR = *msg.pr
			m.selectedPR.PendingReview = pendingReview
			m.selectedPR.Threads = threads
		}
		// Forward to diffview as PRBodyLoadedMsg
		var dvCmd tea.Cmd
		m.diffView, dvCmd = m.diffView.Update(diffview.PRBodyLoadedMsg{PR: m.selectedPR, Err: msg.err})
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
		m.prList = prlist.New(m.svc, m.filters)
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
