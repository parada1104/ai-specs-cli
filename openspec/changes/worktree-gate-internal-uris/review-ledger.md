# Review Ledger: worktree-gate-internal-uris

- Change: `worktree-gate-internal-uris` — branch `change/worktree-gate-internal-uris`
- Reviewers: `review-risk` + `review-reliability` (fresh-context), followed by `jd-fix-agent` fix round and a final `review-risk` pass
- Policy: confirmed defects → fix; suggestions → triage; all findings resolved before PR
- Status legend: open | refuted | fixed | verified | wont-fix | info

## Findings (current status)

| id | reviewer | lens | location | severity | triage | status |
|----|----------|------|----------|----------|--------|--------|
| G-001 | GateReview | reliability | `catalog/recipes/worktree-flow/hooks/worktree-gate.sh` (`resolve_and_check` `local` declaration) | WARNING | confirmed | verified |
| G-002 | GateReview | reliability | `tests/test_worktree_gate_hook.py` cwd/URI regression coverage | SUGGESTION | confirmed | fixed |
| G-003 | GateRiskReview | risk | URI allowlist bypass scope (PATH-only, traversal/absolute masking) | WARNING | confirmed | verified |
| G-004 | GateCriticalFixes | risk | `rest` variable scoping in `resolve_and_check` | WARNING | confirmed | fixed |
| G-005 | GateFinalReview | risk | event-cwd precedence and fallback contract | WARNING | confirmed | verified |

## Resolution notes

- G-001/G-004: `rest` was declared `local` alongside `abs`/`decision` in
  `resolve_and_check`, closing the shell variable-scope leak flagged by review.
- G-002: cwd fallback and URI masking regressions were added to
  `tests/test_worktree_gate_hook.py` (192 lines added, covering PATH-mode
  allowlist, SHELL-mode literal targets, traversal/absolute masking, and event
  vs process cwd precedence).
- G-003: allowlist bypass is scoped to PATH mode; candidates carrying `../`
  traversal or an absolute path after the scheme are classified normally.
  Unknown schemes (`https://`, `file://`, `custom://`) keep normal gating.
- G-005: event `cwd` is used only when non-empty, absolute, and an existing
  directory; otherwise the hook process `$PWD` is the fallback. Fail-open
  preserved.

## Verification

- Focused: `tests/test_worktree_gate_hook.py` — 78 passed.
- Full suite: 1398 tests OK (previous baseline), no regressions.
- All findings resolved; no open findings remain.
