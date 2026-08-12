#!/usr/bin/env bash
# worktree-gate.sh — thin launcher for the autocontained Go worktree gate.
#
# Enforces the worktree-flow discipline: "exploration ends at the first write;
# create a dedicated worktree before writing." The actual gate logic lives in
# the Go binary (implementation of record) or, as a rollback path, in the
# frozen Bash reference worktree-gate-legacy.sh.
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
# Resolution order (first hit wins, design §5):
#   1. $WORKTREE_GATE_BIN if executable — the debugging and pinning escape
#      hatch.
#   2. <project>/ai-specs/recipes/worktree-flow/bin/worktree-gate — an
#      optional project-local pin for air-gapped repos.
#   3. ${AI_SPECS_HOME:-$HOME/.ai-specs}/cache/bin/worktree-gate/
#      <stamped_version>/<os>-<arch>/worktree-gate — the version-keyed cache
#      populated by ai-specs sync (lib/_internal/gate_binary.py).
#   4. Legacy Bash implementation — only when stamped gate_impl is "bash", or
#      it is "auto" and no binary resolved.
#   5. Nothing usable → one line to stderr naming the missing path and the
#      `ai-specs doctor` remedy, then exit 0.
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
  bin="$PWD/ai-specs/recipes/worktree-flow/bin/worktree-gate"
  # The materialized hook runs from the project root in every harness, but be
  # defensive: also try a resolved project root via the parent of ai-specs/.
  if [ ! -x "$bin" ]; then
    local probe="$PWD/ai-specs"
    [ -d "$probe" ] && bin="$(cd "$probe/.." 2>/dev/null && pwd)/ai-specs/recipes/worktree-flow/bin/worktree-gate"
  fi
  if [ -x "$bin" ]; then
    echo "$bin"
    return 0
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

# No usable binary: fall back to the frozen Bash implementation when the
# stamped gate_impl permits it (bash explicitly, or auto with no binary).
case "$stamped_gate_impl" in
  bash|auto)
    legacy="$PWD/ai-specs/recipes/worktree-flow/hooks/worktree-gate-legacy.sh"
    if [ -f "$legacy" ]; then
      exec bash "$legacy"
    fi
    ;;
esac

echo "worktree-gate: no usable gate implementation found (gate_impl='$stamped_gate_impl'); gate is not enforcing. Run 'ai-specs sync' and 'ai-specs doctor'." >&2
exit 0
