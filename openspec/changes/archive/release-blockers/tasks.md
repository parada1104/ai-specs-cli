# Tasks: remove release blockers from bundled gate delivery

Depth: standard

## Process note

This change is a **process correction**: the implementation already exists in
the current uncommitted diff because the plan-build-flow workflow was
accidentally bypassed. Planning artifacts are written retroactively to
truthfully document the already-authorized scope and implementation. Tasks
marked `[done]` under **Existing implementation review** are supported by the
current diff evidence only. Focused verification was run during this
regularization session (evidence recorded below); full validation is recorded
below and passed. No RED evidence and no prior authorization are claimed.

## Planning / reconciliation

- [x] **Classify change** — Standard tier (release/runtime Python code, release
  CI, and tests) and record it in `tasks.md`. *(done: this file, `Depth: standard`)*
- [x] **Write proposal** — scope, non-goals, Tracker section with Trello card
  #70. *(done: `proposal.md`)*
- [x] **Write spec** — observable requirements, scenarios, failure behavior, and
  regression protection. *(done: `specs/release/release-blockers.md`)*
- [x] **Write tasks** — this file. *(done)*
- [x] **Record workflow incident** — note that implementation preceded artifact
  creation due to the accidentally bypassed plan-build-flow. *(done: process
  notes in proposal/tasks; no RED or prior authorization claimed)*

## Existing implementation review

- [x] **Review asset-URL fix** — `lib/_internal/gate_binary.py` adds
  `REPO_OWNER = "parada1104"` / `REPO_NAME = "ai-specs-cli"` and builds
  `_asset_url` from them (was hardcoded `nnodes`). Present in the uncommitted
  diff; implementation already performed.
- [x] **Review parity-job fix** — `.github/workflows/release-worktree-gate.yml`
  parity job now runs `python3 -m unittest tests/test_worktree_gate_parity.py`
  with a comment documenting the pytest incident. Present in the uncommitted
  diff; implementation already performed.
- [x] **Review regression tests** — `tests/test_gate_binary_dist.py` adds two
  tests (canonical asset URL, no divergent owner in module source);
  `tests/test_worktree_gate_release_phase4.py` adds one test (parity job uses
  unittest, not pytest). Present in the uncommitted diff; implementation already
  performed.
- [x] **Confirm no out-of-scope drift** — the uncommitted diff touches exactly
  four files (`.github/workflows/release-worktree-gate.yml`,
  `lib/_internal/gate_binary.py`, `tests/test_gate_binary_dist.py`,
  `tests/test_worktree_gate_release_phase4.py`); no worktree/plan-build/
  Gentle-AI/marketplace or generated-file changes present. *(verified against
  `git diff --stat` during regularization)*

## Focused verification

- [x] **Run asset-URL tests** — `python3 -m unittest discover -s tests -p
  'test_gate_binary_dist.py' -q` ran 15 tests, all OK (includes both asset-URL
  regressions). *(run during regularization; result recorded below)*
- [x] **Run parity-tooling test** — `python3 -m unittest discover -s tests -p
  'test_worktree_gate_release_phase4.py' -q` ran 10 tests, all OK (includes
  `test_release_workflow_parity_job_runs_unittest_not_pytest`). *(run during
  regularization; result recorded below)*
- [x] **Inspect workflow command validity** — the parity job now runs
  `python3 -m unittest tests/test_worktree_gate_parity.py -q`; the parity
  corpus itself is unittest-based (no pytest import in
  `tests/test_worktree_gate_parity.py`).
- [x] **Spot-check URL construction** — `_asset_url` builds
  `https://github.com/{REPO_OWNER}/{REPO_NAME}/releases/download/v<version>/<asset>`
  with `REPO_OWNER = "parada1104"`, matching the canonical owner confirmed via
  `git remote -v`.

### Focused verification evidence (collected during regularization)

```
$ python3 -m unittest discover -s tests -p 'test_gate_binary_dist.py' -q
Ran 15 tests in 0.970s
OK

$ python3 -m unittest discover -s tests -p 'test_worktree_gate_release_phase4.py' -q
Ran 10 tests in 4.349s
OK
```

## Full validation

- [x] **Run repository suite** — `./tests/validate.sh` (syntax checks then
  `./tests/run.sh`) from the repo root and fix any regression. *(passed during
  regularization: 1580 tests OK, 110 skipped, exit 0)*
- [x] **Run Go gate tests** — `go -C catalog/recipes/worktree-flow/gate test
  ./...` (covered by `./tests/run.sh` when `go` is on PATH). *(passed as part
  of `./tests/validate.sh`)*

## Remaining delivery gates

- [ ] **Commit planning files** — commit `openspec/changes/release-blockers/`
  together with the implementation on the review branch (PR creation gate
  requires the change folder committed).
- [ ] **Open PR** — after the artifact gate passes and focused/full validation
  is green.
- [ ] **Run archive-tail before merge** — move
  `openspec/changes/release-blockers/` to `openspec/changes/archive/release-blockers/`,
  commit, and push to the review branch before merge.
- [ ] **Release** — tag push triggers `release-worktree-gate.yml`; the parity
  job must pass on the stock runner and the bundled gate assets must download
  from the canonical owner URL.

## Review workload forecast

- Expected surface: `lib/_internal/gate_binary.py`, the release workflow, two
  test files, and this planning package.
- Standard review risk: URL owner correctness and CI tooling alignment.
- Adversarial cases: divergent owner reintroduced anywhere in the module;
  parity job switched back to pytest; URL template drift.
