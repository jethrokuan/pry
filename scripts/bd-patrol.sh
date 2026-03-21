#!/usr/bin/env bash
# bd-patrol: Watch for ready beads tasks, spawn Claude Code worktrees, land completed work.
# Fully local — workers commit, patrol rebases onto main sequentially (no races).
#
# Usage:
#   ./scripts/bd-patrol.sh                  # default: poll every 5s
#   ./scripts/bd-patrol.sh --interval 30    # poll every 30s
#   ./scripts/bd-patrol.sh --dry-run        # show what would be spawned, don't run
#   ./scripts/bd-patrol.sh --max-workers 3  # limit concurrent worktrees (default: 5)
#   ./scripts/bd-patrol.sh --once           # run once and exit (no loop)

set -euo pipefail

# --- Config ---
POLL_INTERVAL=5
DRY_RUN=false
MAX_WORKERS=5
ONCE=false
CLAUDE_MODEL=""

# --- Parse args ---
while [[ $# -gt 0 ]]; do
  case "$1" in
    --interval)     POLL_INTERVAL="$2"; shift 2 ;;
    --dry-run)      DRY_RUN=true; shift ;;
    --max-workers)  MAX_WORKERS="$2"; shift 2 ;;
    --once)         ONCE=true; shift ;;
    --model)        CLAUDE_MODEL="$2"; shift 2 ;;
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
REPO_ROOT="$(jj workspace root 2>/dev/null || git rev-parse --show-toplevel)"
STATE_DIR="${REPO_ROOT}/.bd-patrol"
mkdir -p "$STATE_DIR"

# State machine via sentinel files in STATE_DIR:
# .worker        — claude worker running (contains ws_name)
# .exit          — worker finished (contains exit code)
# .done          — worker succeeded, queued for landing (contains ws_name)
# .resolver      — conflict resolver running (contains ws_name)
# .resolver-exit — resolver finished (contains exit code)

log() { echo "[$(date '+%H:%M:%S')] $*"; }

cleanup() {
  log "Shutting down..."
  rm -rf "$STATE_DIR"
  exit 0
}
trap cleanup SIGINT SIGTERM

# --- Count active workers (workers + resolvers) ---
count_active_workers() {
  local count=0
  shopt -s nullglob
  for f in "$STATE_DIR"/*.worker "$STATE_DIR"/*.resolver; do
    count=$(( count + 1 ))
  done
  shopt -u nullglob
  echo "$count"
}

# --- Land a completed worker's commit onto main ---
# Returns: 0 = landed, 1 = failed (empty/missing), 2 = resolver spawned
land_worker() {
  local issue_id="$1"
  local ws_name="$2"

  # Find the worker's commit (parent of the worktree's working copy)
  local change_id
  change_id="$(jj log --no-graph -r "${ws_name}@-" -T 'change_id' 2>/dev/null || echo "")"

  if [[ -z "$change_id" ]]; then
    log "No commit found for workspace $ws_name"
    return 1
  fi

  # Check if the commit is empty
  if jj log --no-graph -r "$change_id" -T 'empty' 2>/dev/null | grep -q 'true'; then
    log "Worker $ws_name produced empty commit"
    return 1
  fi

  # Rebase the worker's commit onto main (jj never fails — conflicts are materialized)
  log "Rebasing $issue_id onto main..."
  jj rebase -r "$change_id" -d main 2>/dev/null

  # Check for conflicts
  if jj log --no-graph -r "$change_id" -T 'conflict' 2>/dev/null | grep -q 'true'; then
    log "Conflict detected landing $issue_id — spawning resolver"
    spawn_resolver "$issue_id" "$ws_name" "$change_id"
    return 2
  fi

  # No conflicts — advance main
  jj bookmark set main -r "$change_id" 2>/dev/null
  log "Landed $issue_id on main"

  # Close the issue
  bd close "$issue_id" 2>/dev/null || true

  # Clean up workspace and worktree directory
  cleanup_worktree "$ws_name"
  return 0
}

# --- Clean up a worktree ---
cleanup_worktree() {
  local ws_name="$1"
  jj workspace forget "$ws_name" 2>/dev/null || true
  local ws_path="${REPO_ROOT}/.worktrees/${ws_name}"
  if [[ -d "$ws_path" ]]; then
    rm -rf "$ws_path"
  fi
}

# --- Reap completed workers and land sequentially ---
reap_workers() {
  shopt -s nullglob

  # Phase 1: Collect completed workers → .done files
  for exit_file in "$STATE_DIR"/*.exit; do
    local issue_id
    issue_id="$(basename "$exit_file" .exit)"
    local worker_file="$STATE_DIR/${issue_id}.worker"

    if [[ ! -f "$worker_file" ]]; then
      rm -f "$exit_file"
      continue
    fi

    local ws_name exit_code
    ws_name="$(head -1 "$worker_file")"
    exit_code="$(cat "$exit_file")"

    if [[ "$exit_code" -eq 0 ]]; then
      log "Worker for $issue_id completed, queuing for landing"
      mv "$worker_file" "$STATE_DIR/${issue_id}.done"
    else
      log "Worker for $issue_id failed (exit $exit_code)"
      bd update "$issue_id" --notes="Worker failed with exit code $exit_code" 2>/dev/null || true
      cleanup_worktree "$ws_name"
      rm -f "$worker_file"
    fi
    rm -f "$exit_file"
  done

  # Phase 2: Collect completed resolvers → back to .done
  for exit_file in "$STATE_DIR"/*.resolver-exit; do
    local issue_id
    issue_id="$(basename "$exit_file" .resolver-exit)"
    local resolver_file="$STATE_DIR/${issue_id}.resolver"

    if [[ ! -f "$resolver_file" ]]; then
      rm -f "$exit_file"
      continue
    fi

    local ws_name exit_code
    ws_name="$(head -1 "$resolver_file")"
    exit_code="$(cat "$exit_file")"

    if [[ "$exit_code" -eq 0 ]]; then
      log "Resolver for $issue_id completed, re-queuing for landing"
      echo "$ws_name" > "$STATE_DIR/${issue_id}.done"
    else
      log "Resolver for $issue_id failed (exit $exit_code)"
      bd update "$issue_id" --notes="Resolver failed with exit code $exit_code — needs manual intervention" 2>/dev/null || true
      cleanup_worktree "$ws_name"
    fi
    rm -f "$resolver_file" "$exit_file"
  done

  # Phase 3: Sequential landing queue (one at a time — no races on main)
  for done_file in "$STATE_DIR"/*.done; do
    local issue_id
    issue_id="$(basename "$done_file" .done)"
    local ws_name
    ws_name="$(head -1 "$done_file")"

    land_worker "$issue_id" "$ws_name"
    local rc=$?

    if [[ $rc -eq 0 ]]; then
      # Landed successfully — worktree already cleaned up in land_worker
      rm -f "$done_file"
    elif [[ $rc -eq 2 ]]; then
      # Resolver spawned — remove .done (now tracked by .resolver)
      rm -f "$done_file"
    else
      # Failed (empty/missing commit)
      bd update "$issue_id" --notes="Landing failed — empty or missing commit" 2>/dev/null || true
      cleanup_worktree "$ws_name"
      rm -f "$done_file"
    fi
  done

  shopt -u nullglob
}

# --- Spawn a conflict resolver ---
spawn_resolver() {
  local issue_id="$1"
  local ws_name="$2"
  local change_id="$3"

  local model_flag=""
  if [[ -n "$CLAUDE_MODEL" ]]; then
    model_flag="--model $CLAUDE_MODEL"
  fi

  local prompt
  prompt="$(cat <<PROMPT
You are a conflict resolver. A commit for issue $issue_id was rebased onto main but has conflicts.

## Your task
1. jj workspace update-stale          — ensure worktree reflects the rebased state
2. Resolve ALL conflict markers in the working copy
3. go build ./cmd/...                  — must pass after resolution
4. ginkgo -r -v                        — must pass after resolution
5. jj squash                           — fold your resolution into the conflicted commit

Do NOT create new commits. Just resolve conflicts in-place and squash.
Do NOT rebase or move bookmarks. The patrol process handles that.
If conflicts are too complex to resolve, add notes (bd update $issue_id --notes="...") and exit 1.
PROMPT
)"

  # Track as resolver
  echo "$ws_name" > "$STATE_DIR/${issue_id}.resolver"

  # shellcheck disable=SC2086
  zellij run --name "${ws_name}-resolve" -- \
    bash -c "trap '' INT TSTP QUIT; exec < /dev/null; claude -p --verbose --output-format stream-json --worktree \"$ws_name\" --dangerously-skip-permissions $model_flag \"\$1\" | claude-pretty-printer --layout compact; echo \${PIPESTATUS[0]} > \"$STATE_DIR/${issue_id}.resolver-exit\"" -- "$prompt"

  zellij action focus-previous-pane

  log "Resolver launched for $issue_id ($ws_name)"
}

# --- Spawn a worker ---
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

  # Claim atomically — if it fails, someone else got it
  local claim_err
  if ! claim_err="$(bd update "$issue_id" --claim 2>&1)"; then
    log "Failed to claim $issue_id: $claim_err — skipping"
    return
  fi

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
6. jj commit -m "$issue_id: <summary>"  — commit with issue ID in message
7. bd update $issue_id --notes="<summary of what you changed, files modified, and why>"

Do NOT rebase, do NOT move bookmarks, do NOT close the issue, do NOT push, do NOT create PRs.
Just commit and stop. The patrol process will land your change onto main.
If any quality gate fails or the task is ambiguous, add notes (bd update $issue_id --notes="...") and stop.
PROMPT
)"

  # Track the worker
  echo "$ws_name" > "$STATE_DIR/${issue_id}.worker"

  # shellcheck disable=SC2086
  zellij run --name "$ws_name" -- \
    bash -c "trap '' INT TSTP QUIT; exec < /dev/null; claude -p --verbose --output-format stream-json --worktree \"$ws_name\" --dangerously-skip-permissions $model_flag \"\$1\" | claude-pretty-printer --layout compact; echo \${PIPESTATUS[0]} > \"$STATE_DIR/${issue_id}.exit\"" -- "$prompt"

  # Restore focus to the bd-patrol pane
  zellij action focus-previous-pane

  log "Worker launched for $issue_id ($ws_name)"
}

# --- Main loop ---
log "bd-patrol starting (interval=${POLL_INTERVAL}s, max-workers=${MAX_WORKERS}, dry-run=${DRY_RUN})"

while true; do
  reap_workers

  active_count="$(count_active_workers)"
  available_slots=$(( MAX_WORKERS - active_count ))

  if [[ $available_slots -le 0 ]]; then
    log "All $MAX_WORKERS worker slots occupied, waiting..."
  else
    ready_json="$(bd ready --json --limit "$available_slots" 2>/dev/null || echo "[]")"
    issue_count="$(echo "$ready_json" | jq 'length')"

    if [[ "$issue_count" -eq 0 ]]; then
      if [[ "$active_count" -gt 0 ]]; then
        log "No ready issues ($active_count worker(s) active)"
      fi
    else
      log "Found $issue_count ready issue(s), $available_slots slot(s) available"

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
