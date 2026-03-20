package app

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jkuan/pr-review/internal/config"
	gitpkg "github.com/jkuan/pr-review/internal/git"
	"github.com/jkuan/pr-review/internal/review"
	"github.com/jkuan/pr-review/internal/ui/diffview"
	"github.com/jkuan/pr-review/internal/ui/prdetail"
	"github.com/jkuan/pr-review/internal/ui/prlist"
	"github.com/jkuan/pr-review/internal/ui/submit"
)

// Screen represents the current active screen.
type Screen int

const (
	ScreenPRList Screen = iota
	ScreenPRDetail
	ScreenDiffView
	ScreenSubmit
)

// Model is the top-level application model.
type Model struct {
	svc     review.Service
	cfg     config.Config
	filters []review.PRFilter
	columns []string
	screen  Screen
	width   int
	height  int

	// Screen models
	prList   prlist.Model
	prDetail prdetail.Model
	diffView diffview.Model
	submit   submit.Model

	// State
	selectedPR   *review.PullRequest
	review       *review.PendingReview
	initialPR    int // PR number passed via CLI argument (0 = none)
	userIdentity *review.UserIdentity
}

// New creates the application model.
func New(svc review.Service, cfg config.Config, filters []review.PRFilter, columns []string) Model {
	return Model{
		svc:     svc,
		cfg:     cfg,
		filters: filters,
		columns: columns,
		screen:  ScreenPRList,
		prList:  prlist.New(svc, filters, columns),
	}
}

// NewWithPR creates the application model starting at a specific PR.
func NewWithPR(svc review.Service, cfg config.Config, prNumber int, filters []review.PRFilter, columns []string) Model {
	pr := review.PullRequest{Number: prNumber}
	rev := review.NewPendingReview(prNumber, "", "")
	m := Model{
		svc:       svc,
		cfg:       cfg,
		filters:   filters,
		columns:   columns,
		screen:    ScreenDiffView,
		prList:    prlist.New(svc, filters, columns),
		review:    rev,
		initialPR: prNumber,
	}
	m.diffView = diffview.New(svc, pr, rev, m.diffviewOpts()...)
	return m
}

// diffviewOpts converts config and app state to diffview options.
func (m Model) diffviewOpts() []diffview.Option {
	var opts []diffview.Option
	if m.userIdentity != nil {
		opts = append(opts, diffview.WithUserIdentity(m.userIdentity))
	}
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

// loadUserIdentity fetches the current user's login and teams.
func (m Model) loadUserIdentity() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		login, err := m.svc.CurrentUser(ctx)
		if err != nil {
			return userIdentityMsg{err: err}
		}
		teams, err := m.svc.UserTeams(ctx)
		if err != nil {
			return userIdentityMsg{err: err}
		}
		return userIdentityMsg{
			identity: &review.UserIdentity{
				Login: login,
				Teams: teams,
			},
		}
	}
}

// Init starts the application.
func (m Model) Init() tea.Cmd {
	if m.initialPR > 0 {
		prNumber := m.initialPR
		return tea.Batch(
			m.diffView.Init(),
			tea.WindowSize(),
			m.loadUserIdentity(),
			func() tea.Msg {
				full, err := m.svc.GetPR(context.Background(), prNumber)
				return prBodyLoadedMsg{pr: full, err: err}
			},
		)
	}
	return tea.Batch(
		m.prList.Init(),
		m.loadUserIdentity(),
	)
}

// Update handles all messages, routing to the active screen.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Global messages
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case userIdentityMsg:
		if msg.err == nil && msg.identity != nil {
			m.userIdentity = msg.identity
			// Forward to diffview if it's active
			if m.screen == ScreenDiffView {
				var cmd tea.Cmd
				m.diffView, cmd = m.diffView.Update(diffview.UserIdentityMsg{Identity: msg.identity})
				return m, cmd
			}
		}
		return m, nil
	}

	switch m.screen {
	case ScreenPRList:
		return m.updatePRList(msg)
	case ScreenPRDetail:
		return m.updatePRDetail(msg)
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

// checkoutResultMsg carries the result of a PR checkout operation.
type checkoutResultMsg struct {
	err error
}

func (m Model) updatePRList(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case prlist.PRSelectedMsg:
		m.selectedPR = &msg.PR
		pr := msg.PR
		m.review = review.NewPendingReview(pr.Number, pr.NodeID, pr.HeadSHA)
		m.diffView = diffview.New(m.svc, pr, m.review, m.diffviewOpts()...)
		m.screen = ScreenDiffView
		return m, tea.Batch(
			m.diffView.Init(),
			tea.WindowSize(),
			func() tea.Msg {
				full, err := m.svc.GetPR(context.Background(), pr.Number)
				return prBodyLoadedMsg{pr: full, err: err}
			},
		)
	}

	var cmd tea.Cmd
	m.prList, cmd = m.prList.Update(msg)
	return m, cmd
}

func (m Model) updatePRDetail(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case prBodyLoadedMsg:
		if msg.err == nil && msg.pr != nil {
			m.selectedPR = msg.pr
			m.prDetail.SetPR(*msg.pr)
		} else if m.selectedPR != nil {
			// Body fetch failed; show what we have
			m.prDetail.SetPR(*m.selectedPR)
		}
		return m, nil

	case prdetail.StartReviewMsg:
		m.review = review.NewPendingReview(msg.PR.Number, msg.PR.NodeID, msg.PR.HeadSHA)
		m.diffView = diffview.New(m.svc, msg.PR, m.review, m.diffviewOpts()...)
		m.screen = ScreenDiffView
		return m, tea.Batch(
			m.diffView.Init(),
			tea.WindowSize(),
		)
	case prdetail.CheckoutMsg:
		prNumber := msg.PR.Number
		return m, func() tea.Msg {
			err := gitpkg.CheckoutPR(prNumber)
			return checkoutResultMsg{err: err}
		}
	case checkoutResultMsg:
		if msg.err != nil {
			m.prDetail.SetCheckoutErr(msg.err)
		} else {
			m.prDetail.SetCheckoutSuccess()
		}
		return m, nil
	case prdetail.BackMsg:
		m.screen = ScreenPRList
		return m, tea.WindowSize()
	}

	var cmd tea.Cmd
	m.prDetail, cmd = m.prDetail.Update(msg)
	return m, cmd
}

func (m Model) updateDiffView(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case diffview.SubmitReviewMsg:
		m.submit = submit.New(m.svc, m.review)
		m.screen = ScreenSubmit
		return m, tea.Batch(
			m.submit.Init(),
			tea.WindowSize(),
		)
	case diffview.BackMsg:
		m.review = nil
		m.selectedPR = nil
		m.screen = ScreenPRList
		return m, tea.WindowSize()
	case prBodyLoadedMsg:
		if msg.err == nil && msg.pr != nil {
			m.selectedPR = msg.pr
			// Backfill review fields that may have been empty at creation
			// (e.g., when launched via CLI with just a PR number).
			if m.review != nil {
				if m.review.PRNodeID == "" {
					m.review.PRNodeID = msg.pr.NodeID
				}
				if m.review.CommitID == "" {
					m.review.CommitID = msg.pr.HeadSHA
				}
			}
		}
		// Forward to diffview as PRBodyLoadedMsg
		pr := m.selectedPR
		var dvCmd tea.Cmd
		m.diffView, dvCmd = m.diffView.Update(diffview.PRBodyLoadedMsg{PR: pr, Err: msg.err})
		return m, dvCmd
	}

	var cmd tea.Cmd
	m.diffView, cmd = m.diffView.Update(msg)
	return m, cmd
}

func (m Model) updateSubmit(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case submit.SubmittedMsg:
		m.review = nil
		m.selectedPR = nil
		m.screen = ScreenPRList
		m.prList = prlist.New(m.svc, m.filters, m.columns)
		return m, tea.Batch(
			m.prList.Init(),
			tea.WindowSize(),
		)
	case submit.CancelledMsg:
		m.screen = ScreenDiffView
		return m, tea.WindowSize()
	}

	var cmd tea.Cmd
	m.submit, cmd = m.submit.Update(msg)
	return m, cmd
}

// View renders the active screen.
func (m Model) View() string {
	switch m.screen {
	case ScreenPRList:
		return m.prList.View()
	case ScreenPRDetail:
		return m.prDetail.View()
	case ScreenDiffView:
		return m.diffView.View()
	case ScreenSubmit:
		return m.submit.View()
	}
	return ""
}
