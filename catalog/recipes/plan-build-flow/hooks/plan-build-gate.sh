#!/usr/bin/env bash
# plan-build-gate.sh — pre-tool-use guard distributed by the plan-build-flow recipe.
#
# Enforces the plan-build artifact precondition: "classify and write the plan
# before touching production code." Blocks Edit/Write/MultiEdit/NotebookEdit
# calls that target a PRODUCTION directory (default top-level src, lib, catalog)
# while NO active change folder exists (no openspec/changes/<slug>/tasks.md
# outside archive/). Edits to the plan itself (openspec/changes/**), to
# non-production paths (tests, docs), and to gitignored agent config are always
# allowed.
#
# Normalized contract (one script, every harness):
#   stdin  = JSON { "event", "tool_name", "tool_input": {file_path|notebook_path}, "cwd" }
#   exit 0 → allow.   exit 2 → block (stderr is surfaced to the agent).
# Fail-open: any parse/lookup error allows the edit (a buggy guard must never
# wedge all editing).
#
# Config (env-only; no stamped placeholder):
#   PLAN_BUILD_GATE_MODE   always (default) | ask | off
#   PLAN_BUILD_GATE_PATHS  space-separated production top-level dirs (default: "src lib catalog")

gate_mode="${PLAN_BUILD_GATE_MODE:-always}"
case "$gate_mode" in
  always|ask|off) ;;
  *)
    echo "plan-build-gate: ignoring invalid PLAN_BUILD_GATE_MODE='${gate_mode}'; falling back to always." >&2
    gate_mode=always ;;
esac

# off → disable the gate entirely.
[ "$gate_mode" = off ] && exit 0

prod_dirs="${PLAN_BUILD_GATE_PATHS:-src lib catalog}"

input="$(cat)"

# Extract tool name, target path, and cwd from the normalized event.
parsed="$(printf '%s' "$input" | python3 -c '
import json, sys
try:
    d = json.load(sys.stdin)
except Exception:
    sys.exit(0)
ti = d.get("tool_input", {}) or {}
fp = ti.get("file_path") or ti.get("notebook_path") or ""
print((d.get("tool_name", "") or "") + "\t" + fp + "\t" + (d.get("cwd", "") or ""))
' 2>/dev/null)" || exit 0

tool_name="${parsed%%$'\t'*}"
rest="${parsed#*$'\t'}"
file_path="${rest%%$'\t'*}"
cwd="${rest#*$'\t'}"
[ -n "$file_path" ] || exit 0

# Resolve to an absolute path (relative file_path is joined with the event cwd).
case "$file_path" in
  /*) abs="$file_path" ;;
  *)  abs="${cwd:-$PWD}/$file_path" ;;
esac

# Walk up to the nearest existing directory (file may not exist yet on Write).
dir="$(dirname "$abs")"
while [ ! -d "$dir" ] && [ "$dir" != "/" ] && [ "$dir" != "." ]; do
  dir="$(dirname "$dir")"
done
[ -d "$dir" ] || exit 0

git -C "$dir" rev-parse --is-inside-work-tree >/dev/null 2>&1 || exit 0
repo_root="$(git -C "$dir" rev-parse --show-toplevel 2>/dev/null)" || exit 0
[ -n "$repo_root" ] || exit 0

# Repo-relative path of the target. Resolve symlinks on both sides first so a
# symlinked prefix (e.g. macOS /tmp → /private/tmp) does not defeat the strip.
rel="$(python3 -c 'import os,sys; print(os.path.relpath(os.path.realpath(sys.argv[1]), os.path.realpath(sys.argv[2])))' "$abs" "$repo_root" 2>/dev/null)" || exit 0
[ -n "$rel" ] || exit 0
# Target outside the repo → not our concern; allow.
case "$rel" in ..|../*) exit 0 ;; esac

# Always allow gitignored agent config (machine setup, never committed).
case "$rel" in
  .claude/settings*.json|*/.claude/settings*.json|.claude/hooks/*|*/.claude/hooks/*) exit 0 ;;
esac

# Only gate edits under a production top-level directory.
first="${rel%%/*}"
is_prod=0
for p in $prod_dirs; do
  [ "$first" = "$p" ] && is_prod=1 && break
done
[ "$is_prod" -eq 1 ] || exit 0

# Production edit: allow only if an ACTIVE (non-archived) change folder exists.
# The glob openspec/changes/*/tasks.md matches direct children only, so archived
# changes under openspec/changes/archive/<slug>/tasks.md are naturally excluded.
shopt -s nullglob
active=0
for f in "$repo_root"/openspec/changes/*/tasks.md; do
  active=1
  break
done
[ "$active" -eq 1 ] && exit 0

echo "plan-build-gate: refusing to ${tool_name:-edit} '$rel' — no active change folder (openspec/changes/<slug>/tasks.md) found. Classify the change and write the plan first, then implement. Writing planning artifacts under openspec/changes/ is never blocked." >&2
if [ "$gate_mode" = ask ]; then
  echo "plan-build-gate: to bypass for this invocation, re-run with PLAN_BUILD_GATE_MODE=off" >&2
fi
exit 2
