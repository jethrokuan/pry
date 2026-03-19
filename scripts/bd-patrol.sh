#!/usr/bin/env bash
# bd-patrol: Watch for ready beads tasks, spawn Claude Code worktrees, merge completed work.
# Fully local — no GitHub PRs. Workers commit in jj worktrees, patrol merges into main.
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

# Active workers: track via sentinel files in STATE_DIR
# Running: $STATE_DIR/<issue_id>.worker  (contains worktree name + PID)
# Done:    $STATE_DIR/<issue_id>.exit    (contains exit code)

log() { echo "[$(date '+%H:%M:%S')] $*"; }

cleanup() {
  log "Shutting down..."
  rm -rf "$STATE_DIR"
  exit 0
}
trap cleanup SIGINT SIGTERM

# --- Count active workers ---
count_active_workers() {
  local count=0
  shopt -s nullglob
  for f in "$STATE_DIR"/*.worker; do
    count=$(( count + 1 ))
  done
  shopt -u nullglob
  echo "$count"
}

# --- Merge a worker's commits into main ---
merge_worker() {
  local issue_id="$1"
  local ws_name="$2"

  # Find the worker's commit (parent of the worktree's working copy)
  local change_id
  change_id="$(jj log --no-graph -r "${ws_name}@-" -T 'change_id' 2>/dev/null || echo "")"

  if [[ -z "$change_id" ]]; then
    log "No commit found for workspace $ws_name, nothing to merge"
    return 1
  fi

  # Check if the commit is empty
  if jj diff -r "$change_id" --stat 2>/dev/null | grep -q 'no changes'; then
    log "Worker $ws_name produced no changes, skipping merge"
    return 1
  fi

  # Rebase the commit onto main, then advance main
  if jj rebase -r "$change_id" -d main 2>&1; then
    jj bookmark set main -r "$change_id" 2>&1
    log "Merged $issue_id into main (main now at $change_id)"
    return 0
  else
    log "Failed to rebase $issue_id onto main — may need manual resolution"
    return 1
  fi
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

# --- Reap completed workers ---
reap_workers() {
  shopt -s nullglob
  local exit_files=("$STATE_DIR"/*.exit)
  shopt -u nullglob
  if [[ ${#exit_files[@]} -eq 0 ]]; then return; fi
  for exit_file in "${exit_files[@]}"; do

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
      log "Worker for $issue_id completed, merging $ws_name into main..."
      if merge_worker "$issue_id" "$ws_name"; then
        bd close "$issue_id" 2>/dev/null || true
      fi
    else
      log "Worker for $issue_id failed (exit $exit_code)"
      bd update "$issue_id" --notes="Worker failed with exit code $exit_code" 2>/dev/null || true
    fi

    cleanup_worktree "$ws_name"
    rm -f "$worker_file" "$exit_file"
  done
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

Do NOT push, do NOT create PRs. Just commit locally. Patrol will merge.
If any quality gate fails or the task is ambiguous, add notes (bd update $issue_id --notes="...") and stop.
PROMPT
)"

  # Track the worker
  echo "$ws_name" > "$STATE_DIR/${issue_id}.worker"

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
    '; echo \$? > \"$STATE_DIR/${issue_id}.exit\"" -- "$prompt"

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
