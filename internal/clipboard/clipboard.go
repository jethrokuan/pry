// Package clipboard provides clipboard read/write operations
// using golang.design/x/clipboard.
package clipboard

import (
	"golang.design/x/clipboard"
)

func init() {
	if err := clipboard.Init(); err != nil {
		// Clipboard unavailable (e.g. headless server).
		// Read/Write will fail gracefully at call time.
	}
}

// WriteText writes text to the system clipboard.
func WriteText(text string) error {
	clipboard.Write(clipboard.FmtText, []byte(text))
	return nil
}
