#!/usr/bin/env bash
# qodana-pr-review.sh — PR-mode Qodana scan for this worktree (native, no Docker).
#
# 1. Resolves the qodana CLI (PATH, /opt/homebrew/bin, ~/homebrew/bin); waits
#    and retries if it is still being installed via brew.
# 2. Resolves QODANA_TOKEN (env, ai-specs.env of this worktree or of the main
#    checkout, ~/.qodana/token, .qodana/token).
#    The release Go linter (qodana-go) requires it — there is no Community
#    edition for Go (verified against CLI 2026.2.2 / Docker Hub).
# 3. Ensures a baseline SARIF exists at .qodana/baseline.sarif.json. If
#    missing, scans the merge-base with the base branch from a temporary
#    local clone (a linked worktree's .git points outside a docker mount).
# 4. Scans the current worktree in PR mode (-b baseline) natively, reporting
#    only NEW problems, grouped by severity with file:line.
#
# Usage: qodana-pr-review.sh [base-branch]   (default: development)
# Env:   QODANA_TOKEN, QODANA_MAX_WAIT_SECS (default 900), QODANA_DOCKER=1
#        to fall back to docker mode instead of native.
set -euo pipefail

BASE_BRANCH="${1:-${QODANA_BASE_BRANCH:-development}}"
LINTER="qodana-go"            # CLI 2026.2+ value; 'qodana-go-community' is rejected (legacy name)
BASELINE=".qodana/baseline.sarif.json"
RESULTS=".qodana/results"
MAX_WAIT="${QODANA_MAX_WAIT_SECS:-900}"

ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"
mkdir -p .qodana

resolve_qodana() {
  if command -v qodana >/dev/null 2>&1; then command -v qodana; return 0; fi
  for candidate in /opt/homebrew/bin/qodana "$HOME/homebrew/bin/qodana"; do
    if [ -x "$candidate" ]; then echo "$candidate"; return 0; fi
  done
  return 1
}

QODANA="$(resolve_qodana || true)"
if [ -z "${QODANA:-}" ]; then
  waited=0
  step=15
  echo "qodana CLI not found; waiting up to ${MAX_WAIT}s for the brew install to finish..."
  while [ "$waited" -lt "$MAX_WAIT" ]; do
    sleep "$step"; waited=$((waited + step))
    if QODANA="$(resolve_qodana)"; then break; fi
    echo "  still waiting (${waited}s)..."
  done
  [ -n "${QODANA:-}" ] || { echo "ERROR: qodana CLI not found after ${MAX_WAIT}s." >&2; exit 1; }
fi
echo "qodana CLI: $QODANA ($("$QODANA" --version))"

# --- token (release Go linter requires it) ----------------------------------
if [ -z "${QODANA_TOKEN:-}" ]; then
  # ai-specs.env is the repo's direnv-loaded env file (git-ignored); parse it
  # directly so the skill works even in sessions where direnv has not reloaded.
  if [ -f "$ROOT/ai-specs.env" ]; then
    token_from_env_file="$(grep -E '^QODANA_TOKEN=' "$ROOT/ai-specs.env" | tail -1 | cut -d= -f2- | tr -d '"' | tr -d '[:space:]')"
    [ -n "$token_from_env_file" ] && export QODANA_TOKEN="$token_from_env_file"
  fi
  # Worktrees inherit direnv from the main checkout's .envrc (ancestor dirs);
  # read the main repo's env file directly too, so sessions launched before a
  # direnv reload still see the token.
  main_env="$(git rev-parse --path-format=absolute --git-common-dir 2>/dev/null)/../ai-specs.env"
  if [ -z "${QODANA_TOKEN:-}" ] && [ -f "$main_env" ]; then
    token_from_env_file="$(grep -E '^QODANA_TOKEN=' "$main_env" | tail -1 | cut -d= -f2- | tr -d '"' | tr -d '[:space:]')"
    [ -n "$token_from_env_file" ] && export QODANA_TOKEN="$token_from_env_file"
  fi
fi
if [ -z "${QODANA_TOKEN:-}" ]; then
  for token_file in "$HOME/.qodana/token" "$ROOT/.qodana/token"; do
    if [ -s "$token_file" ]; then
      export QODANA_TOKEN="$(tr -d '[:space:]' < "$token_file")"
      echo "QODANA_TOKEN loaded from $token_file"
      break
    fi
  done
fi
[ -n "${QODANA_TOKEN:-}" ] || {
  echo "ERROR: QODANA_TOKEN required (release qodana-go linter; no Community Go edition exists)." >&2
  echo "  Add QODANA_TOKEN=<token> to ai-specs.env, or write it to ~/.qodana/token" >&2
  echo "  or .qodana/token (both git-ignored)." >&2
  exit 1
}

MODE=(--within-docker=false)
[ "${QODANA_DOCKER:-0}" = "1" ] && MODE=()

# --- baseline ---------------------------------------------------------------
if [ ! -s "$BASELINE" ]; then
  merge_base="$(git merge-base "$BASE_BRANCH" HEAD)" \
    || { echo "ERROR: cannot resolve merge-base with $BASE_BRANCH." >&2; exit 1; }
  tmp_clone=".qodana/baseline-clone"
  rm -rf "$tmp_clone"
  echo "Generating baseline from merge-base $merge_base (first run may download the linter)..."
  git clone -q --no-hardlinks "$ROOT" "$tmp_clone"
  git -C "$tmp_clone" checkout -q "$merge_base"
  if "$QODANA" scan -l "$LINTER" "${MODE[@]}" -i "$tmp_clone" -o "$tmp_clone/.qodana/results" --save-report=false; then
    sarif="$(ls "$tmp_clone"/.qodana/results/qodana.sarif* 2>/dev/null | head -1 || true)"
    [ -n "$sarif" ] && { cp "$sarif" "$BASELINE"; echo "Baseline saved: $BASELINE"; }
  fi
  rm -rf "$tmp_clone"
  [ -s "$BASELINE" ] || { echo "ERROR: baseline scan failed; $BASELINE not created." >&2; exit 1; }
fi

# --- PR-mode scan (only new problems) ---------------------------------------
echo "Scanning worktree in PR mode (baseline: $BASELINE)..."
"$QODANA" scan -l "$LINTER" "${MODE[@]}" -i "$ROOT" -o "$RESULTS" -b "$ROOT/$BASELINE" --save-report=false

sarif="$(ls "$RESULTS"/qodana.sarif* 2>/dev/null | head -1 || true)"
[ -n "$sarif" ] || { echo "ERROR: no SARIF produced under $RESULTS." >&2; exit 1; }

# --- report new issues -------------------------------------------------------
python3 "$(dirname "${BASH_SOURCE[0]}")/sarif_new_issues.py" "$sarif"
