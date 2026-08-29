#!/usr/bin/env bash
# worktree-gate.sh — thin launcher for the autocontained Go worktree gate.
#
# Enforces the worktree-flow discipline: "exploration ends at the first write;
# create a dedicated worktree before writing." The actual gate logic lives in
# the Go binary (the only implementation).
#
# Dual-input contract (one script, every harness):
#   PATH mode stdin = JSON { "event", "tool_name",
#     "tool_input": {file_path|notebook_path}, "cwd" }
#   SHELL mode stdin = JSON with tool_input.command (or script/cmd) OR Cursor
#     native top-level { "command", "cwd", … }
#   exit 0 → allow.   exit 2 → block (stderr is surfaced to the agent).
# Fail-open: any resolution failure warns once and exits 0, so a broken
# installation can never wedge every edit. Override protected branches via
# WORKTREE_GATE_PROTECTED. gate_mode off disables both path and shell gating.
# gate_scope/gate_impl are stamped by sync and may be overridden per
# invocation (scope via WORKTREE_GATE_SCOPE; impl has no env override).
#
# Resolution order (first hit wins):
#   1. $WORKTREE_GATE_BIN if executable — the debugging and pinning escape
#      hatch.
#   2. Project-local <recipe_root>/bin/worktree-gate — the optional air-gapped
#      pin, where <recipe_root> is derived from THIS launcher's physical
#      BASH_SOURCE[0] location (hooks/../), never from $PWD.
#   3. ${AI_SPECS_HOME:-$HOME/.ai-specs}/cache/bin/worktree-gate/
#      <stamped_version>/<os>-<arch>/worktree-gate — the version-keyed cache
#      populated by ai-specs sync (lib/_internal/gate_binary.py).
#   4. Nothing usable → one line to stderr naming the missing binary and the
#      `ai-specs sync` / `ai-specs sync --refresh-gates` / `ai-specs doctor`
#      remedy, then exit 0.
#
# bash 3.2 only by contract (no mapfile, no associative arrays, no ${v,,}):
# macos ships bash 3.2 and every harness spawns this script directly.

stamped_gate_mode="__WORKTREE_GATE_MODE__"
stamped_gate_scope="__WORKTREE_GATE_SCOPE__"
stamped_repo_topology="__WORKTREE_REPO_TOPOLOGY__"
stamped_gate_impl="__WORKTREE_GATE_IMPL__"
stamped_gate_version="__WORKTREE_GATE_VERSION__"
protected="${WORKTREE_GATE_PROTECTED:-main development}"

# Platform detection: empty target means "no binary for this host".
_goos=""
_goarch=""
case "$(uname -s)" in
  Darwin) _goos="darwin" ;;
  Linux) _goos="linux" ;;
esac
case "$(uname -m)" in
  arm64|aarch64) _goarch="arm64" ;;
  x86_64|amd64) _goarch="amd64" ;;
esac

# Resolve gate mode: env override beats stamped sync value; invalid values
# warn and fall back (mirrors the legacy reference contract).
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

_resolve_gate_scope() {
  local override="${WORKTREE_GATE_SCOPE:-}"
  if [ -n "$override" ]; then
    case "$override" in
      auto|superrepo|subrepo) echo "$override"; return ;;
      *) echo "worktree-gate: invalid WORKTREE_GATE_SCOPE='$override'; falling back to stamped scope." >&2 ;;
    esac
  fi
  case "$stamped_gate_scope" in
    auto|superrepo|subrepo) echo "$stamped_gate_scope" ;;
    *) echo "worktree-gate: missing or invalid stamped gate_scope='$stamped_gate_scope'; falling back to auto." >&2; echo auto ;;
  esac
}

# off → disable the gate entirely, before scope/topology evaluation.
[ "$gate_mode" = off ] && exit 0
gate_scope="$(_resolve_gate_scope)"

# Derive the physical installation root from BASH_SOURCE[0]. Prints the recipe
# root (launcher_dir/..) or nothing when the reference cannot be resolved.
# $PWD participates exactly once: anchoring a relative BASH_SOURCE reference.
# It is never used to locate project-local assets (design §4).
_launcher_root() {
  local src="${BASH_SOURCE[0]:-}"
  [ -n "$src" ] || return 1
  if [ ! -e "$src" ]; then
    # Relative reference: anchor to the invocation process cwd once.
    case "$src" in
      /*) return 1 ;;
      *) src="$PWD/$src" ;;
    esac
    [ -e "$src" ] || return 1
  fi
  # Follow the final launcher symlink (and any chain) to a physical path.
  local current="$src"
  local n=0
  while [ -L "$current" ]; do
    local target
    target="$(readlink "$current" 2>/dev/null)" || break
    [ -n "$target" ] || break
    case "$target" in
      /*) current="$target" ;;
      *) current="$(dirname "$current")/$target" ;;
    esac
    n=$((n+1))
    if [ "$n" -gt 40 ]; then return 1; fi
  done
  local dir
  dir="$(cd "$(dirname "$current")" 2>/dev/null && pwd -P)" || return 1
  local root
  root="$(cd "$dir/.." 2>/dev/null && pwd -P)" || return 1
  printf '%s\n' "$root"
  return 0
}

# Resolve an implementation. Prints the command to exec, or nothing when the
# legacy path applies (the legacy file carries its own stamped values).
_resolve_binary() {
  local bin=""
  if [ -n "${WORKTREE_GATE_BIN:-}" ]; then
    if [ -x "$WORKTREE_GATE_BIN" ]; then
      echo "$WORKTREE_GATE_BIN"
      return 0
    fi
    echo "worktree-gate: WORKTREE_GATE_BIN='$WORKTREE_GATE_BIN' is not executable; ignoring." >&2
  fi
  # Project-local pin under the derived physical installation root. The
  # launcher's own BASH_SOURCE[0] location decides — never $PWD.
  local recipe_root=""
  if recipe_root="$(_launcher_root)"; then
    bin="$recipe_root/bin/worktree-gate"
    if [ -x "$bin" ]; then
      echo "$bin"
      return 0
    fi
  fi
  if [ -n "$_goos" ] && [ -n "$_goarch" ]; then
    local home="${AI_SPECS_HOME:-$HOME/.ai-specs}"
    bin="$home/cache/bin/worktree-gate/$stamped_gate_version/$_goos-$_goarch/worktree-gate"
    if [ -x "$bin" ]; then
      echo "$bin"
      return 0
    fi
  fi
  return 1
}

bin="$(_resolve_binary)"
if [ -n "$bin" ]; then
  # WORKTREE_GATE_VERIFY=1 opts into a per-invocation self-test (paranoid or
  # forensic use). Never on the hot path by default.
  if [ "${WORKTREE_GATE_VERIFY:-0}" = "1" ]; then
    if ! "$bin" --selftest >/dev/null 2>&1; then
      echo "worktree-gate: cached binary failed --selftest: $bin; see 'ai-specs doctor'." >&2
      exit 0
    fi
  fi
  exec "$bin" --gate-mode "$gate_mode" --gate-scope "$gate_scope" --repo-topology "$stamped_repo_topology" --protected "$protected"
fi

echo "worktree-gate: no usable gate binary resolved (gate_impl='$stamped_gate_impl'); gate is not enforcing. Run 'ai-specs sync', 'ai-specs sync --refresh-gates', or 'ai-specs doctor'." >&2
exit 0
