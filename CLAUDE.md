# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Is

A terminal UI (TUI) for reviewing GitHub pull requests with inline comments and code suggestions. Built with Go and the Bubble Tea framework.

## Developer Guide

See **[dev.md](dev.md)** for detailed documentation of the domain model, architecture, state ownership, and async patterns. Keep `dev.md` up to date when making changes to domain types, screen flow, or architectural patterns.

## Build & Run

```bash
# Build
go build -o pry ./cmd/...

# Run (must be inside a GitHub-hosted git repo with `gh` authenticated)
./pry

# Install globally
go install github.com/jethrokuan/pry/cmd@latest
```

## Testing

Tests use **Ginkgo v2** and **Gomega**. Always use Ginkgo for new tests.

```bash
# Run all tests (preferred — uses Ginkgo CLI)
ginkgo -r -v

# Run tests for a specific package
ginkgo -v ./internal/diff/
ginkgo -v ./internal/review/

# Also works via go test
go test ./...
```

No linter is configured.

## Architecture

**Bubble Tea (Elm-architecture) TUI** with a top-level screen router (`internal/app/app.go`) that owns all screen models and routes messages between them.

### Screen flow

```
PRList → PRDetail → DiffView → Submit
                       ↕
                  Comment Editor
```

Each screen is a Bubble Tea `Model` in its own package under `internal/ui/`. Screen transitions happen via typed messages (e.g., `prlist.PRSelectedMsg`, `diffview.OpenCommentMsg`) handled in `app.go`'s `Update` method.

### Key packages

- **`internal/review`** — Domain types (`PullRequest`, `InlineComment`, `PendingReview`, etc.) and `Service` interface. All UI code depends on this package, never on a forge-specific package directly.
- **`internal/github`** — GitHub API layer using `go-gh` (reuses `gh` CLI auth). The `Adapter` type implements `review.Service`. Uses both REST and GraphQL.
- **`internal/app`** — Top-level model, screen routing, state management (selected PR, pending review)
- **`internal/diff`** — Unified diff parser, data types (`DiffFile`, `Hunk`, `DiffLine`), ANSI renderer with optional `delta` integration
- **`internal/ui/*`** — Individual screen models (prlist, prdetail, diffview, comment, submit)
- **`internal/git`** — Local git operations (checkout, repo detection)

### Domain service pattern

UI packages depend on the `review.Service` interface, not the concrete GitHub client. This enables:
- Testing with mock implementations
- Adding new forge backends (GitLab, Gitea, etc.) without changing UI code

```
internal/review/   ← domain types + Service interface
internal/github/   ← implements review.Service via Adapter
internal/ui/*      ← imports review, never github
internal/app/      ← wires review.Service to screens
```

### Review state

`review.PendingReview` (created in `app.go` when entering review) accumulates `InlineComment`s as the user comments. On submit, all pending comments are sent with the review via the `Service` interface.

## Configuration

User configuration lives at `~/.config/pry/config.toml`. An annotated example is at `config.example.toml` in the repo root.

**Currently configurable:** theme selection (10 built-in themes), per-color overrides, editor command, delta toggle, page size, PR list filters, PR list columns, file tree owner filter.

**Not yet configurable (hardcoded):** keybindings, paddings/margins, border styles, comment height caps.

### Guidelines for contributors

- **Prefer configurability over hardcoding.** When adding new UI behavior, colors, or layout choices, expose them in `internal/config/config.go` with sensible defaults rather than hardcoding values.
- **Always update `config.example.toml`** when adding or changing a config option. The example file is the primary user-facing documentation for configuration.
- **Use semantic color names** from the Theme struct (e.g., `AccentPrimary`, `FgMuted`) rather than raw color literals in UI code.
- **Config loading** is in `internal/config/config.go`. Theme resolution is in `internal/ui/styles/theme.go`.

## Dependencies

- **Bubble Tea / Lip Gloss / Bubbles / Glamour** — TUI framework, styling, components, markdown rendering (all from Charm)
- **go-gh** — GitHub API client that reuses `gh` CLI authentication
- **Ginkgo v2 / Gomega** — BDD testing framework and matcher library
- **delta** (optional runtime) — Syntax-highlighted diff rendering

## Version Control (jj)

This project uses **Jujutsu (jj)** instead of raw git commands.

```bash
jj status              # Working copy status
jj diff                # Show changes
jj commit -m "msg"     # Commit current changes
jj new                 # Start a new change
jj git push            # Push to remote
jj bookmark create X   # Create a bookmark (branch)
jj log                 # View history
```

**Rules:**
- Use `jj` for ALL version control — do NOT use `git` directly
- Always commit to a topic bookmark, not `main`
- Use `jj git push` to push, never `git push`

## Autonomous Workers

This project uses `bd-patrol` to dispatch autonomous Claude Code instances in worktrees. If you are running as an autonomous worker, these rules are **mandatory**.

### Scope discipline

- **Only work on your assigned issue.** Do not fix unrelated bugs, refactor nearby code, add comments to unchanged files, or make "while I'm here" improvements.
- Keep your diff minimal. Every changed line must be justified by the issue description.
- If you discover a problem outside your issue's scope, file a new beads issue (`bd create`) and move on.

### Quality gates (must pass before closing)

1. `go build ./cmd/...` — build must succeed
2. `ginkgo -r -v` — all tests must pass
3. `jj diff` — review your own changes; remove anything unrelated

Do NOT close an issue if any gate fails.

### Error handling / fail gracefully

- If the task is ambiguous or underspecified, add notes to the issue (`bd update <id> --notes="..."`) explaining what's unclear and stop. Do not guess.
- If tests fail and the fix isn't obvious, add notes describing the failure and stop. Do not make speculative changes hoping they'll pass.
- If the build breaks, do not attempt more than one fix cycle. Add notes and stop.

### No external side effects

- Do NOT comment on GitHub issues/PRs, post to Slack, or interact with any external service (except creating a PR — see below).
- Do NOT modify CI/CD configuration, GitHub Actions workflows, or Makefiles unless the issue explicitly requires it.
- Do NOT push to `main`. Always work on a topic bookmark.

### Pull request required

- Autonomous workers **MUST** create a pull request before finishing. Work is not complete without a PR.
- Use `gh pr create` to open the PR against `main`.
- The PR **must** include:
  - A clear title summarizing the change
  - A **Summary** section explaining what the PR solves and why, explaining the motivation for the change
  - A **Test plan** section describing how the change was tested (e.g., which tests were added/run, manual verification steps)
- Do NOT stop or close the issue until the PR has been created and the URL is confirmed.

### Conflict avoidance

- Keep changes to the minimum set of files needed.
- Avoid editing high-contention files (`CLAUDE.md`, `go.mod`, `go.sum`, `cmd/main.go`, `internal/app/app.go`) unless the issue specifically requires it.
- If you must edit a shared file, make the smallest possible change.

<!-- BEGIN BEADS INTEGRATION v:1 profile:minimal hash:b9766037 -->
## Beads Issue Tracker

This project uses **bd (beads)** for issue tracking. Run `bd prime` to see full workflow context and commands.

### Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --claim  # Claim work
bd close <id>         # Complete work
```

### Rules

- Use `bd` for ALL task tracking — do NOT use TodoWrite, TaskCreate, or markdown TODO lists
- Run `bd prime` for detailed command reference and session close protocol
- Use `bd remember` for persistent knowledge — do NOT use MEMORY.md files

## Landing the Plane (Session Completion)

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `jj git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Build, tests
3. **Update issue status** - Close finished work, update in-progress items
4. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   bd dolt push
   jj git push
   jj log -r 'mine()'  # Verify your changes are pushed
   ```
5. **CREATE A PULL REQUEST** - This is MANDATORY (if code changed):
   ```bash
   gh pr create --title "Short description" --body "$(cat <<'EOF'
   ## Summary
   <What this PR does and why.>

   ## Test plan
   <How this was tested: new/existing tests run, manual verification, etc.>
   EOF
   )"
   ```
6. **Verify** - All changes committed, pushed, AND PR created
7. **Hand off** - Provide context for next session (if interactive)

**CRITICAL RULES:**
- Work is NOT complete until `jj git push` succeeds AND a PR is created
- NEVER stop before pushing and creating a PR - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push and create the PR
- If push or PR creation fails, resolve and retry until it succeeds
<!-- END BEADS INTEGRATION -->
