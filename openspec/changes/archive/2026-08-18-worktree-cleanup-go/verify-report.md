# Verification report: worktree-cleanup-go

## Verify evidence

- Verdict: PASS
- Command: `./tests/validate.sh`
- Exit: 0
- Date: 2026-08-18
- Commit: 0511c51
- ready_for_archive: true

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

**exit 0 — 1834 tests, 0 failures**, run by the coordinator on commit `0511c51`.

### Correction: the earlier failures were NOT pre-existing

An earlier draft of this report attributed 10 failures to "existing
release/root-propagation checks". That attribution was wrong, and it was
established by running the same suites on the base branch:

| Suite | clean `development` | this worktree, before the fix |
|---|---|---|
| `test_worktree_gate_release_phase4` | 10 OK | 9 failures |
| `test_worktree_root_propagation` | 10 OK | 1 failure |

This change caused them. Migrating cleanup into the gate Go module changes the
built binary, so the committed `catalog/recipes/worktree-flow/bin/SHA256SUMS`
trust root still described the previous binary:

```
committed : 075aff9f0ae1853b1b477b9f6e5e6bd5df65f63c99abe1ad941ae5ac8abd322a  worktree-gate-darwin-arm64
rebuilt   : 215f391a6cc559467cd334c2d5002abec3d438ab2e064e9b1fbe8230d444a969  worktree-gate-darwin-arm64
```

Regenerating the digests with the canonical `go1.24.13` restored both suites to
10 OK. Left unregenerated, the release workflow's checksum gate fails on tag
push and publishes **zero** gate assets, silently dropping every user to the
Bash fallback.

**Discipline this cost:** a failure is only pre-existing once it has been
reproduced on the base branch. Until then it is a hypothesis.

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
- Criterion 5: PASS — remote deletion is covered by the focused suite and the full suite now passes (exit 0, 1834 tests), so the qualifier no longer applies
  reports success only after `git ls-remote --heads` proves absence.
- Criterion 6: PASS — the cleanup template is a verified-binary launcher and
  exits 2 without a current executable receipt; no destructive Bash fallback is
  present.
- Criterion 7: PASS — the existing single Go module/build matrix/cache/digest/
  receipt distribution remains the cleanup implementation's distribution seam.
- Criterion 8: PASS — `./tests/validate.sh` exit 0, 1834 tests, 0 failures on commit 0511c51; the blocking digest failures were this change's own and were resolved by regenerating the trust root, not waived
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
