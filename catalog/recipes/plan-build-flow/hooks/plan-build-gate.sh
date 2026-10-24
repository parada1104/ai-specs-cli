#!/usr/bin/env bash
# plan-build-gate.sh — pre-tool-use guard distributed by the plan-build-flow recipe.
#
# Enforces plan-before-build for production paths. Planning artifacts live under
# openspec/changes/<slug>/ and are always writable. In an initialized submodule
# worktree, the containing superproject is the canonical planning root; the
# topology proof is Go-owned (worktree-gate --resolve-central-root) and any
# unproven answer fails closed.
#
# stdin: normalized JSON {event, tool_name, tool_input, cwd}
# exit 0: allow; exit 2: block. Malformed or unrelated events fail open.
# PLAN_BUILD_GATE_PATHS is scope only (default: src lib catalog).
# The ledger mode is Go-owned: this hook invokes `--ledger` with no mode flag and
# Go resolves the effective mode from env, the witness-bound recipe config and
# the warn default (see ledger_mode.go / ledger_cmd.go).

prod_dirs="${PLAN_BUILD_GATE_PATHS:-src lib catalog}"
[ -n "${prod_dirs// /}" ] || prod_dirs="src lib catalog"

input="$(cat)"

# Normalize the target once. realpath is intentionally non-strict: destination
# components that do not exist yet remain in the canonical path, while existing
# symlink ancestors and final targets are resolved before any boundary decision.
# The native CLI (dist/ai-specs, installed by install.sh and upgrade) normalizes
# the target. A missing or non-executable binary, or an event with no string
# target, prints nothing: the hook exits 0 (fail open), as a python failure did.
cli_bin="${AI_SPECS_HOME:-$HOME/.ai-specs}/dist/ai-specs"
parsed=""
if [ -x "$cli_bin" ]; then
  parsed="$(printf '%s' "$input" | "$cli_bin" hook normalize-path 2>/dev/null)" || parsed=""
fi
[ -n "$parsed" ] || exit 0

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
# only acquires a verified binary and maps the JSON verdict; Go resolves the
# effective mode from env, the witness-bound recipe config and the warn default.
# A missing/unverified binary, a parse error, or an IO failure fails open
# (exit 0). `openspec/**` never reaches this point, so it is never blocked.

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
  # stdin: verdict JSON. $1: top-level key, read through `hook field`. Without
  # the CLI there is no verdict to read: empty output, the caller allows.
  [ -x "$cli_bin" ] || return 1
  "$cli_bin" hook field "$1" 2>/dev/null
}

_ledger_ask() {
  # $1 bin, $2 checkpoint, $3 root, $4 prefix; stdin: the full verdict JSON. The
  # display lines read `prompt.*` through `hook field`; without the CLI they are
  # skipped and the opt-out flow runs unchanged.
  local bin="$1" checkpoint="$2" root="$3" prefix="$4"
  local payload side value
  payload="$(cat)"
  if [ -x "$cli_bin" ]; then
    for side in local remote code git; do
      value="$(printf '%s' "$payload" | "$cli_bin" hook field "prompt.evidence.$side" 2>/dev/null)"
      printf '  %s: %s\n' "$side" "${value:-(unavailable)}" >&2
    done
    value="$(printf '%s' "$payload" | "$cli_bin" hook field prompt.choices 2>/dev/null)"
    printf '  choices: %s\n' "$value" >&2
  fi
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
      if "$bin" --ledger --checkpoint "$checkpoint" --project-root "$root" \
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
  # $1 checkpoint, $2 root, $3 prefix. Returns 0 allow / 2 block. No mode flag is
  # forwarded: Go resolves env, the witness-bound recipe config and the default.
  local checkpoint="$1" root="$2" prefix="$3"
  local bin
  bin="$(_ledger_binary)" || return 0
  local out rc
  out="$("$bin" --ledger --checkpoint "$checkpoint" --project-root "$root" 2>/dev/null)"
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
    printf '%s' "$out" | _ledger_ask "$bin" "$checkpoint" "$root" "$prefix"
    return $?
  fi
  if [ "$active" = 1 ] && [ "$decision" != allow ]; then
    echo "${prefix}: ${decision} at ${checkpoint} — ${reason:-no primary item}" >&2
  fi
  return 0
}

_ledger_work_start() {
  # `off` skipping is Go's short-circuit (runLedger returns before grading).
  _ledger_grade "work-start" "$repo_root" "plan-build-gate"
}

# Work-start fires before the first non-read-only production write, whether or
# not a change folder exists (design checkpoint table). The artifact gate below
# is a separate, unchanged rule.
_ledger_work_start || exit 2

has_active_plan() {
  local f
  shopt -s nullglob
  for f in "$1"/openspec/changes/*/tasks.md; do
    [ -n "$f" ] || continue
    return 0
  done
  return 1
}

# --- Central planning root (verified-Go bridge only) -------------------------
# The submodule topology proof is Go-owned: this host only acquires the verified
# binary, runs `--resolve-central-root` from the probe directory, and validates
# the returned JSON against this repository. A missing or unverified binary, a
# nonzero exit, a malformed or empty payload, or a central root that does not
# contain the nearest repository root leaves the topology unproven, so the
# nearest-root gate below fails closed. `submodule` is diagnostic only.
central_root=""
central_sub=""
central_root_proof() {
  local bin out cand sub
  bin="$(_ledger_binary)" || return 1
  # Go resolves upward from the current directory, so the query must run from
  # the probe directory (the target's nearest existing ancestor).
  out="$(cd "$probe_dir" 2>/dev/null && "$bin" --resolve-central-root 2>/dev/null)" || return 1
  [ -n "$out" ] || return 1
  [ -x "$cli_bin" ] || return 1
  cand="$(printf '%s' "$out" | "$cli_bin" hook field --string central_root 2>/dev/null)" || return 1
  sub="$(printf '%s' "$out" | "$cli_bin" hook field --string submodule 2>/dev/null)" || return 1
  case "$cand" in /*) ;; *) return 1 ;; esac
  case "$cand$sub" in *$'\t'*|*$'\r'*|*$'\n'*) return 1 ;; esac
  cand="$(cd "$cand" 2>/dev/null && pwd -P)" || return 1
  is_under "$cand" "$repo_root" || return 1
  [ "$cand" != "$repo_root" ] || return 1
  central_root="$cand"
  central_sub="$sub"
  return 0
}

# Preserve the nearest-root gate and its existing diagnostic for standalone
# repositories and unresolved topology. Central lookup is intentionally lazy.
if has_active_plan "$repo_root"; then
  exit 0
fi

if central_root_proof; then
  if has_active_plan "$central_root"; then
    exit 0
  fi
  echo "plan-build-gate: refusing to ${tool_name:-edit} '$rel' — no active change folder found under central planning root '$central_root/openspec/changes/' for submodule '$central_sub'. Classify the change and write the central plan first; writes under that planning tree are allowed." >&2
  exit 2
fi

echo "plan-build-gate: refusing to ${tool_name:-edit} '$rel' — no active change folder (openspec/changes/<slug>/tasks.md) found. Classify the change and write the plan first, then implement. Writing planning artifacts under openspec/changes/ is never blocked." >&2
exit 2
