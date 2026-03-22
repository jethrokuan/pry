// Package logging configures slog to write to a log file so that log output
// does not interfere with the Bubble Tea TUI.
package logging

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

const logFileName = "debug.log"

// Setup configures the global slog logger to write to a file.
// When verbose is true the log level is set to Debug; otherwise it is Info.
// The log file is created at ~/.config/pry/debug.log.
// Returns a cleanup function that should be deferred by the caller.
func Setup(verbose bool) (cleanup func(), err error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to determine config dir: %w", err)
	}

	logDir := filepath.Join(dir, "pry")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create log dir %s: %w", logDir, err)
	}

	logPath := filepath.Join(logDir, logFileName)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file %s: %w", logPath, err)
	}

	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}

	handler := slog.NewTextHandler(f, &slog.HandlerOptions{
		Level: level,
	})
	slog.SetDefault(slog.New(handler))

	slog.Info("pry started", "verbose", verbose, "logFile", logPath)

	return func() { f.Close() }, nil
}
