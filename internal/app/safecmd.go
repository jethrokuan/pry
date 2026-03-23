package app

import (
	"fmt"
	"log/slog"
	"runtime/debug"

	tea "charm.land/bubbletea/v2"
)

// CmdPanicMsg is sent when a tea.Cmd panics instead of crashing the program.
type CmdPanicMsg struct {
	Err error
}

// safeCmd wraps a message-producing function with panic recovery.
// If the function panics, the panic is logged and a CmdPanicMsg is returned
// instead of corrupting the terminal.
func safeCmd(fn func() tea.Msg) tea.Cmd {
	return func() (msg tea.Msg) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic in command", "error", r, "stack", string(debug.Stack()))
				msg = CmdPanicMsg{Err: fmt.Errorf("internal error: %v", r)}
			}
		}()
		return fn()
	}
}
