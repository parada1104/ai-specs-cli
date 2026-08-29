# Verification Report: retire-decision-matrix

## Verdict

**PASS — no regressions introduced; Full evidence shape satisfied.**

Apply phases 1–3 are implemented and the focused doc-contract tests are green.
The repository validation suite passes in full on this worktree
(`./tests/validate.sh`, GNU bash 5.3.9): 1378 tests, 0 failures, 0 errors,
exit 0.

## Baseline comparison

The earlier verify pass (2026-08-09, change worktree at `7af4b22`) recorded
103 failures and 60 errors under `./tests/run.sh` on both the change worktree
and the pristine baseline. Those failures were **pre-existing Bash baseline
failures** caused by `catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh`
failing to parse under `/bin/bash` 3.2 (stock macOS): Bash 3.2 mis-tracks
backtick literals inside quoted heredocs nested in command substitution, so
every gate invocation exited 2 with "unexpected EOF while looking for matching
backtick", which surfaced as a large failure cluster in the hook/recipe tests.

Those failures are **addressed by the separate compatibility PR #189**
(`fix(worktree): make tracker-card-gate parse under bash 3.2`, commit
`192fd4e`, merged into `development` on 2026-08-09). PR #189 removes the
literal backticks from the embedded Python heredocs (building them at runtime
via `chr(96)`), adds a bash-3.2 regression test class, and documents baseline
"71 failed under 3.2 → fixed: 36 passed under both 3.2 and 5.3; full suite
1402 OK".

This change (`retire-decision-matrix`) is documentation/config/spec-only; it
touches none of the failing Bash machinery. Re-running the full suite after
merging `origin/development` (which contains #189) shows the baseline failures
are gone: **1378 tests, 0 failures, 0 errors, exit 0** — identical failure
count before and after this change's apply, with the delta attributable to
#189, not to this change.

## Success-criteria mapping

- Criterion 1: PASS — live `sdd-adaptive-contract` spec directory removed.
- Criterion 2: PASS — `openspec/config.yaml` parses (Ruby Psych) and has no
  live `sdd` key; live-tree scan matches only replacement plan-build
  vocabulary.
- Criterion 3: PASS — `docs/recipe-schema.md` no longer documents `[sdd]` or
  `threshold`.
- Criterion 4: PASS — `tests/test_manifest_contract_docs.py` passes (12/12)
  and rejects retired surfaces instead of asserting them.
- Criterion 5: PASS — Trello README ceremony note points at
  `plan-build-flow` depth vocabulary.
- Criterion 6: PASS — plan-build delta records legacy migration mapping; every
  scenario is covered by an explicit test or documented verification step.
- Criterion 7: PASS — archive subtree is byte-identical and unchanged
  (`git status -- openspec/changes/archive/` clean).
- Criterion 8: PASS — `./tests/run.sh` and `./tests/validate.sh` are green on
  the applied tree (GNU bash 5.3.9).

## Verify evidence

- Verdict: PASS
- Command: `./tests/validate.sh`
- Exit: 0
- Date: 2026-08-09
- Commit: b50dcc1
- ready_for_archive: true

## Verification notes

- Focused: `python3 -m unittest tests.test_manifest_contract_docs
  tests.test_plan_build_flow_recipe` — 37/37 pass, exit 0.
- `openspec/config.yaml`: Ruby Psych `YAML.unsafe_load_file` OK; `grep -n sdd`
  empty.
- Retired-token live scan: `sdd-adaptive-contract`, `openspec-sdd-decision`,
  `sdd.decision_matrix`, `sdd.threshold` absent from the four named live
  surfaces; the only test occurrences are the intentional ban-list literals in
  the enforcement tests, per the delta spec scenario exemption.
- Archive gate: `git status -- openspec/changes/archive/` clean before and
  after apply; no archive move performed (PR must be created first; archives
  preserved).
- Tracker: Trello card #66 (In Progress → Review) carries the change; tracker
  block present in `proposal.md` and `tasks.md` (`gate_mode: warn`).

`ready_for_archive: true` reflects artifact readiness; the archive-tail move
itself is deferred until the PR is created per the plan-build pre-merge
archive gate.
