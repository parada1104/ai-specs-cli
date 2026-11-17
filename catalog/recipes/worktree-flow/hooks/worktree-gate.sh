#!/usr/bin/env bash
# worktree-gate.sh — installed-CLI launcher for the Go worktree gate.
#
# PATH mode stdin is the harness event JSON; exit 0 allows and exit 2 blocks.
# Resolution failure fails open with one warning so a broken installation
# cannot wedge every edit. The CLI owns gate mode resolution.
# bash 3.2 only by contract (macOS ships bash 3.2).

stamped_gate_mode="__WORKTREE_GATE_MODE__"
stamped_gate_scope="__WORKTREE_GATE_SCOPE__"
stamped_repo_topology="__WORKTREE_REPO_TOPOLOGY__"
protected="${WORKTREE_GATE_PROTECTED:-main development}"

# Forward the raw stamped gate mode; Go owns mode resolution. Scope resolution
# stays here for transport; the CLI re-resolves it idempotently.
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

gate_scope="$(_resolve_gate_scope)"
if [ -n "${WORKTREE_GATE_BIN:-}" ]; then
  if [ -x "$WORKTREE_GATE_BIN" ]; then
    # WORKTREE_GATE_VERIFY=1 applies to the explicit override too; a failing
    # override is ignored (like a non-executable one) so enforcement continues
    # with the installed CLI instead of silently allowing the write.
    if [ "${WORKTREE_GATE_VERIFY:-0}" = "1" ] && ! "$WORKTREE_GATE_BIN" --selftest >/dev/null 2>&1; then
      echo "worktree-gate: WORKTREE_GATE_BIN failed --selftest: $WORKTREE_GATE_BIN; ignoring." >&2
    else
      exec "$WORKTREE_GATE_BIN" --gate-mode "$stamped_gate_mode" --gate-scope "$gate_scope" --repo-topology "$stamped_repo_topology" --protected "$protected"
    fi
  else
    echo "worktree-gate: WORKTREE_GATE_BIN='$WORKTREE_GATE_BIN' is not executable; ignoring." >&2
  fi
fi
cli_bin="${AI_SPECS_HOME:-$HOME/.ai-specs}/dist/ai-specs"
if [ -x "$cli_bin" ]; then
  if ! "$cli_bin" worktree-gate --version >/dev/null 2>&1; then
    echo "worktree-gate: installed ai-specs CLI is too old; gate is not enforcing. Run 'ai-specs upgrade'." >&2
    exit 0
  fi
  # WORKTREE_GATE_VERIFY=1 opts into the same self-test via the CLI verb.
  if [ "${WORKTREE_GATE_VERIFY:-0}" = "1" ]; then
    if ! "$cli_bin" worktree-gate --selftest >/dev/null 2>&1; then
      echo "worktree-gate: installed CLI failed worktree-gate --selftest: $cli_bin; see 'ai-specs doctor'." >&2
      exit 0
    fi
  fi
  exec "$cli_bin" worktree-gate --gate-mode "$stamped_gate_mode" --gate-scope "$gate_scope" --repo-topology "$stamped_repo_topology" --protected "$protected"
fi

echo "worktree-gate: no usable gate binary resolved (gate_impl='__WORKTREE_GATE_IMPL__'); gate is not enforcing. Run 'ai-specs sync', 'ai-specs sync --refresh-gates', or 'ai-specs doctor'." >&2
exit 0
