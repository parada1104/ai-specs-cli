# Archive Report: worktree-gate-bash-coverage

**Archived**: 2026-07-31
**Branch**: feat/worktree-gate-bash-coverage
**Verify verdict**: ready_for_archive (round 1 of 3 max)

## Summary

Closed a real, exploited gap in the `worktree-flow` recipe's `worktree-gate.sh`
pre-tool-use guard: it only intercepted `Edit|Write|MultiEdit|NotebookEdit`
tool calls, so any write via `bash` (heredoc, `python3 -c`, `cat >`, `tee`,
`sed -i`) completely bypassed the "no writes on protected branches in the main
worktree" guarantee. This was discovered live in-session: a subagent's
structured `write` tool call failed for an unrelated reason and it fell back
to a `python3` heredoc to write the file directly, evading the gate entirely.

## Canonical spec changes

Merged 5 ADDED requirements into `openspec/specs/worktree-flow/spec.md`
(16 scenarios total):

1. **Shell Command Write-Bypass Detection** — dual-input contract (structured
   path + shell command string), best-effort write-pattern heuristics
   (redirects, tee, sed -i/perl -i, cp/mv, interpreter write calls), fail-open
   on ambiguity.
2. **Dual Hook Registration for Shell Matchers** — second `[[provides.hooks]]`
   entry (`Bash|Shell|Execute|Terminal` matcher) sharing the same script;
   genuinely separate Cursor `beforeShellExecution` registration.
3. **Ask-mode and message parity for shell blocks** — same
   `WORKTREE_GATE_MODE=off` bypass hint and no new bypass surface beyond
   existing `gate_mode`.
4. **Anti-Fallback Skill and Brief Guidance** — SKILL.md + brief
   `workflow_rules` explicitly forbid retrying a blocked/errored write via
   bash.
5. **Honest per-harness shell-gate coverage documentation** — docs describe
   this as best-effort and uneven by harness, not a uniform sandbox.

Pre-existing requirements (merge-detection heuristics, dirty-worktree skip,
bounded candidate resolution, pre-delegation brief check) are untouched.

## Implementation notes

A critical bash quoting bug was found and fixed mid-apply: the embedded Python
heuristic parser was originally `python3 -c '...'` with literal single quotes
inside Python regex source (`["\\']`) — bash cannot escape a `'` inside a
single-quoted string, so the script had a syntax error and crashed on every
invocation. Fixed by switching to `python3 - "$input" <<'PYEOF' ... PYEOF`
(quote-delimited heredoc, JSON passed via `argv[1]` since the heredoc itself
occupies stdin).

`lib/_internal/hooks-render.py` required **no changes** — confirmed
empirically (not just per the design's claim): `recipe-materialize.py`'s
`runtime_hooks` loop is generic over any number of `[[provides.hooks]]`
entries, and Cursor's `_matcher_targets_file_writes` set-intersection against
`Bash|Shell|Execute|Terminal` is empty, so the new shell hook renders as a
genuinely separate `beforeShellExecution` registration without any renderer
work.

## Verification composition (documented deviation)

Planned: independent `sdd-verify` + 2 blind judges. In practice, `sdd-verify`
and `jd-judge-b` both failed on model rate limits (5h and monthly-11-day
resets) — an infrastructure/quota issue, not a code finding. `jd-judge-a`
completed a full independent review (APPROVE, 2 low-severity non-blocking
findings, including live command-injection testing). The orchestrator
directly performed the specific checks originally assigned to the two
unavailable reviewers (regression diffing, TOML validity, bash 3.2
compatibility, message differentiation, scope discipline) rather than wait 11
days for a monthly quota reset. Full detail in `verify-report.md` in this
archive folder.

## Files changed

- `catalog/recipes/worktree-flow/hooks/worktree-gate.sh` (dual-input contract + quoting fix)
- `catalog/recipes/worktree-flow/recipe.toml` (second hook entry, version 1.2.4 → 1.3.0, anti-fallback brief rule)
- `catalog/recipes/worktree-flow/skills/worktree-flow/SKILL.md` (anti-fallback rule)
- `catalog/recipes/worktree-flow/README.md` (shell-write coverage note with residual gaps)
- `docs/runtime-hooks.md` (per-harness shell-coverage matrix)
- `tests/test_worktree_gate_hook.py` (extended to 42 tests)
- `openspec/specs/worktree-flow/spec.md` (canonical, +5 requirements)

## Tests

`./tests/validate.sh`: 1123 tests OK (final run pending post-archive-commit
confirmation below).
