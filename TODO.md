# TODO

## Architecture

### High Priority

- [x] Add `context.Context` to all `review.Service` methods
  - ~~Enables request cancellation on screen transitions (e.g., esc during in-flight comment syncs)~~
  - ~~Enables timeout control for API calls~~
  - ~~Create a cancellable context per screen transition in `app.go`~~
  - ~~Easier to add now than retrofit later~~
  - Done: All I/O methods on `Service` interface now accept `context.Context` as first parameter.
    Call sites use `context.Background()` — cancellable contexts per screen transition is a follow-up.

- [x] Break up the diffview god object (~2,900 lines, 40+ fields on Model)
  - ~~Extract `diffnav` — cursor, viewport scrolling, diff line mapping~~
  - ~~Extract `commentpanel` — inline editor, comment CRUD, sync orchestration~~
  - ~~Extract `searchbar` — search/filter/goto input modes~~
  - ~~Keep `diffview.Model` as a compositor that delegates to sub-components~~
  - ~~Makes each piece independently testable~~
  - Done: Model fields extracted into three sub-component structs (`DiffNav`, `CommentPanel`,
    `SearchBar`) in dedicated files. Pure navigation methods (`buildDiffLines`, `rebuildTreeRows`,
    `syncTreeCursorToFileCursor`, `syncTreeViewportToCursor`) moved to `DiffNav`. `Model` composes
    all three via `nav`, `comments`, `search` fields and acts as coordinator.

- [x] Fix viewport sync when comments are present
  - ~~The diff cursor can go offscreen when scrolling up (k) near comments — the viewport offset calculation doesn't account for the extra lines rendered by inline comments~~
  - ~~Cycling to comments (]c/[c) can land on a line where the comment block itself isn't visible~~
  - ~~Root cause: `syncViewportToCursor()` uses raw `diffCursor` position but `renderDiffWithCursor()` injects comment lines that shift the actual rendered line count~~
  - ~~Fix needs to map logical diff cursor position to rendered line position (accounting for expanded comment heights) when computing viewport offsets~~
  - Done: Added `renderedLineForCursor()` that maps logical diffCursor to rendered line position
    (accounting for hunk headers, comment blocks, and inter-hunk blank lines). `syncViewportToCursor()`
    now uses the rendered position. Added `syncViewportToCursorWithComments()` used by `]c`/`[c`
    navigation to ensure both the cursor line and its comment block are visible.

- [x] Cap inline comment display height and make comments scrollable
  - ~~Long comments currently consume unbounded vertical space and block diff scrolling~~
  - ~~Limit rendered comment height to a fraction of screen height (e.g., 1/3)~~
  - ~~Add a scrollable viewport within the comment region when content overflows~~
  - ~~Should work for both existing and pending comments~~
  - Done: Expanded comment blocks are capped at 1/3 of viewport height (min 5 lines).
    When truncated, an inline `… N more lines [enter to view all]` hint appears.
    Press `enter` on any line with comments to open a scrollable full-screen popup
    (j/k/pgup/pgdn to scroll, esc to close).

- [x] Parse and render comment markdown when displaying comments
  - ~~Comments currently display as raw markdown text~~
  - ~~Use Glamour (already a dependency via prdetail) to render comment bodies~~
  - ~~Applies to both existing comments and pending comments in the diff view~~
  - Done: Added `renderMarkdown()` helper using Glamour with auto-style and word wrap.
    Applied to inline expanded comments (`renderLineComments`), the comment popup
    (`buildCommentPopupContent`), and updated `commentRenderedLines` to account for
    rendered markdown line counts. Falls back to raw text on rendering errors.

### Medium Priority

- [x] Fix error handling
  - ~~`app.go:146`: `_ = gitpkg.CheckoutPR(...)` silently swallows errors — surface to user~~
  - ~~`diffview` stores a single `m.err` and `m.syncErr` — concurrent failures overwrite each other~~
  - ~~Successful syncs clear `m.syncErr = nil`, potentially erasing unrelated failures~~
  - ~~Use scoped errors (e.g., `m.loadErr`, `m.commentSyncErrors map[int]error`)~~
  - Done: Checkout errors now surface via `checkoutResultMsg` → `prdetail.SetCheckoutErr()`.
    Diffview replaced `m.err`/`m.syncErr` with scoped `m.loadErr`, `m.reviewCreateErr`,
    `m.viewedErr`, and `m.commentSyncErrors map[int]error`. Successful syncs only clear
    their own error entry, not unrelated failures.

- [ ] Narrow `review.Service` interface (Interface Segregation)
  - Current interface has 14 methods; split into focused interfaces (`PRLister`, `DiffFetcher`, `ReviewManager`, `ViewedTracker`)
  - Each screen accepts only the interfaces it needs
  - Reduces mock surface for tests

- [x] Collapse or invert Adapter/Client relationship
  - ~~`github/adapter.go` is 86 lines of pure pass-through boilerplate~~
  - ~~Option A: Make `Client` implement `review.Service` directly~~
  - ~~Option B (preferred): `Client` returns raw GitHub API types, `Adapter` owns all GitHub→domain type mapping~~
  - Done: Went with Option A — `Client` now implements `review.Service` directly.
    Deleted `adapter.go`. Renamed `Owner()`→`RepoOwner()`, `Repo()`→`RepoName()`,
    `FetchPRFiles`→`FetchDiffFiles` (absorbing the patch mapping), `AddCommentToPendingReview`→`AddReviewComment`.
    `PRFile` type unexported to `prFile` since it's no longer needed outside the package.

### Low Priority

- [x] Index comments for render performance
  - ~~`commentsForLine()` / `localPendingForLine()` do O(n) scans per line per render~~
  - ~~Build a `map[string][]Comment` index keyed by `"path:line:side"`, rebuilt on comment changes~~
  - Done: Added `existingIndex` and `localPendingIndex` maps to `CommentPanel`, keyed by
    `"path:line:side"`. `rebuildCommentIndex()` rebuilds both maps from source data and is
    called at every mutation point (comment load, add, edit, delete, sync). `commentsForLine()`
    and `localPendingForLine()` now do O(1) map lookups instead of O(n) scans.

- [ ] Add a CLI framework (cobra or flag)
  - Current arg parsing is manual `os.Args[1]` — no `--help`, no flags, no version
  - Will be needed for `--filter`, `--repo`, config file path, etc.

- [ ] Rate limiting / request deduplication
  - `markTreeItemViewed()` fires one API call per file in a folder (unbounded concurrency)
  - No cancellation of stale requests when rapidly switching files
  - Add a semaphore for batch operations; combine with context cancellation

- [x] Guard escape key against discarding unsaved content
  - ~~Pressing esc in the inline comment editor immediately closes it and discards any text~~
  - ~~If the textarea has content, require a double-esc or show a "discard? (esc again)" confirmation~~
  - ~~Same principle should apply to other input modes (search, filter) where the user has typed something~~
  - Done: First esc on a non-empty inline comment editor sets `confirmDiscard` and shows
    "Press esc again to discard" in the help line. Second esc closes the editor. Any other
    key resets the confirmation. Empty editors still close immediately on first esc.

- [x] Add loading state for `ScreenPRDetail` before body is loaded
  - ~~Currently renders an uninitialized `prdetail.Model` between screen switch and `prBodyLoadedMsg`~~
  - Done: Added `bodyLoaded` flag to `prdetail.Model` and `SetPR()` method. `app.go` now creates
    `prdetail.New()` immediately on PR selection (with partial data) showing header info and
    "Loading PR details..." for the body. When `prBodyLoadedMsg` arrives, `SetPR()` updates the
    PR data and sets the viewport content. Both the PR-list-selection and direct-PR-number paths
    are covered.

- [x] Add confirmation before quitting during an active review
  - ~~Too easy to accidentally hit `q` or `ctrl+c` and lose review state (pending comments, viewed files progress)~~
  - ~~Should prompt "You have N pending comments. Quit?" or similar when there's unsaved review work~~
  - ~~Could skip the prompt if there are no local-only comments (everything already synced to the forge)~~
  - Done: First `q`/`ctrl+c` in the DiffView with pending comments sets `confirmQuit` and shows
    "You have N pending comment(s). Press q again to quit." in the footer. Second press quits.
    Any other key resets the confirmation. Skips the prompt when there are no pending comments.

## Features

- [ ] File tree narrowing and unnarrowing
  - Narrow the file tree to show only a subset of files, hiding the rest
  - Key narrowing modes:
    - **My files**: narrow to files owned by me (via CODEOWNERS lookup)
    - **Path regex**: narrow by a user-entered path pattern
    - **Viewed/unviewed**: narrow to only unviewed or only viewed files
  - Should be toggleable — easy to narrow and widen back to the full tree
  - Could use a keybinding (e.g., `F` or `:narrow`) to enter narrowing mode with a picker for the mode
  - Narrowing should persist across file navigation but reset on PR switch

- [ ] Add a "My Pending Reviews" PR filter
  - Show PRs where I have a PENDING (draft) review — i.e., reviews I've started but not yet submitted
  - Useful for resuming in-progress reviews across sessions
  - Could be a new `PRFilter` value or combined into the existing filter cycle

- [ ] Improve "Needs My Review" filter to exclude PRs I've already approved
  - Currently uses GitHub's `review-requested:@me` search qualifier, which includes PRs where I've already submitted an approval
  - Client-side: filter out PRs where my latest review is APPROVED (requires fetching review state per PR)
  - Alternatively, combine search qualifiers or use GraphQL to check `latestReviews` in the list query

- [ ] Show a brief flash message when cycling/searching finds no matches
  - e.g., ]c with no comments → "No comments in this file"
  - ]F with all files viewed → "All files viewed"
  - /search with no hits → "No matches"
  - Flash should auto-dismiss after ~1-2 seconds or on next keypress
  - Display in the footer/status bar area

- [ ] Support alternative diff pagers — particularly difftastic
