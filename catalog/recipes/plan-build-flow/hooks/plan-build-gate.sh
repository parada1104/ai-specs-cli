#!/usr/bin/env bash
# plan-build-gate.sh — pre-tool-use guard distributed by the plan-build-flow recipe.
#
# Enforces plan-before-build for production paths. Planning artifacts live under
# openspec/changes/<slug>/ and are always writable. In an initialized submodule
# worktree, the containing superproject is the canonical planning root; topology
# discovery is read-only and fail-safe.
#
# stdin: normalized JSON {event, tool_name, tool_input, cwd}
# exit 0: allow; exit 2: block. Malformed or unrelated events fail open.
# PLAN_BUILD_GATE_PATHS is scope only (default: src lib catalog).

prod_dirs="${PLAN_BUILD_GATE_PATHS:-src lib catalog}"
[ -n "${prod_dirs// /}" ] || prod_dirs="src lib catalog"

input="$(cat)"

# Normalize the target once. realpath is intentionally non-strict: destination
# components that do not exist yet remain in the canonical path, while existing
# symlink ancestors and final targets are resolved before any boundary decision.
parsed="$(printf '%s' "$input" | python3 -c '
import json
import os
import sys

try:
    event = json.load(sys.stdin)
    tool_input = event.get("tool_input", {}) or {}
    file_path = tool_input.get("file_path") or tool_input.get("notebook_path") or ""
    if not isinstance(file_path, str) or not file_path:
        sys.exit(0)
    cwd = event.get("cwd") or os.getcwd()
    if not isinstance(cwd, str):
        sys.exit(0)
    base = cwd if os.path.isabs(cwd) else os.path.abspath(cwd)
    target = os.path.realpath(file_path if os.path.isabs(file_path) else os.path.join(base, file_path))
    probe = os.path.dirname(target)
    while probe and not os.path.isdir(probe) and probe != os.path.dirname(probe):
        probe = os.path.dirname(probe)
    if not os.path.isdir(probe):
        sys.exit(0)
    print((event.get("tool_name", "") or "") + "\t" + target + "\t" + os.path.realpath(probe))
except Exception:
    sys.exit(0)
' 2>/dev/null)" || exit 0

tool_name="${parsed%%$'\t'*}"
rest="${parsed#*$'\t'}"
file_path="${rest%%$'\t'*}"
probe_dir="${rest#*$'\t'}"
[ -n "$file_path" ] && [ -n "$probe_dir" ] || exit 0
abs="$file_path"

# The target repository is authoritative for ordinary standalone behavior.
git -C "$probe_dir" rev-parse --is-inside-work-tree >/dev/null 2>&1 || exit 0
repo_root="$(git -C "$probe_dir" rev-parse --show-toplevel 2>/dev/null)" || exit 0
[ -n "$repo_root" ] || exit 0

# Component-aware canonical containment; the trailing slash rejects lookalikes.
is_under() {
  [ "$1" = "$2" ] && return 0
  case "$2/" in
    "$1"/*) return 0 ;;
  esac
  return 1
}

is_under "$repo_root" "$abs" || exit 0
rel="${abs#"$repo_root"/}"
[ "$rel" = "$abs" ] && rel="."

# Gitignored agent configuration is machine setup, never a production change.
case "$rel" in
  .claude/settings*.json|*/.claude/settings*.json|.claude/hooks/*|*/.claude/hooks/*) exit 0 ;;
esac

# Artifact writes are unconditional at the nearest repository root.
is_under "$repo_root/openspec/changes" "$abs" && exit 0

first="${rel%%/*}"
is_prod=0
for p in $prod_dirs; do
  [ "$first" = "$p" ] && is_prod=1 && break
done
[ "$is_prod" -eq 1 ] || exit 0

has_active_plan() {
  local f
  shopt -s nullglob
  for f in "$1"/openspec/changes/*/tasks.md; do
    return 0
  done
  return 1
}

# Derive a central root only from a proven initialized submodule relationship.
# The common git directory is the primary signal because linked submodule
# worktrees report an empty --show-superproject-working-tree.
central_root=""
central_sub=""
resolve_central_root() {
  local gcd pre name cand rel_sub registered sub_dir status sup
  gcd="$(git -C "$probe_dir" rev-parse --git-common-dir 2>/dev/null)" || return 1
  gcd="$(cd "$probe_dir" 2>/dev/null && cd "$gcd" 2>/dev/null && pwd -P)" || return 1

  case "$gcd" in
    */.git/modules/*)
      # Use the final /modules/ marker: superproject paths may contain that
      # component, while an inner submodule has an earlier modules prefix.
      pre="${gcd%/modules/*}"
      name="${gcd##*/modules/}"
      [ -n "$name" ] || return 1
      case "$pre" in
        */.git/modules/*) return 1 ;;
      esac
      [ "${pre##*/}" = ".git" ] || return 1
      cand="${pre%/.git}"
      ;;
    *)
      # Legacy non-absorbed layouts may provide this corroborating fact, but it
      # is never the sole signal for the modern linked-worktree path.
      sup="$(git -C "$probe_dir" rev-parse --show-superproject-working-tree 2>/dev/null)" || return 1
      [ -n "$sup" ] || return 1
      cand="$(cd "$sup" 2>/dev/null && pwd -P)" || return 1
      is_under "$cand" "$repo_root" || return 1
      rel_sub="${repo_root#"$cand"/}"
      name=""
      ;;
  esac

  [ "$cand" != "$repo_root" ] || return 1
  [ -d "$cand/.git" ] || return 1
  [ -f "$cand/.gitmodules" ] || return 1

  if [ -n "$name" ]; then
    registered="$(git -C "$cand" config -f "$cand/.gitmodules" --get "submodule.$name.path" 2>/dev/null)" || return 1
    [ -n "$registered" ] || return 1
    rel_sub="$registered"
  else
    [ -n "$rel_sub" ] || return 1
  fi
  sub_dir="$(cd "$cand/$rel_sub" 2>/dev/null && pwd -P)" || return 1
  is_under "$cand" "$sub_dir" || return 1
  [ -e "$sub_dir/.git" ] || return 1
  status="$(git -C "$cand" submodule status -- "$rel_sub" 2>/dev/null)" || return 1
  case "$status" in
    "") return 1 ;;
    -*) return 1 ;;
  esac
  central_root="$cand"
  central_sub="$rel_sub"
  return 0
}

# Preserve the nearest-root gate and its existing diagnostic for standalone
# repositories and unresolved topology. Central lookup is intentionally lazy.
if has_active_plan "$repo_root"; then
  exit 0
fi

if resolve_central_root; then
  if has_active_plan "$central_root"; then
    exit 0
  fi
  echo "plan-build-gate: refusing to ${tool_name:-edit} '$rel' — no active change folder found under central planning root '$central_root/openspec/changes/' for submodule '$central_sub'. Classify the change and write the central plan first; writes under that planning tree are allowed." >&2
  exit 2
fi

echo "plan-build-gate: refusing to ${tool_name:-edit} '$rel' — no active change folder (openspec/changes/<slug>/tasks.md) found. Classify the change and write the plan first, then implement. Writing planning artifacts under openspec/changes/ is never blocked." >&2
exit 2
