# termwright integration

Go client for [termwright](https://github.com/fcoury/termwright) — a Playwright-inspired
terminal automation framework. Enables end-to-end testing of the pry TUI by
driving it through a real PTY.

## How it works

termwright runs as a **daemon** that:
1. Spawns the TUI binary inside a PTY (pseudo-terminal)
2. Exposes a JSON-RPC API over a Unix socket
3. Supports screen reading, keyboard input, waiting for text, and screenshots

This Go client wraps that JSON-RPC protocol so we can write Ginkgo integration
tests that exercise the real TUI binary.

## Prerequisites

```bash
cargo install termwright   # Requires Rust 1.85+
```

Tests skip automatically if `termwright` is not installed.

## Example: testing a screen transition

```go
client, err := termwright.Spawn(120, 40, "./pry")
Expect(err).NotTo(HaveOccurred())
defer client.Close()

// Wait for PR list to load
err = client.WaitForText("Pull Requests", 10*time.Second)
Expect(err).NotTo(HaveOccurred())

// Navigate to first PR
client.Press("Enter")
err = client.WaitForText("Files changed", 10*time.Second)
Expect(err).NotTo(HaveOccurred())

// Verify screen content
screen, _ := client.Screen()
Expect(screen).To(ContainSubstring("Files changed"))
```

## Daemon JSON-RPC methods

| Method | Params | Description |
|--------|--------|-------------|
| `handshake` | — | Version/PID exchange |
| `screen` | `format` | Get screen (Text, Json, JsonCompact) |
| `type` | `text` | Type a string |
| `press` | `key` | Press a key (Enter, Escape, Up, q, …) |
| `hotkey` | `ctrl`, `alt`, `ch` | Key combo (Ctrl+C, etc.) |
| `wait_for_text` | `text`, `timeout_ms` | Block until text appears |
| `wait_for_text_gone` | `text`, `timeout_ms` | Block until text disappears |
| `wait_for_pattern` | `pattern`, `timeout_ms` | Block until regex matches |
| `wait_for_idle` | `idle_ms`, `timeout_ms` | Block until screen stabilizes |
| `wait_for_exit` | `timeout_ms` | Block until process exits |
| `resize` | `cols`, `rows` | Change terminal size |
| `status` | — | Check if process has exited |
| `screenshot` | `font`, `font_size` | Capture PNG screenshot |

## Limitations

- **Rust dependency**: termwright must be installed separately (`cargo install`)
- **macOS/Linux only**: No Windows support
- **No CI by default**: Tests require the binary; skip gracefully in CI unless provisioned
- **PTY-based**: Tests are slower than unit tests (~seconds per scenario)
- **GitHub auth**: Testing the real pry binary requires `gh` auth and a repo context

## Recommended usage

Use termwright tests for **smoke tests and screen transition verification**, not as a
replacement for unit tests. Good candidates:
- PR list loads and renders
- Key bindings navigate between screens (PRList → PRDetail → DiffView)
- Comment editor opens and accepts input
- Submit screen shows pending comments
- Ctrl+C / q exits cleanly
