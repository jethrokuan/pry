# Developer Guide

## Domain Model

### PullRequest

The central domain type. Represents a forge-agnostic pull/merge request.

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
├── ReviewID int          ← forge review ID (0 if not yet created on forge)
├── ReviewNodeID string   ← GraphQL ID for mutations
├── Comments []InlineComment
├── Body string           ← review summary
├── Event ReviewEvent     ← COMMENT | APPROVE | REQUEST_CHANGES
└── ViewedFiles map[string]bool
```

The review is created lazily on the forge — `ReviewID` starts at 0 and gets populated when the first comment triggers `CreatePendingReview`. Comments are synced individually via `AddReviewComment` and tracked by `SyncStatus`.

### InlineComment

A pending comment with async sync tracking.

```
InlineComment
├── Path, Line, StartLine, Side, Body   ← content
├── LocalID int       ← monotonic ID assigned locally
├── ForgeID int       ← forge ID once synced (0 until then)
├── SyncStatus        ← Pending → InFlight → Complete | Failed
└── SyncError error
```

`LocalID` is the stable identifier during the review session. `ForgeID` appears after successful sync.

### ExistingComment

Read-only comment already submitted to the forge. Displayed alongside pending comments in the diff view.

### UserIdentity

The authenticated user's login and team memberships. Loaded asynchronously at app startup, stored on `AppContext`. Used for CODEOWNERS-based file filtering.

## Architecture

### Elm Architecture (Bubble Tea)

Every screen is a Bubble Tea `Model` with `Init()`, `Update(msg) (Model, Cmd)`, and `View()`. The top-level `app.Model` routes messages to the active screen.

### AppContext

Shared state passed to screens that need forge access:

```go
type Context struct {
    Svc          review.Service
    UserIdentity *review.UserIdentity  // nil until async load completes
}
```

Allocated once in `app.New()`, passed by pointer. When `UserIdentity` loads asynchronously, it's set on the context and all screens see it.

### Service Interface

All UI code depends on `review.Service`, never on the GitHub adapter directly. The interface covers:

- **Repo info**: `RepoOwner`, `RepoName`
- **Auth**: `CurrentUser`, `UserTeams`
- **PR operations**: `ListPRs`, `GetPR`, `FetchDiffFiles`
- **Comments & review**: `FetchCommentsAndReview`, `AddReviewComment`, `DeleteReviewComment`, `EditReviewComment`
- **Review lifecycle**: `CreatePendingReview`, `SubmitReview`
- **Viewed files**: `FetchViewedFiles`, `MarkFileAsViewed`, `UnmarkFileAsViewed`
- **Misc**: `ListMentionableUsers`, `UploadImage`

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
├── ctx *appctx.Context         ← shared: Svc + UserIdentity
├── selectedPR *PullRequest     ← the active PR (nil on PR list)
│   ├── PendingReview           ← created by pr.StartReview()
│   └── ExistingComments        ← fetched in diffview, stored on PR
├── prList prlist.Model         ← takes svc, filters, columns, cfg.PRList
│   ├── tabBar tabbar.Model    ← horizontal filter tab carousel
│   └── sidebar sidebar.Model  ← toggleable PR preview pane
├── diffView diffview.Model     ← takes ctx + pr (reads pr.PendingReview)
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
│   └── reviewtest/  Mock Service for tests
├── github/       GitHub adapter (implements review.Service)
├── app/          Top-level model, screen routing
├── ui/
│   ├── diffview/ Diff review screen (largest screen)
│   ├── prlist/   PR list + tab bar + sidebar preview
│   ├── prdetail/ PR metadata view
│   ├── submit/   Review submission
│   ├── components/tabbar/   Reusable horizontal tab carousel
│   ├── components/sidebar/  Reusable toggleable sidebar viewport
│   └── styles/   Theme + style definitions
├── diff/         Unified diff parser
├── config/       User configuration (TOML)
├── git/          Local git operations
├── testutil/     Test builders + mock service
└── ...
```
