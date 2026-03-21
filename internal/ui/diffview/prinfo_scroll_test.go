package diffview

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/jkuan/pr-review/internal/appctx"
	"github.com/jkuan/pr-review/internal/review"
)

var _ = ginkgo.Describe("PR Info Popup Scrolling", func() {
	newTestModel := func() Model {
		lines := make([]string, 100)
		for i := range lines {
			lines[i] = "Description line with some content"
		}
		longBody := strings.Join(lines, "\n")

		pr := &review.PullRequest{
			Number: 1,
			Title:  "Test PR",
			Author: "test",
			Base:   "main",
			Body:   longBody,
		}

		pr.StartReview()
		m := New(&appctx.Context{}, pr)
		m.loading = false

		// Set window size
		m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
		return m
	}

	ginkgo.It("should scroll viewport down with j key", func() {
		m := newTestModel()

		// Open PR info popup via 'i'
		m, _ = m.Update(tea.KeyPressMsg{Code: 'i'})
		gomega.Expect(m.prInfoActive).To(gomega.BeTrue())
		
		gomega.Expect(m.prInfoViewport.TotalLineCount()).To(
			gomega.BeNumerically(">", m.prInfoViewport.Height()), "Content should overflow viewport")

		// Capture initial rendered popup
		initialPopup := m.renderPRInfoPopup()
		initialOffset := m.prInfoViewport.YOffset()
		gomega.Expect(initialOffset).To(gomega.Equal(0))

		// Press j to scroll down
		m, _ = m.Update(tea.KeyPressMsg{Code: 'j'})
		gomega.Expect(m.prInfoViewport.YOffset()).To(gomega.Equal(1),
			"YOffset should be 1 after pressing j")

		// Verify popup render changed
		afterPopup := m.renderPRInfoPopup()
		gomega.Expect(afterPopup).NotTo(gomega.Equal(initialPopup),
			"Rendered popup should change after scrolling")
	})

	ginkgo.It("should scroll with arrow keys", func() {
		m := newTestModel()
		m, _ = m.Update(tea.KeyPressMsg{Code: 'i'})

		// Press down arrow
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		gomega.Expect(m.prInfoViewport.YOffset()).To(gomega.Equal(1))

		// Press up arrow
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
		gomega.Expect(m.prInfoViewport.YOffset()).To(gomega.Equal(0))
	})

	ginkgo.It("should handle multiple scroll actions", func() {
		m := newTestModel()
		m, _ = m.Update(tea.KeyPressMsg{Code: 'i'})

		for i := 0; i < 10; i++ {
			m, _ = m.Update(tea.KeyPressMsg{Code: 'j'})
		}
		gomega.Expect(m.prInfoViewport.YOffset()).To(gomega.Equal(10))
	})

	ginkgo.It("should not scroll beyond content", func() {
		m := newTestModel()
		m, _ = m.Update(tea.KeyPressMsg{Code: 'i'})

		maxOffset := m.prInfoViewport.TotalLineCount() - m.prInfoViewport.Height()
		
		// Scroll way past the end
		for i := 0; i < 200; i++ {
			m, _ = m.Update(tea.KeyPressMsg{Code: 'j'})
		}
		gomega.Expect(m.prInfoViewport.YOffset()).To(gomega.BeNumerically("<=", maxOffset))
	})

	ginkgo.It("View should change after scrolling", func() {
		m := newTestModel()
		m, _ = m.Update(tea.KeyPressMsg{Code: 'i'})

		view1 := m.View()

		for i := 0; i < 5; i++ {
			m, _ = m.Update(tea.KeyPressMsg{Code: 'j'})
		}

		view2 := m.View()
		gomega.Expect(view2).NotTo(gomega.Equal(view1),
			"Full View() should change after scrolling the popup")
	})

	ginkgo.It("should scroll down with mouse wheel", func() {
		m := newTestModel()
		m, _ = m.Update(tea.KeyPressMsg{Code: 'i'})
		gomega.Expect(m.prInfoActive).To(gomega.BeTrue())

		initialOffset := m.prInfoViewport.YOffset()

		// Simulate mouse wheel down
		m, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
		gomega.Expect(m.prInfoViewport.YOffset()).To(gomega.BeNumerically(">", initialOffset),
			"YOffset should increase after mouse wheel down")
	})

	ginkgo.It("should scroll up with mouse wheel", func() {
		m := newTestModel()
		m, _ = m.Update(tea.KeyPressMsg{Code: 'i'})

		// Scroll down first with keyboard
		for i := 0; i < 10; i++ {
			m, _ = m.Update(tea.KeyPressMsg{Code: 'j'})
		}
		offsetAfterDown := m.prInfoViewport.YOffset()

		// Simulate mouse wheel up
		m, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
		gomega.Expect(m.prInfoViewport.YOffset()).To(gomega.BeNumerically("<", offsetAfterDown),
			"YOffset should decrease after mouse wheel up")
	})
})
