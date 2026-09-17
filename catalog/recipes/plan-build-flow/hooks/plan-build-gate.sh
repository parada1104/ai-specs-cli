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
# __TRACKER_LIB_INTERNAL__ is stamped at sync with the CLI's lib/_internal, where
# ledger_bridge.py ships; an empty or unstamped value skips the witness lookup
# (fail open), exactly like a missing or unverified binary.

stamped_lib_internal="__TRACKER_LIB_INTERNAL__"
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

# --- Ledger work-start checkpoint (acquisition + JSON bridge only) ------------
# The five ledger checkpoints are graded by the verified Go `--ledger` mode on
# the shared worktree-gate binary (one predicate, one trust root). This host
# only resolves the A9 mode, acquires a verified binary, and maps the JSON
# verdict. A missing/unverified binary, a parse error, or an IO failure fails
# open (exit 0). `openspec/**` never reaches this point, so it is never blocked.

_ledger_bridge() {
  # The evidence bridge lives beside the CLI's other internals. An empty or
  # unstamped value means the bridge is unavailable, so the witness recipe id is
  # unresolved and the lookup keeps the warn-first default (fail open).
  case "$stamped_lib_internal" in
    ""|__*) return 1 ;;
  esac
  [ -f "$stamped_lib_internal/ledger_bridge.py" ] || return 1
  printf '%s\n' "$stamped_lib_internal"
}

_ledger_recipe_id() {
  # The bound recipe id from the durable witness (via the bridge), or nothing
  # when the bridge is unstamped. Reading the witness is acquisition, not grading.
  local lib
  lib="$(_ledger_bridge)" || return 0
  python3 - "$lib" "$1" <<'PY' 2>/dev/null || true
import sys
from pathlib import Path
lib = sys.argv[1]
sys.path.insert(0, lib)
import ledger_bridge
print(ledger_bridge.recipe_id(Path(sys.argv[2])))
PY
}

_ledger_mode() {
  # $1 root, $2 legacy gate-mode hint, $3 witness recipe id (may be empty). The
  # config section is recipes.<recipe>; without a recipe id only the env override
  # and the stamped hint apply, which keeps the warn-first default.
  local root="$1"
  local gate_hint="${2:-}"
  local recipe="${3:-}"
  python3 - "$root" "${TRACKER_LEDGER_MODE:-}" "$gate_hint" "$recipe" <<'PY' 2>/dev/null
import sys, tomllib
from pathlib import Path
root, env_mode, gate_hint = sys.argv[1], sys.argv[2], sys.argv[3]
recipe = sys.argv[4] if len(sys.argv) > 4 else ""
ledger = gate = ""
if recipe:
    try:
        data = tomllib.loads((Path(root) / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8"))
        cfg = ((data.get("recipes") or {}).get(recipe) or {}).get("config") or {}
        ledger = cfg.get("ledger_mode") or ""
        gate = cfg.get("gate_mode") or ""
    except Exception:
        pass
if gate_hint in ("off", "warn", "always") and not gate:
    # The bound recipe's own gate_mode wins; the stamped legacy hint fills in when
    # that config section declares none (the pre-witness behavior).
    gate = gate_hint
if env_mode in ("always", "ask", "warn"):
    print(env_mode)
elif ledger in ("always", "ask", "warn"):
    print(ledger)
elif gate == "off":
    print("off")
elif gate == "always":
    print("always")
else:
    print("warn")
PY
}

_ledger_binary() {
  if [ -n "${WORKTREE_GATE_BIN:-}" ] && [ -x "$WORKTREE_GATE_BIN" ]; then
    printf '%s\n' "$WORKTREE_GATE_BIN"
    return 0
  fi
  local home="${AI_SPECS_HOME:-$HOME/.ai-specs}"
  local version=""
  if [ -f "$home/VERSION" ]; then version="$(tr -d '[:space:]' < "$home/VERSION")"; fi
  [ -n "$version" ] || version="dev"
  local goos goarch
  case "$(uname -s)" in Darwin) goos=darwin ;; Linux) goos=linux ;; *) return 1 ;; esac
  case "$(uname -m)" in arm64|aarch64) goarch=arm64 ;; x86_64|amd64) goarch=amd64 ;; *) return 1 ;; esac
  local candidate="$home/cache/bin/worktree-gate/$version/$goos-$goarch/worktree-gate"
  [ -x "$candidate" ] || return 1
  [ -f "$candidate.verified" ] || return 1
  printf '%s\n' "$candidate"
}

_ledger_field() {
  # stdin: verdict JSON. $1: top-level key.
  python3 -c '
import json, sys
try:
    data = json.load(sys.stdin)
except Exception:
    raise SystemExit(1)
value = data.get(sys.argv[1], "")
if isinstance(value, bool):
    print("1" if value else "0")
elif value is None:
    print("")
else:
    print(value)
' "$1" 2>/dev/null
}

_ledger_ask() {
  # $1 bin, $2 checkpoint, $3 mode, $4 root, $5 prefix; stdin: prompt JSON.
  local bin="$1" checkpoint="$2" mode="$3" root="$4" prefix="$5"
  python3 -c '
import json, sys
try:
    prompt = json.load(sys.stdin) or {}
except Exception:
    prompt = {}
ev = prompt.get("evidence") or {}
for side in ("local", "remote", "code", "git"):
    print("  %s: %s" % (side, ev.get(side) or "(unavailable)"))
print("  choices: " + ", ".join(prompt.get("choices") or []))
' >&2 2>/dev/null
  if ! { exec 3</dev/tty; } 2>/dev/null; then
    echo "${prefix}: ${checkpoint} needs a decision but no terminal is available; no opt-out was recorded; blocking." >&2
    return 2
  fi
  printf '%s: opt out of the %s checkpoint? [y/N] ' "$prefix" "$checkpoint" >&2
  local answer=""
  read -r answer <&3 || answer=""
  exec 3<&-
  case "$answer" in
    y|Y|yes|YES|Yes)
      if "$bin" --ledger --checkpoint "$checkpoint" --ledger-mode "$mode" --project-root "$root" \
          --decide "{\"checkpoint\":\"$checkpoint\",\"kind\":\"opt-out\",\"choice\":\"continue\"}" >/dev/null 2>&1; then
        echo "${prefix}: opt-out recorded for ${checkpoint}; proceeding." >&2
        return 0
      fi
      echo "${prefix}: failed to record the ${checkpoint} opt-out; blocking." >&2
      return 2
      ;;
  esac
  echo "${prefix}: no opt-out recorded for ${checkpoint}; blocking." >&2
  return 2
}

_ledger_grade() {
  # $1 checkpoint, $2 mode, $3 root, $4 prefix. Returns 0 allow / 2 block.
  local checkpoint="$1" mode="$2" root="$3" prefix="$4"
  local bin
  bin="$(_ledger_binary)" || return 0
  local out rc
  out="$("$bin" --ledger --checkpoint "$checkpoint" --ledger-mode "$mode" --project-root "$root" 2>/dev/null)"
  rc=$?
  [ -n "$out" ] || return 0
  local decision reason active
  decision="$(printf '%s' "$out" | _ledger_field decision)" || return 0
  [ -n "$decision" ] || return 0
  reason="$(printf '%s' "$out" | _ledger_field reason)"
  active="$(printf '%s' "$out" | _ledger_field active)"
  if [ "$rc" = 2 ] || [ "$decision" = block ]; then
    echo "${prefix}: blocked at ${checkpoint} — ${reason:-missing tracked item}" >&2
    return 2
  fi
  if [ "$decision" = ask ] && [ "$reason" = identity_unavailable ]; then
    # No identity means no durable key for a checkpoint-scoped answer (A2/A5), so
    # ask cannot collect a recordable decision here: report and proceed without
    # inferring one (the spec's non-blocking identity_unavailable rule).
    echo "${prefix}: ${checkpoint} cannot be keyed (identity_unavailable); reporting and proceeding without recording a decision." >&2
    return 0
  fi
  if [ "$decision" = ask ]; then
    printf '%s' "$out" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(json.dumps(d.get("prompt")))' 2>/dev/null \
      | _ledger_ask "$bin" "$checkpoint" "$mode" "$root" "$prefix"
    return $?
  fi
  if [ "$active" = 1 ] && [ "$decision" != allow ]; then
    echo "${prefix}: ${decision} at ${checkpoint} — ${reason:-no primary item}" >&2
  fi
  return 0
}

_ledger_work_start() {
  local mode
  mode="$(_ledger_mode "$repo_root" "" "$(_ledger_recipe_id "$repo_root")")"
  [ "$mode" = off ] && return 0
  _ledger_grade "work-start" "$mode" "$repo_root" "plan-build-gate"
}

# Work-start fires before the first non-read-only production write, whether or
# not a change folder exists (design checkpoint table). The artifact gate below
# is a separate, unchanged rule.
_ledger_work_start || exit 2

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
