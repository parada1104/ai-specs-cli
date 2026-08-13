# Spec: release blockers in bundled gate delivery

## Requirements

- REQ-1: `_asset_url(version, asset_name)` in `lib/_internal/gate_binary.py` must
  build the download URL from the canonical repository owner `parada1104` and
  repository name `ai-specs-cli`, not a hardcoded divergent owner.
- REQ-2: `lib/_internal/gate_binary.py` must not reference the divergent `nnodes`
  owner anywhere in the module.
- REQ-3: The release workflow parity job
  (`.github/workflows/release-worktree-gate.yml`, job `parity`) must invoke the
  parity test `tests/test_worktree_gate_parity.py` with the repository's canonical
  unittest runner (`python3 -m unittest ...`), consistent with `./tests/run.sh`.
- REQ-4: The parity job must not depend on `pytest` or any other undeclared
  third-party test runner.
- REQ-5: Release/runtime behavior outside these two fixes (binary build,
  SHA256SUMS gate, asset attachment, gate logic) must remain unchanged.

## Scenarios

### S1: sync resolves a worktree-gate asset URL -> canonical owner
Event: `gate_binary._asset_url("9.9.9", "worktree-gate-darwin-arm64")` is
invoked during a sync acquisition attempt.
Result: URL is
`https://github.com/parada1104/ai-specs-cli/releases/download/v9.9.9/worktree-gate-darwin-arm64`
— no `nnodes` owner anywhere in the URL.

### S2: module regression check -> no divergent owner
Event: the full source of `lib/_internal/gate_binary.py` is scanned for the
divergent `nnodes` owner.
Result: no match — the regression test
`test_gate_binary_module_has_no_divergent_repository_owner` passes.

### S3: release parity job on a stock GitHub runner -> runs unittest
Event: the `parity` job of `release-worktree-gate.yml` executes on
`ubuntu-latest` with only `actions/setup-python` (no third-party packages
installed).
Result: the job runs `python3 -m unittest tests/test_worktree_gate_parity.py`
and does not fail because `pytest` is absent.

### S4: parity tooling regression check -> no pytest dependency
Event: the parity section of `.github/workflows/release-worktree-gate.yml` is
inspected for test-runner references.
Result: `python3 -m unittest` is present, `tests/test_worktree_gate_parity.py`
is referenced, and `pytest` is absent — the regression test
`test_release_workflow_parity_job_runs_unittest_not_pytest` passes.

### S5: workflow failure behavior -> parity job fails the release
Event: the parity test detects a deviation from the frozen Bash reference corpus.
Result: the parity job exits non-zero and fails the release workflow — parity
remains a release gate, only the runner/tooling is corrected.

### S6: regression protection -> future edits keep both fixes
Event: a future change reintroduces `nnodes` into `gate_binary.py` or switches
the parity job back to `pytest`.
Result: the corresponding regression tests fail and the change cannot land
unnoticed.

## Failure behavior

- REQ-1/REQ-2 failure (divergent owner in URL): every `ai-specs sync` binary
  acquisition 404s; sync degrades to its warning path and the gate binary cannot
  be acquired from Releases.
- REQ-3/REQ-4 failure (pytest dependency): the parity job fails on the stock
  GitHub runner with `ModuleNotFoundError: No module named 'pytest'`, blocking
  the release.

## Regression protection

- `tests/test_gate_binary_dist.py`: asserts the exact canonical asset URL and
  scans the module source for the divergent owner.
- `tests/test_worktree_gate_release_phase4.py`: parses the parity section of the
  release workflow and asserts the unittest runner is used and pytest is absent.
