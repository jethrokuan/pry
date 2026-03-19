// Package termwright provides a Go client for the termwright daemon's JSON-RPC
// API. termwright (https://github.com/fcoury/termwright) is a Playwright-inspired
// automation framework for terminal TUI applications.
//
// The daemon mode spawns a TUI in a PTY and exposes control over a Unix socket.
// This client connects to that socket, enabling Go integration tests to drive
// and assert against the running TUI.
//
// Prerequisites:
//   - Install termwright: cargo install termwright
//   - macOS or Linux (no Windows support)
package termwright

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// Client controls a termwright daemon session.
type Client struct {
	cmd      *exec.Cmd
	sockPath string
	conn     net.Conn
	reqID    atomic.Uint64
	mu       sync.Mutex // serializes writes
}

// request is a JSON-RPC request sent to the daemon.
type request struct {
	ID     uint64      `json:"id"`
	Method string      `json:"method"`
	Params interface{} `json:"params"`
}

// response is a JSON-RPC response from the daemon.
type response struct {
	ID     uint64           `json:"id"`
	Result json.RawMessage  `json:"result"`
	Error  *responseError   `json:"error,omitempty"`
}

type responseError struct {
	Code    int              `json:"code"`
	Message string           `json:"message"`
	Data    json.RawMessage  `json:"data,omitempty"`
}

func (e *responseError) Error() string {
	return fmt.Sprintf("termwright RPC error %d: %s", e.Code, e.Message)
}

// ScreenFormat controls how screen content is returned.
type ScreenFormat string

const (
	ScreenFormatText        ScreenFormat = "Text"
	ScreenFormatJSON        ScreenFormat = "Json"
	ScreenFormatJSONCompact ScreenFormat = "JsonCompact"
)

// Spawn starts a termwright daemon for the given command and connects to it.
// cols and rows set the virtual terminal size.
func Spawn(cols, rows int, command string, args ...string) (*Client, error) {
	// Create a temp directory for the socket
	tmpDir, err := os.MkdirTemp("", "termwright-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	sockPath := filepath.Join(tmpDir, "termwright.sock")

	// Build daemon command
	cmdArgs := []string{
		"daemon",
		"--cols", fmt.Sprintf("%d", cols),
		"--rows", fmt.Sprintf("%d", rows),
		"--socket", sockPath,
		"--",
		command,
	}
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.Command("termwright", cmdArgs...)
	cmd.Stdout = os.Stderr // daemon logs go to stderr for debugging
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("start termwright daemon: %w", err)
	}

	// Wait for the socket to appear
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(sockPath); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	conn, err := net.DialTimeout("unix", sockPath, 5*time.Second)
	if err != nil {
		cmd.Process.Kill()
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("connect to daemon socket: %w", err)
	}

	c := &Client{
		cmd:      cmd,
		sockPath: sockPath,
		conn:     conn,
	}

	// Perform handshake
	var hs struct {
		ProtocolVersion  string `json:"protocol_version"`
		TermwrightVersion string `json:"termwright_version"`
		PID              int    `json:"pid"`
	}
	if err := c.call("handshake", nil, &hs); err != nil {
		c.Close()
		return nil, fmt.Errorf("handshake: %w", err)
	}

	return c, nil
}

// call sends a JSON-RPC request and decodes the response into result.
func (c *Client) call(method string, params interface{}, result interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	id := c.reqID.Add(1)
	req := request{ID: id, Method: method, Params: params}

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	data = append(data, '\n')

	c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.conn.Write(data); err != nil {
		return fmt.Errorf("write request: %w", err)
	}

	c.conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	var resp response
	dec := json.NewDecoder(c.conn)
	if err := dec.Decode(&resp); err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.Error != nil {
		return resp.Error
	}

	if result != nil && resp.Result != nil {
		if err := json.Unmarshal(resp.Result, result); err != nil {
			return fmt.Errorf("unmarshal result: %w", err)
		}
	}
	return nil
}

// Screen returns the current terminal screen content as text.
func (c *Client) Screen() (string, error) {
	params := map[string]string{"format": string(ScreenFormatText)}
	var result string
	if err := c.call("screen", params, &result); err != nil {
		return "", err
	}
	return result, nil
}

// ScreenJSON returns the screen content as JSON (optimized for AI consumption).
func (c *Client) ScreenJSON() (json.RawMessage, error) {
	params := map[string]string{"format": string(ScreenFormatJSONCompact)}
	var result json.RawMessage
	if err := c.call("screen", params, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// TypeStr sends a string of characters as keyboard input.
func (c *Client) TypeStr(text string) error {
	return c.call("type", map[string]string{"text": text}, nil)
}

// Press sends a single key press (e.g. "Enter", "Escape", "Up", "Down", "q").
func (c *Client) Press(key string) error {
	return c.call("press", map[string]string{"key": key}, nil)
}

// Hotkey sends a key combination with modifiers.
func (c *Client) Hotkey(ctrl, alt bool, ch string) error {
	params := map[string]interface{}{
		"ctrl": ctrl,
		"alt":  alt,
		"ch":   ch,
	}
	return c.call("hotkey", params, nil)
}

// WaitForText blocks until the specified text appears on screen.
func (c *Client) WaitForText(text string, timeout time.Duration) error {
	params := map[string]interface{}{
		"text":       text,
		"timeout_ms": int(timeout.Milliseconds()),
	}
	return c.call("wait_for_text", params, nil)
}

// WaitForTextGone blocks until the specified text disappears from screen.
func (c *Client) WaitForTextGone(text string, timeout time.Duration) error {
	params := map[string]interface{}{
		"text":       text,
		"timeout_ms": int(timeout.Milliseconds()),
	}
	return c.call("wait_for_text_gone", params, nil)
}

// WaitForPattern blocks until the regex pattern matches screen content.
func (c *Client) WaitForPattern(pattern string, timeout time.Duration) error {
	params := map[string]interface{}{
		"pattern":    pattern,
		"timeout_ms": int(timeout.Milliseconds()),
	}
	return c.call("wait_for_pattern", params, nil)
}

// WaitForIdle blocks until the screen has been stable for the given duration.
func (c *Client) WaitForIdle(idle, timeout time.Duration) error {
	params := map[string]interface{}{
		"idle_ms":    int(idle.Milliseconds()),
		"timeout_ms": int(timeout.Milliseconds()),
	}
	return c.call("wait_for_idle", params, nil)
}

// WaitForExit blocks until the spawned process exits. Returns the exit code.
func (c *Client) WaitForExit(timeout time.Duration) (int, error) {
	params := map[string]interface{}{
		"timeout_ms": int(timeout.Milliseconds()),
	}
	var result struct {
		ExitCode int `json:"exit_code"`
	}
	if err := c.call("wait_for_exit", params, &result); err != nil {
		return -1, err
	}
	return result.ExitCode, nil
}

// Resize changes the terminal dimensions.
func (c *Client) Resize(cols, rows int) error {
	params := map[string]interface{}{
		"cols": cols,
		"rows": rows,
	}
	return c.call("resize", params, nil)
}

// Status returns whether the process has exited and its exit code.
func (c *Client) Status() (exited bool, exitCode int, err error) {
	var result struct {
		Exited   bool `json:"exited"`
		ExitCode int  `json:"exit_code"`
	}
	if err := c.call("status", nil, &result); err != nil {
		return false, -1, err
	}
	return result.Exited, result.ExitCode, nil
}

// Close terminates the daemon and cleans up resources.
func (c *Client) Close() error {
	if c.conn != nil {
		c.conn.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		c.cmd.Process.Kill()
		c.cmd.Wait()
	}
	if c.sockPath != "" {
		os.RemoveAll(filepath.Dir(c.sockPath))
	}
	return nil
}
