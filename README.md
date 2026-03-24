# pry

A terminal UI for reviewing GitHub pull requests with inline comments and code suggestions — the workflow `gh pr review` doesn't support.

![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)

## Why

GitHub's `gh` CLI can list and merge PRs, but reviewing with inline line comments requires the web UI. This tool brings that workflow to your terminal: browse PRs, read diffs, leave comments on specific lines, suggest code changes, and submit your review — all without leaving the shell.

## Install

```bash
go install github.com/jethrokuan/pry/cmd@latest
```

Or build from source:

```bash
git clone https://github.com/jethrokuan/pry.git
cd pry
go build -o pry ./cmd/...
```

### Requirements

- [gh](https://cli.github.com/) CLI, authenticated (`gh auth login`)
- Go 1.25+ (build only)


## Usage

Run from any directory inside a GitHub-hosted git repository:

```bash
pry
```

### Flow

```
PR List → Diff View → Submit Review
              ↕
        Comment Editor
```

1. **PR List** — Browse open PRs filtered by review requests, all open, or authored by you. Preview PR details in the sidebar.
2. **Diff View** — Navigate files and diffs, leave inline comments and code suggestions. Mark files as viewed (synced to GitHub). Expand/fold comment threads with tab/shift-tab.
3. **Submit** — Choose Comment, Approve, or Request Changes and submit your review.

### Features

- **Mark files as viewed** — Press `m` to mark/unmark files, synced to GitHub via GraphQL (`markFileAsViewed`). Viewed files show a checkmark in the file tree.
- **Collapsible comment threads** — Inline comments are folded by default, showing a count. Press `tab` to expand, `shift+tab` to fold.
- **Pending/draft reviews** — On entering review, any existing pending (draft) review is fetched from GitHub. Its comments appear inline alongside submitted comments. On submit, new comments are included with the review.

## Keybindings

### PR List

| Key | Action |
|-----|--------|
| `j` / `k` | Navigate up/down |
| `enter` | Select PR |
| `tab` / `shift+tab` | Next/previous tab |
| `/` | Edit filter text |
| `r` | Refresh |
| `J` / `K` | Scroll preview down/up |
| `ctrl+d` / `ctrl+u` | Half page down/up |
| `w` | Open PR in browser |
| `y` | Copy PR number |
| `Y` | Copy PR URL |
| `?` | Show help |
| `ctrl+c` | Quit |

### Diff View

| Key | Action |
|-----|--------|
| `j` / `k` | Scroll diff |
| `ctrl+d` / `ctrl+u` | Page down/up |
| `f` / `F` | Next/previous file |
| `h` / `H` | Next/previous hunk |
| `c` / `C` | Next/previous comment |
| `t` | Toggle file tree |
| `enter` | New comment / open comments |
| `space` | Start visual selection |
| `r` | Reply to comment |
| `tab` | Toggle fold (comments/hunks) |
| `shift+tab` | Toggle all folds |
| `m` | Mark/unmark current file as viewed |
| `/` | Search in file |
| `n` / `N` | Next/previous search match |
| `ctrl+p` | Filter files (jump to file) |
| `T` then `o` | Toggle CODEOWNERS team filter |
| `T` then `f` | Narrow tree by regex path filter |
| `T` then `x` | Clear all filters |
| `ctrl+o` / `ctrl+i` | Jump back/forward in history |
| `i` | Toggle PR info popup |
| `w` | Open PR in browser |
| `y` | Copy link to current line |
| `ctrl+e` | Open file in `$EDITOR` at current line |
| `ctrl+s` | Submit review |
| `?` | Show help |
| `esc` | Back to PR list |

#### Comment Selection Mode

When a comment is selected:

| Key | Action |
|-----|--------|
| `j` / `k` | Navigate between comments |
| `enter` | Open comment popup |
| `r` | Reply to comment |
| `e` | Edit comment (pending only) |
| `d` | Delete comment (pending only) |
| `esc` | Deselect comment |

### Comment Editor

| Key | Action |
|-----|--------|
| `ctrl+s` | Save comment |
| `ctrl+t` | Toggle comment / suggestion mode |
| `ctrl+e` | Open in `$EDITOR` |
| `ctrl+v` | Paste image from clipboard |
| `esc` | Cancel |

### Submit Review

| Key | Action |
|-----|--------|
| `1` / `2` / `3` | Comment / Approve / Request Changes |
| `b` | Edit review body |
| `ctrl+s` | Submit |
| `esc` | Cancel (keep pending comments) |

## Code Suggestions

Press `enter` on a diff line to open the comment editor, then `ctrl+t` to switch to suggestion mode. For multi-line suggestions, use `space` to start a visual selection first. The editor opens pre-filled with the selected code. Edit the code to what it should be — on submission it gets wrapped in a GitHub suggestion block:

````markdown
```suggestion
your edited code here
```
````

Reviewers see a one-click "Apply suggestion" button on GitHub.

## Project Structure

```
cmd/root.go                  Entry point
internal/
  app/app.go                 Top-level screen router (Bubble Tea)
  github/
    client.go                GitHub API client (wraps go-gh)
    pr.go                    PR listing via GraphQL
    diff.go                  Diff/file fetching
    review.go                Review submission, inline comments
  git/checkout.go            Local git operations
  diff/
    model.go                 DiffFile, Hunk, DiffLine types
    parser.go                Unified diff parser + position mapping
    renderer.go              Built-in ANSI rendering
  ui/
    prlist/model.go          PR list screen
    diffview/model.go        Diff viewer with file tree
    comment/model.go         Comment/suggestion editor
    submit/model.go          Review submission screen
    styles/styles.go         Shared lipgloss styles
  config/config.go           Configuration
```

## Built With

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) — TUI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) — Styling
- [Bubbles](https://github.com/charmbracelet/bubbles) — TUI components (viewport, textarea, spinner)
- [Glamour](https://github.com/charmbracelet/glamour) — Markdown rendering
- [go-gh](https://github.com/cli/go-gh) — GitHub API client (reuses `gh` auth)

## License

MIT
