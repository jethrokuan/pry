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

// ReadImage reads image data (PNG) from the system clipboard.
// Returns nil, nil if the clipboard does not contain image data.
func ReadImage() ([]byte, error) {
	data := clipboard.Read(clipboard.FmtImage)
	if len(data) == 0 {
		return nil, nil
	}
	return data, nil
}

// WriteText writes text to the system clipboard.
func WriteText(text string) error {
	clipboard.Write(clipboard.FmtText, []byte(text))
	return nil
}
