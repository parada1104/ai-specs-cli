#!/usr/bin/env bash
# worktree-gate.sh — pre-tool-use guard distributed by the worktree-flow recipe.
#
# Enforces the worktree-flow discipline: "exploration ends at the first write;
# create a dedicated worktree before writing." Blocks Edit/Write/MultiEdit/
# NotebookEdit calls that target the canonical MAIN worktree while it is on a
# protected branch (default: main, development). Edits inside a linked worktree
# (under .worktrees/) are always allowed.
#
# Normalized contract (one script, every harness):
#   stdin  = JSON { "event", "tool_name", "tool_input": {file_path|notebook_path}, "cwd" }
#   exit 0 → allow.   exit 2 → block (stderr is surfaced to the agent).
# Fail-open: any parse/lookup error allows the edit (a buggy guard must never
# wedge all editing). Override protected branches via WORKTREE_GATE_PROTECTED.

stamped_gate_mode="__WORKTREE_GATE_MODE__"
protected="${WORKTREE_GATE_PROTECTED:-main development}"

# Resolve gate mode: env override beats stamped sync value; invalid values warn and fall back.
_resolve_gate_mode() {
  local candidate="${WORKTREE_GATE_MODE:-$stamped_gate_mode}"
  case "$candidate" in always|ask|off) echo "$candidate" ; return ;;
  esac
  if [ -n "${WORKTREE_GATE_MODE:-}" ]; then
    echo "worktree-gate: ignoring invalid WORKTREE_GATE_MODE='${WORKTREE_GATE_MODE}'; falling back to stamped mode." >&2
  elif [ "$stamped_gate_mode" != always ] && [ "$stamped_gate_mode" != ask ] && [ "$stamped_gate_mode" != off ]; then
    echo "worktree-gate: invalid stamped gate_mode='${stamped_gate_mode}'; falling back to always." >&2
  fi
  case "$stamped_gate_mode" in always|ask|off) echo "$stamped_gate_mode" ;;
  *) echo always ;;
  esac
}
gate_mode="$(_resolve_gate_mode)"

# off → disable the gate entirely.
[ "$gate_mode" = off ] && exit 0

input="$(cat)"

# Extract tool name + target path from the normalized event. python3 is a
# project prerequisite.
parsed="$(printf '%s' "$input" | python3 -c '
import json, sys
try:
    d = json.load(sys.stdin)
except Exception:
    sys.exit(0)
ti = d.get("tool_input", {}) or {}
fp = ti.get("file_path") or ti.get("notebook_path") or ""
print((d.get("tool_name", "") or "") + "\t" + fp)
' 2>/dev/null)" || exit 0

tool_name="${parsed%%$'\t'*}"
file_path="${parsed#*$'\t'}"
[ -n "$file_path" ] || exit 0

# Walk up to the nearest existing directory (file may not exist yet on Write).
dir="$(dirname "$file_path")"
while [ ! -d "$dir" ] && [ "$dir" != "/" ] && [ "$dir" != "." ]; do
  dir="$(dirname "$dir")"
done
[ -d "$dir" ] || exit 0

git -C "$dir" rev-parse --is-inside-work-tree >/dev/null 2>&1 || exit 0

git_dir="$(git -C "$dir" rev-parse --absolute-git-dir 2>/dev/null)" || exit 0
common_dir="$(git -C "$dir" rev-parse --path-format=absolute --git-common-dir 2>/dev/null)" || exit 0

# Linked worktree (git_dir != common_dir) → allowed.
[ "$git_dir" != "$common_dir" ] && exit 0

# Main worktree: gate on the current branch.
branch="$(git -C "$dir" symbolic-ref --short HEAD 2>/dev/null)" || exit 0
for b in $protected; do
  if [ "$branch" = "$b" ]; then
    # Allow local, gitignored agent config (machine setup, never committed).
    case "$file_path" in
      */.claude/settings*.json|.claude/settings*.json|*/.claude/hooks/*) exit 0 ;;
    esac
    echo "worktree-gate: refusing to ${tool_name:-edit} '$file_path' on protected branch '$branch' in the main worktree. Create a dedicated worktree first (e.g. /worktree-new) and edit there — exploration ends at the first write." >&2
    if [ "$gate_mode" = ask ]; then
      echo "worktree-gate: to bypass for this invocation, re-run with WORKTREE_GATE_MODE=off" >&2
    fi
    exit 2
  fi
done

exit 0
