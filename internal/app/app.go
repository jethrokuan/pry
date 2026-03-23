package app

import (
	"context"
	"fmt"
	"log/slog"

	tea "charm.land/bubbletea/v2"

	"github.com/jethrokuan/pry/internal/appctx"
	"github.com/jethrokuan/pry/internal/config"
	"github.com/jethrokuan/pry/internal/review"
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
	ctx     *appctx.Context
	cfg     config.Config
	filters []review.PRFilter
	screen  Screen
	width   int
	height  int

	// Screen models
	prList   prlist.Model
	diffView diffview.Model
	submit   submit.Model

	// State
	selectedPR *review.PullRequest
	initialPR  int // PR number passed via CLI argument (0 = none)
}

// New creates the application model.
func New(svc review.Service, cfg config.Config, filters []review.PRFilter) Model {
	ctx := &appctx.Context{Svc: svc}
	return Model{
		ctx:     ctx,
		cfg:     cfg,
		filters: filters,
		screen:  ScreenPRList,
		prList:  prlist.New(ctx, filters),
	}
}

// NewWithPR creates the application model starting at a specific PR.
func NewWithPR(svc review.Service, cfg config.Config, prNumber int, filters []review.PRFilter) Model {
	ctx := &appctx.Context{Svc: svc}
	pr := &review.PullRequest{Number: prNumber}
	pr.StartReview()
	m := Model{
		ctx:        ctx,
		cfg:        cfg,
		filters:    filters,
		screen:     ScreenDiffView,
		prList:     prlist.New(ctx, filters),
		selectedPR: pr,
		initialPR:  prNumber,
	}
	m.diffView = diffview.New(ctx, pr, m.diffviewOpts()...)
	return m
}

// diffviewOpts converts config to diffview options.
func (m Model) diffviewOpts() []diffview.Option {
	var opts []diffview.Option
	if m.cfg.FileTree.OwnerFilter != nil && !*m.cfg.FileTree.OwnerFilter {
		opts = append(opts, diffview.WithOwnerFilterDisabled())
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
	users []string
	err   error
}

// loadUserIdentity fetches the current user's login and teams.
func (m Model) loadUserIdentity() tea.Cmd {
	return safeCmd(func() tea.Msg {
		ctx := context.Background()
		login, err := m.ctx.Svc.CurrentUser(ctx)
		if err != nil {
			return userIdentityMsg{err: err}
		}
		teams, err := m.ctx.Svc.UserTeams(ctx)
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
		users, err := m.ctx.Svc.ListMentionableUsers(context.Background())
		return mentionableUsersMsg{users: users, err: err}
	})
}

// Init starts the application.
func (m Model) Init() tea.Cmd {
	if m.initialPR > 0 {
		prNumber := m.initialPR
		return tea.Batch(
			m.diffView.Init(),
			m.loadUserIdentity(),
			m.loadMentionableUsers(),
			safeCmd(func() tea.Msg {
				full, err := m.ctx.Svc.GetPR(context.Background(), prNumber)
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

	// Global messages
	switch msg := msg.(type) {
	case CmdPanicMsg:
		slog.Error("command panic recovered", "error", msg.Err)
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case userIdentityMsg:
		if msg.err == nil && msg.identity != nil {
			m.ctx.UserIdentity = msg.identity
			// Forward to diffview if it's active
			if m.screen == ScreenDiffView {
				var cmd tea.Cmd
				m.diffView, cmd = m.diffView.Update(diffview.UserIdentityMsg{Identity: msg.identity})
				return m, cmd
			}
		}
		return m, nil
	case mentionableUsersMsg:
		if msg.err == nil {
			m.ctx.MentionableUsers = msg.users
			// Forward to diffview if it's active
			if m.screen == ScreenDiffView {
				var cmd tea.Cmd
				m.diffView, cmd = m.diffView.Update(diffview.MentionableUsersMsg{Users: msg.users})
				return m, cmd
			}
		}
		return m, nil
	}

	switch m.screen {
	case ScreenPRList:
		return m.updatePRList(msg)
	case ScreenDiffView:
		return m.updateDiffView(msg)
	case ScreenSubmit:
		return m.updateSubmit(msg)
	}

	return m, nil
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
		m.diffView = diffview.New(m.ctx, pr, m.diffviewOpts()...)
		m.screen = ScreenDiffView
		prNumber := pr.Number
		return m, tea.Batch(
			m.diffView.Init(),
			m.windowSizeCmd(),
			safeCmd(func() tea.Msg {
				full, err := m.ctx.Svc.GetPR(context.Background(), prNumber)
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
		m.submit = submit.New(m.ctx, m.selectedPR)
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
			existingComments := m.selectedPR.ExistingComments
			*m.selectedPR = *msg.pr
			m.selectedPR.PendingReview = pendingReview
			m.selectedPR.ExistingComments = existingComments
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
		m.prList = prlist.New(m.ctx, m.filters)
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
	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}
