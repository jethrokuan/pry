# Developer Guide

## Domain Model

### PullRequest

The central domain type. Represents a GitHub pull request.

```
PullRequest
├── Identity: Number, NodeID, Title, Author, Branch, Base, URL, HeadSHA
├── Metadata: State, Draft, Labels, CreatedAt, UpdatedAt
├── Stats: Additions, Deletions, Files
├── CI: ChecksPass, ChecksSummary
├── Review Status: ReviewDecision, PendingTeams, MyReviewState
├── Content: Body
│
├── PendingReview *PendingReview   ← nil until user starts reviewing
└── ExistingComments []ExistingComment
```

`PullRequest` is always passed by pointer (`*PullRequest`) and shared across screens. When the user selects a PR, the app allocates one and all screens reference the same object. Updates to PR metadata (e.g., from async body fetch) happen in-place, preserving the `PendingReview` and `ExistingComments`.

### PendingReview

Models GitHub's pending/draft review — at most one per user per PR. Created via `pr.StartReview()`.

```
PendingReview
├── ReviewID int          ← GitHub review ID (0 if not yet created)
├── ReviewNodeID string   ← GraphQL ID for mutations
├── Comments []InlineComment
├── Body string           ← review summary
├── Event ReviewEvent     ← COMMENT | APPROVE | REQUEST_CHANGES
└── ViewedFiles map[string]bool
```

The review is created lazily on GitHub — `ReviewID` starts at 0 and gets populated when the first comment triggers `data.CreatePendingReview()`. Comments are synced individually via `data.AddReviewComment()` and tracked by `SyncStatus`.

### InlineComment

A pending comment with async sync tracking.

```
InlineComment
├── Path, Line, StartLine, Side, Body   ← content
├── LocalID int       ← monotonic ID assigned locally
├── ForgeID int       ← GitHub ID once synced (0 until then)
├── SyncStatus        ← Pending → InFlight → Complete | Failed
└── SyncError error
```

`LocalID` is the stable identifier during the review session. `ForgeID` appears after successful sync.

### ExistingComment

Read-only comment already submitted to GitHub. Displayed alongside pending comments in the diff view.

### UserIdentity

The authenticated user's login and team memberships. Loaded asynchronously at app startup, stored on `AppContext`. Used for CODEOWNERS-based file filtering.

## Architecture

### Elm Architecture (Bubble Tea)

Every screen is a Bubble Tea `Model` with `Init()`, `Update(msg) (Model, Cmd)`, and `View()`. The top-level `app.Model` routes messages to the active screen.

### Data Layer

The `internal/data` package provides package-level functions for all GitHub API calls. Initialized once via `data.Init()` in `cmd/root.go`. UI screens call `data.*` directly — no interfaces or dependency injection.

Key functions:
- **Repo info**: `data.RepoOwner()`, `data.RepoName()`
- **Auth**: `data.CurrentUser()`, `data.UserTeams()`
- **PR operations**: `data.FetchPullRequests()`, `data.FetchPR()`, `data.FetchDiffFiles()`
- **Comments & review**: `data.FetchCommentsAndReview()`, `data.AddReviewComment()`, `data.DeleteReviewComment()`, `data.EditReviewComment()`
- **Review lifecycle**: `data.CreatePendingReview()`, `data.SubmitReview()`
- **Viewed files**: `data.FetchViewedFiles()`, `data.MarkFileAsViewed()`, `data.UnmarkFileAsViewed()`
- **Misc**: `data.ListMentionableUsers()`

### Screen Flow

```
PRList ──[PRSelectedMsg]──→ DiffView ──[SubmitReviewMsg]──→ Submit
                               ↑                              │
                               └──────[CancelledMsg]───────────┘
                               │
                          [BackMsg] → PRList
```

An alternative path goes through PRDetail before DiffView (via `StartReviewMsg`).

### State Ownership

```
app.Model
├── selectedPR *PullRequest     ← the active PR (nil on PR list)
│   ├── PendingReview           ← created by pr.StartReview()
│   └── ExistingComments        ← fetched in diffview, stored on PR
├── prList prlist.Model         ← takes filters; calls data.* directly
│   ├── tabBar tabbar.Model    ← horizontal filter tab carousel
│   └── sidebar sidebar.Model  ← toggleable PR preview pane
├── diffView diffview.Model     ← takes pr (reads pr.PendingReview); calls data.*
└── submit submit.Model         ← takes ctx + pr (reads pr.PendingReview)
```

Key invariant: `selectedPR.PendingReview` is the single source of truth for review state. No screen has its own copy.

### Async Comment Sync

When the user adds a comment in diffview:

1. Comment added to `pr.PendingReview.Comments` with `SyncStatus = SyncPending`
2. If `ReviewID == 0`, `CreatePendingReview` is called first
3. Comment marked `SyncInFlight`, `AddReviewComment` called async
4. On success: `SyncComplete` + `ForgeID` set. On failure: `SyncFailed` + `SyncError` set
5. Submit screen waits for `InFlightCount() == 0` before allowing submission

### PR Update In-Place

When `prBodyLoadedMsg` arrives with full PR data from `GetPR`:

```go
pendingReview := m.selectedPR.PendingReview
existingComments := m.selectedPR.ExistingComments
*m.selectedPR = *msg.pr
m.selectedPR.PendingReview = pendingReview
m.selectedPR.ExistingComments = existingComments
```

This preserves review state while updating metadata (title, body, SHA, etc.).

## Package Layout

```
internal/
├── appctx/       AppContext (shared Svc + UserIdentity)
├── review/       Domain types + Service interface
├── data/         GitHub API (package-level functions, initialized via Init())
├── app/          Top-level model, screen routing
├── ui/
│   ├── diffview/ Diff review screen (largest screen)
│   ├── prlist/   PR list + tab bar + sidebar preview
│   ├── submit/   Review submission
│   ├── components/tabbar/   Reusable horizontal tab carousel
│   ├── components/sidebar/  Reusable toggleable sidebar viewport
│   └── styles/   Theme + style definitions
├── diff/         Unified diff parser
├── config/       User configuration (TOML)
├── git/          Local git operations
├── testutil/     Test builders
└── ...
```
