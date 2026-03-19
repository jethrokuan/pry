#!/usr/bin/env bash
# bd-patrol: Watch for ready beads tasks and spawn Claude Code worktrees to work on them.
# Also auto-merges PRs that pass CI and periodically updates local main.
#
# Usage:
#   ./scripts/bd-patrol.sh                  # default: poll every 5s
#   ./scripts/bd-patrol.sh --interval 30    # poll every 30s
#   ./scripts/bd-patrol.sh --dry-run        # show what would be spawned, don't run
#   ./scripts/bd-patrol.sh --max-workers 3  # limit concurrent worktrees (default: 5)
#   ./scripts/bd-patrol.sh --once           # run once and exit (no loop)
#   ./scripts/bd-patrol.sh --no-auto-merge  # disable auto-merge of passing PRs
#   ./scripts/bd-patrol.sh --merge-strategy squash  # merge strategy: merge|squash|rebase (default: squash)

set -euo pipefail

# --- Config ---
POLL_INTERVAL=5
DRY_RUN=false
MAX_WORKERS=5
ONCE=false
CLAUDE_MODEL=""
AUTO_MERGE=true
MERGE_STRATEGY="squash"
MAIN_UPDATE_INTERVAL=60  # seconds between main branch updates
LAST_MAIN_UPDATE=0

# --- Parse args ---
while [[ $# -gt 0 ]]; do
  case "$1" in
    --interval)   POLL_INTERVAL="$2"; shift 2 ;;
    --dry-run)    DRY_RUN=true; shift ;;
    --max-workers) MAX_WORKERS="$2"; shift 2 ;;
    --once)       ONCE=true; shift ;;
    --model)      CLAUDE_MODEL="$2"; shift 2 ;;
    --no-auto-merge) AUTO_MERGE=false; shift ;;
    --merge-strategy) MERGE_STRATEGY="$2"; shift 2 ;;
    --main-update-interval) MAIN_UPDATE_INTERVAL="$2"; shift 2 ;;
    -h|--help)
      sed -n '2,/^$/s/^# //p' "$0"
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      exit 1
      ;;
  esac
done

# --- State ---
REPO_ROOT="$(git rev-parse --show-toplevel)"
STATE_DIR="${REPO_ROOT}/.bd-patrol"
mkdir -p "$STATE_DIR"

log() { echo "[$(date '+%H:%M:%S')] $*"; }

cleanup() {
  log "Shutting down..."
  rm -rf "$STATE_DIR"
  exit 0
}
trap cleanup SIGINT SIGTERM

# Count currently in_progress issues as active workers
count_active_workers() {
  bd list --status=in_progress --json 2>/dev/null | jq 'length' 2>/dev/null || echo 0
}

spawn_worker() {
  local issue_id="$1"
  local title="$2"
  local description="$3"
  local issue_type="$4"
  local priority="$5"

  local ws_name="w$(head -c 4 /dev/urandom | xxd -p)"

  if $DRY_RUN; then
    log "[DRY RUN] Would spawn worker for $issue_id: $title"
    return
  fi

  # Claim the issue before spawning so bd ready won't return it again
  bd update "$issue_id" --claim 2>/dev/null || true

  log "Spawning worker for $issue_id ($ws_name): $title"

  local model_flag=""
  if [[ -n "$CLAUDE_MODEL" ]]; then
    model_flag="--model $CLAUDE_MODEL"
  fi

  local prompt
  prompt="$(cat <<PROMPT
You are an autonomous worker. Follow CLAUDE.md strictly — especially the "Autonomous Workers" section.

## Your assignment
Issue: $issue_id
Title: $title
Type: $issue_type | Priority: $priority
Description: $description

## Workflow
1. bd show $issue_id                    — read full issue details
2. Implement ONLY what the issue asks   — no drive-by fixes
3. go build ./cmd/...                   — must pass
4. ginkgo -r -v                         — must pass
5. jj diff                              — review your own diff, remove anything unrelated
6. jj commit -m "<issue_id>: <summary>"
7. bd close $issue_id
8. bd dolt push && jj git push          — work is NOT done until pushed

If any quality gate fails or the task is ambiguous, add notes (bd update $issue_id --notes="...") and stop.
PROMPT
)"

  # shellcheck disable=SC2086
  zellij run --name "$ws_name" -- \
    bash -c "claude -p --verbose --output-format stream-json --worktree \"$ws_name\" --dangerously-skip-permissions $model_flag \"\$1\" | jq -rj '
      if .type == \"assistant\" then
        (.message.content[]? |
          if .type == \"text\" then .text + \"\\n\"
          elif .type == \"tool_use\" then \"→ \" + .name + \" \" + (.input | tostring | .[0:120]) + \"\\n\"
          elif .type == \"tool_result\" then \"  \" + (.content // \"\" | tostring | .[0:200]) + \"\\n\"
          else empty end)
      elif .type == \"result\" then
        \"\\n✓ Done\\n\"
      else empty end
    '" -- "$prompt"

  # Restore focus to the bd-patrol pane so workers don't steal focus
  zellij action focus-previous-pane

  log "Worker launched for $issue_id ($ws_name)"
}

# --- Auto-merge passing PRs ---
auto_merge_prs() {
  if ! $AUTO_MERGE || $DRY_RUN; then
    return
  fi

  # List open PRs targeting main, authored by the current user (the bot operator)
  local prs
  prs="$(gh pr list --base main --json number,headRefName,statusCheckRollup,isDraft --limit 50 2>/dev/null || echo "[]")"

  local count
  count="$(echo "$prs" | jq 'length')"
  if [[ "$count" -eq 0 ]]; then
    return
  fi

  echo "$prs" | jq -c '.[]' | while IFS= read -r pr; do
    local pr_num head_branch is_draft
    pr_num="$(echo "$pr" | jq -r '.number')"
    head_branch="$(echo "$pr" | jq -r '.headRefName')"
    is_draft="$(echo "$pr" | jq -r '.isDraft')"

    # Skip draft PRs
    if [[ "$is_draft" == "true" ]]; then
      continue
    fi

    # Check if all status checks passed (or no checks exist)
    local checks_passed
    checks_passed="$(echo "$pr" | jq '
      .statusCheckRollup
      | if . == null or length == 0 then true
        else all(.[]; .state == "SUCCESS" or (.status == "COMPLETED" and .conclusion == "SUCCESS"))
        end
    ')"

    if [[ "$checks_passed" == "true" ]]; then
      log "Auto-merging PR #${pr_num} (${head_branch}) — all checks passed"
      if gh pr merge "$pr_num" "--${MERGE_STRATEGY}" --delete-branch 2>&1; then
        log "Successfully merged PR #${pr_num}"
      else
        log "Failed to merge PR #${pr_num} — may need manual intervention"
      fi
    fi
  done
}

# --- Update local main branch ---
update_main() {
  local now
  now="$(date +%s)"
  local elapsed=$(( now - LAST_MAIN_UPDATE ))

  if [[ $elapsed -lt $MAIN_UPDATE_INTERVAL ]]; then
    return
  fi

  LAST_MAIN_UPDATE="$now"
  log "Updating local main branch..."
  if jj git fetch 2>/dev/null; then
    log "Main branch updated"
  else
    log "Warning: failed to update main branch"
  fi
}

# --- Main loop ---
log "bd-patrol starting (interval=${POLL_INTERVAL}s, max-workers=${MAX_WORKERS}, dry-run=${DRY_RUN}, auto-merge=${AUTO_MERGE})"

while true; do
  # Auto-merge passing PRs and update main before dispatching new workers
  auto_merge_prs
  update_main

  active_count="$(count_active_workers)"
  available_slots=$(( MAX_WORKERS - active_count ))

  if [[ $available_slots -le 0 ]]; then
    log "All $MAX_WORKERS worker slots occupied ($active_count in_progress), waiting..."
  else
    # Fetch ready issues as JSON
    ready_json="$(bd ready --json --limit "$available_slots" 2>/dev/null || echo "[]")"
    issue_count="$(echo "$ready_json" | jq 'length')"

    if [[ "$issue_count" -eq 0 ]]; then
      log "No ready issues found ($active_count active)"
    else
      log "Found $issue_count ready issue(s), $available_slots slot(s) available"

      # Use process substitution to avoid subshell (preserves array mutations)
      while IFS= read -r issue; do
        id="$(echo "$issue" | jq -r '.id')"
        title="$(echo "$issue" | jq -r '.title')"
        description="$(echo "$issue" | jq -r '.description // ""')"
        issue_type="$(echo "$issue" | jq -r '.issue_type // "task"')"
        priority="$(echo "$issue" | jq -r '.priority // 2')"

        spawn_worker "$id" "$title" "$description" "$issue_type" "$priority"
      done < <(echo "$ready_json" | jq -c '.[]')
    fi
  fi

  if $ONCE; then
    log "Done (--once mode)"
    exit 0
  fi

  sleep "$POLL_INTERVAL"
done
