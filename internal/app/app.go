package app

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

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
	svc    review.Service
	screen Screen
	width  int
	height int

	// Screen models
	prList   prlist.Model
	prDetail prdetail.Model
	diffView diffview.Model
	submit   submit.Model

	// State
	selectedPR *review.PullRequest
	review     *review.PendingReview
	initialPR  int // PR number passed via CLI argument (0 = none)
}

// New creates the application model.
func New(svc review.Service) Model {
	return Model{
		svc:    svc,
		screen: ScreenPRList,
		prList: prlist.New(svc),
	}
}

// NewWithPR creates the application model starting at a specific PR.
func NewWithPR(svc review.Service, prNumber int) Model {
	return Model{
		svc:        svc,
		screen:     ScreenPRDetail,
		prList:     prlist.New(svc),
		prDetail:   prdetail.New(review.PullRequest{Number: prNumber}),
		initialPR:  prNumber,
	}
}

// Init starts the application.
func (m Model) Init() tea.Cmd {
	if m.initialPR > 0 {
		prNumber := m.initialPR
		return tea.Batch(
			tea.WindowSize(),
			func() tea.Msg {
				full, err := m.svc.GetPR(context.Background(), prNumber)
				return prBodyLoadedMsg{pr: full, err: err}
			},
		)
	}
	return m.prList.Init()
}

// Update handles all messages, routing to the active screen.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Global messages
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
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
		m.screen = ScreenPRDetail
		m.prDetail = prdetail.New(msg.PR)
		pr := msg.PR
		return m, tea.Batch(
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
		m.diffView = diffview.New(m.svc, msg.PR, m.review)
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
	switch msg.(type) {
	case diffview.SubmitReviewMsg:
		m.submit = submit.New(m.svc, m.review)
		m.screen = ScreenSubmit
		return m, tea.Batch(
			m.submit.Init(),
			tea.WindowSize(),
		)
	case diffview.BackMsg:
		m.screen = ScreenPRDetail
		return m, tea.WindowSize()
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
		m.prList = prlist.New(m.svc)
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
