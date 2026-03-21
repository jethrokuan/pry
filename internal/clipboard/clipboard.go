// Package clipboard provides platform-specific clipboard image reading.
package clipboard

import (
	"fmt"
	"os/exec"
	"runtime"
)

// ReadImage reads image data from the system clipboard.
// Returns the PNG image bytes and nil error on success.
// Returns nil, nil if the clipboard does not contain image data.
// Returns nil, error if there was an error checking the clipboard.
func ReadImage() ([]byte, error) {
	switch runtime.GOOS {
	case "darwin":
		return readImageDarwin()
	case "linux":
		return readImageLinux()
	default:
		return nil, fmt.Errorf("clipboard image paste not supported on %s", runtime.GOOS)
	}
}

// readImageDarwin reads clipboard image data on macOS using osascript.
func readImageDarwin() ([]byte, error) {
	// Check if clipboard contains image data
	checkScript := `try
	clipboard info for «class PNGf»
	return "png"
on error
	try
		clipboard info for «class TIFF»
		return "tiff"
	on error
		return "none"
	end try
end try`

	out, err := exec.Command("osascript", "-e", checkScript).Output()
	if err != nil {
		return nil, fmt.Errorf("failed to check clipboard: %w", err)
	}

	format := string(out)
	if len(format) > 0 && format[len(format)-1] == '\n' {
		format = format[:len(format)-1]
	}

	if format == "none" {
		return nil, nil
	}

	// Read the image data as PNG (or convert TIFF to PNG via sips)
	if format == "png" {
		return readClipboardPNG()
	}
	// For TIFF, read and convert
	return readClipboardTIFF()
}

// readClipboardPNG reads PNG data directly from macOS clipboard.
func readClipboardPNG() ([]byte, error) {
	script := `set pngData to the clipboard as «class PNGf»
set tmpPath to (POSIX path of (path to temporary items folder)) & "pry-clipboard.png"
try
	set f to open for access tmpPath with write permission
	set eof of f to 0
	write pngData to f
	close access f
on error
	try
		close access tmpPath
	end try
	error "failed to write clipboard image"
end try
return tmpPath`

	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		return nil, fmt.Errorf("failed to read clipboard PNG: %w", err)
	}

	path := string(out)
	if len(path) > 0 && path[len(path)-1] == '\n' {
		path = path[:len(path)-1]
	}

	data, err := readAndRemove(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read temp file: %w", err)
	}
	return data, nil
}

// readClipboardTIFF reads TIFF data from macOS clipboard and converts to PNG via sips.
func readClipboardTIFF() ([]byte, error) {
	script := `set tiffData to the clipboard as «class TIFF»
set tmpPath to (POSIX path of (path to temporary items folder)) & "pry-clipboard.tiff"
try
	set f to open for access tmpPath with write permission
	set eof of f to 0
	write tiffData to f
	close access f
on error
	try
		close access tmpPath
	end try
	error "failed to write clipboard image"
end try
return tmpPath`

	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		return nil, fmt.Errorf("failed to read clipboard TIFF: %w", err)
	}

	tiffPath := string(out)
	if len(tiffPath) > 0 && tiffPath[len(tiffPath)-1] == '\n' {
		tiffPath = tiffPath[:len(tiffPath)-1]
	}

	// Convert TIFF to PNG using sips (available on all macOS)
	pngPath := tiffPath + ".png"
	if err := exec.Command("sips", "-s", "format", "png", tiffPath, "--out", pngPath).Run(); err != nil {
		removeFile(tiffPath)
		return nil, fmt.Errorf("failed to convert TIFF to PNG: %w", err)
	}
	removeFile(tiffPath)

	data, err := readAndRemove(pngPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read converted PNG: %w", err)
	}
	return data, nil
}

// readImageLinux reads clipboard image data on Linux using xclip.
func readImageLinux() ([]byte, error) {
	// Check for xclip
	if _, err := exec.LookPath("xclip"); err != nil {
		return nil, fmt.Errorf("xclip not found (install xclip for clipboard image support)")
	}

	// Try to read PNG from clipboard
	data, err := exec.Command("xclip", "-selection", "clipboard", "-t", "image/png", "-o").Output()
	if err != nil {
		// No image in clipboard
		return nil, nil
	}
	if len(data) == 0 {
		return nil, nil
	}
	return data, nil
}
