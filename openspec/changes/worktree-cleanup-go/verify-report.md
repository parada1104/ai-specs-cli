# Verification report: worktree-cleanup-go

## Verify evidence

- Verdict: NEEDS DECISION
- Command: `./tests/validate.sh`
- Exit: 1
- Date: 2026-08-18
- Commit: working tree (not committed; coordinator owns commit)
- ready_for_archive: false

## Focused evidence

- `go -C catalog/recipes/worktree-flow/gate test ./...` — PASS.
- `go -C catalog/recipes/worktree-flow/gate vet ./...` — PASS.
- `python3 -m unittest tests.test_worktree_cleanup -q` — PASS (30 tests).
- `python3 -m unittest tests.test_worktree_flow_recipe tests.test_worktree_cleanup -q` — PASS (50 tests).
- `bash -n catalog/recipes/worktree-flow/templates/worktree-cleanup.sh` — PASS.
- `git diff --check` — PASS.

The cleanup integration suite now builds the Go module in a temporary binary,
invokes the stable launcher through a verified test pin, and preserves all
existing merge-proof scenarios. Remote integration verifies the branch is absent
with `git ls-remote --heads`; protected-name wrappers refuse before each
worktree/local/remote destructive entry point; dirty/unmerged and structural
batch cases are covered.

## Full validation result

`./tests/validate.sh` ran the full repository suite (`Ran 1831 tests`) but exited
1 with failures in existing worktree-gate release/root-propagation checks:

1. `test_canonical_sums_comparison_ignores_header_and_order` and the related
   committed digest tests fail because the locally built gate assets do not match
   the repository's existing `SHA256SUMS` values. This is expected when the
   working tree changes the Go module and the trust-root digest file was not
   regenerated; no digest file was changed because release asset regeneration is
   coordinator-owned and the task forbids staging/committing.
2. `test_sync_stamps_launcher_and_builds_gate_into_scratch_cache` fails in the
   pre-existing root-propagation path because its source checkout/cache fixture
   expects the current release distribution state. Focused recipe and cleanup
   suites pass; the test failure needs coordinator review before claiming a full
   green validation.

No provisioning-owned paths were edited by this implementation. The
materialized cleanup override was not edited directly.

## Success-criteria mapping

- Criterion 1: PASS — Go cleanup integration preserves regular, fast-forward,
  squash/rebase, partial/reverted squash, dirty, detached, stale-base,
  dual-remote, no-fetch, and newline-path scenarios; reverted-squash and newline
  cases have dedicated Go tests.
- Criterion 2: PASS — built-in/configured protected names are checked immediately
  before worktree, local branch, forced local branch, remote, and verification
  deletion wrappers; refusals are loud errors.
- Criterion 3: PASS — dirty/unmerged worktrees are preserved, main/linked
  invocation boundaries are enforced, and held-branch checks run before local
  and remote deletion.
- Criterion 4: PASS — candidates/modules are slices with explicit Go range loops;
  Python integration proves every candidate and initialized submodule is visited.
- Criterion 5: PASS (focused) — remote deletion uses `git push --delete` and
  reports success only after `git ls-remote --heads` proves absence.
- Criterion 6: PASS — the cleanup template is a verified-binary launcher and
  exits 2 without a current executable receipt; no destructive Bash fallback is
  present.
- Criterion 7: PASS — the existing single Go module/build matrix/cache/digest/
  receipt distribution remains the cleanup implementation's distribution seam.
- Criterion 8: NEEDS DECISION — focused tests are green, but the full validation
  command is blocked by the two release/root-propagation failures above.

## Quality signals

- Coverage: not configured.
- Linter: not configured; `go vet` passed.
- Type checker: not configured.
- Formatter: `gofmt` applied; `gofmt -l` is part of validation.

## Falsifiability / TDD note

Each new behavior was introduced with a failing focused assertion before the
corresponding production path: command registration/protected wrappers,
classification and iteration, remote deletion verification, newline tree proof,
and reverted squash rejection. The final focused runs were repeated after each
fix. A full revert-and-rerun of every individual fix was not performed after the
entire batch because the repository's requested worker boundary prohibits
commits/rollback history manipulation; the RED failures are recorded by the
initial focused runs and are reproducible by removing the corresponding Go
implementation/wrapper.
