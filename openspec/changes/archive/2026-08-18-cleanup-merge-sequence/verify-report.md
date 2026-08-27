# Verification report: cleanup-merge-sequence

## Verify evidence

- Verdict: PASS
- Command: `./tests/validate.sh`
- Exit: 0
- Result: **1871 tests, OK**
- Date: 2026-08-18
- Commit: 873b003
- ready_for_archive: true

The exit code was captured from a direct run, not from a pipeline. An earlier
attempt read `./tests/validate.sh 2>&1 | tail -40`, whose exit status belongs to
`tail`, not to the suite — the same class of measurement error this session has
already produced more than once. That reading was discarded and the suite re-run
without a pipe.

## Focused evidence

- `go vet ./...` in `catalog/recipes/worktree-flow/gate` — PASS.
- `go test ./...` in `catalog/recipes/worktree-flow/gate` — PASS.
- `gofmt -l` — clean.
- `python3 -m pytest tests/test_git_pr_flow_recipe.py` — 14 passed.

## Falsifiability

Every correction in this round was proven able to fail. A test that cannot fail
is not evidence, and this change's original defect was a test that asserted the
bug.

| Correction | Sabotage applied | Result |
|---|---|---|
| Stale branches need content proof | Force the refusal to `return true, "merged"` | `TestCleanupRefusesStaleBranchWhoseMergeCannotBeProven` and `TestStaleBranchWithLandedContentIsStillRemovable` both fail |
| NUL-delimited path reassembly | Replace `bytes.Split(paths, []byte{0})` with a newline split | `TestCleanupProvesMergeForNewlinePathInTree` fails; the pre-existing `TestCleanupPreservesNewlinePathInTreeProof` keeps passing, which is exactly the asymmetry Judge B reported |
| Remote deletion before local | Restore the original order | `TestRemoteDeletionFailureLeavesLocalBranchForRetry` fails on the local-branch survival assertion |
| Documented cleanup order | Swap the two steps in `SKILL.md` prose | `test_skill_documents_the_implemented_cleanup_order` fails |

`TestRemoteDeletionFailureLeavesLocalBranchForRetry` does not stop at survival.
It deletes the bare remote to force the failure, then re-creates and re-pushes
it and runs cleanup again, asserting both the local and remote branches are gone
and the pass exits 0. Recoverability is demonstrated, not argued.

## Trust root

`catalog/recipes/worktree-flow/bin/SHA256SUMS` was regenerated with the canonical
`go1.24.13` toolchain **after** the final `cleanup.go` edit:

```
0a258db907d9099c166d03a55b2c189a32222adfc71dd9185eb41cc741aba3f6  worktree-gate-darwin-arm64
9abf689407e42593d5da643cb4484810ab7978475d365ea45ea2573c10b82f93  worktree-gate-darwin-amd64
5312ae6f886d0a9fa9fb73222f4339101e7d8a929609f88d2f07f940a6dcae80  worktree-gate-linux-amd64
7d78e54bb4ef34addd7cd10d7cd3684c22e924118f364c0a2c3359fd6e2fcbce  worktree-gate-linux-arm64
```

A stale trust root fails the CI checksum gate on tag push and publishes zero
gate assets, dropping every user silently to the Bash fallback. The previous
change in this area learned that the expensive way.

## Known non-findings

The suite logs three `worktree-gate: download failed … v0.21.0 … HTTP 404`
warnings and falls back to the Bash implementation. Those fixtures pin `v0.21.0`,
whose assets were never published; the current trust root is `v0.22.0`. This is
pre-existing and unrelated to this change — the fallback behaving exactly as
designed.

## Success-criteria mapping

- Criterion 1: PASS — `cleanupOnePass` removes the worktree, deletes and verifies
  the remote branch, then deletes the local branch; `syncBaseCleanup` runs
  `git pull --ff-only` only after every pass returns without failures, and is a
  no-op under `--dry-run`.
- Criterion 2: PASS — `test_skill_never_recommends_delete_branch` asserts the
  flag is absent, and `test_skill_documents_the_implemented_cleanup_order`
  asserts the documented sequence matches the implemented one rather than merely
  containing the right words.
- Criterion 3: PASS — the path-presence fallback is removed.
  `TestCleanupRefusesStaleBranchWhoseMergeCannotBeProven` proves an unprovable
  branch survives; `TestStaleBranchWithLandedContentIsStillRemovable` proves the
  feature still works for a branch that genuinely landed.
- Criterion 4: PASS — `isMergedCleanup` and its three-term OR chain are
  unchanged; `assertDeletable` still guards every destructive call and
  `branchHeldByWorktree` still refuses a branch a worktree holds.
- Criterion 5: PASS — candidates are iterated with explicit Go range loops over
  NUL-delimited fields; `TestCleanupProvesMergeForNewlinePathInTree` proves exact
  path reassembly, and `TestCleanupContinuesAfterOneCandidateFails` proves one
  failure does not abandon the rest of the pass.
- Criterion 6: PASS — `./tests/validate.sh` exit 0, 1871 tests, OK, on commit
  `873b003`. `catalog/recipes/worktree-flow/bin/SHA256SUMS` regenerated with the
  canonical go1.24.13 toolchain after the final `cleanup.go` edit.
