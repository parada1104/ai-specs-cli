# Proposal: remove release blockers from bundled gate delivery

## Why

The bundled worktree-gate delivery has two release blockers that must be removed
before the next release can proceed:

1. **Divergent asset URL owner.** `lib/_internal/gate_binary.py` built the
   worktree-gate binary download URL from a hardcoded `nnodes` repository owner.
   The canonical owner everywhere else in the product is `parada1104` (the git
   remote, `install.sh`, `bin/ai-specs`, `catalog/README.md`, and the release
   workflow). The released worktree-gate assets are attached to
   `parada1104/ai-specs-cli` Releases, so the divergent `nnodes` owner made every
   `ai-specs sync` binary acquisition 404.
2. **Parity job depends on an undeclared test runner.** The release workflow
   parity job invoked `python3 -m pytest tests/test_worktree_gate_parity.py`,
   which depends on third-party `pytest` that is not declared/required by the
   repository. The stock GitHub runner has no `pytest`, so the parity job failed.
   The canonical runner is `unittest` (`python3 -m unittest discover -s tests -p
   'test_*.py'`, as executed by `./tests/run.sh`).

## What changes

1. Introduce canonical `REPO_OWNER`/`REPO_NAME` constants in
   `lib/_internal/gate_binary.py` and build the `_asset_url` from them, replacing
   the hardcoded `nnodes` owner.
2. Change the release parity job to run the parity test with the repository's
   actual unittest runner (`python3 -m unittest tests/test_worktree_gate_parity.py`)
   instead of the undeclared `pytest`.
3. Add regression tests guarding both fixes (asset URL owner and parity
   runner/tooling).

## Non-goals

- Worktree semantics, gate classification, or protected-branch enforcement.
- Plan/build-flow behavior, triggers, or skill wiring.
- Gentle AI integration or any agent/harness feature.
- Marketplace or packaging changes.
- Release publication, tagging, or asset attachment mechanics.

## Tracker

- **card_id**: 70
- **url**: https://trello.com/c/9cPC5FSU/70-story-remove-release-blockers-from-bundled-gate-delivery

## Plan

1. Record the change, classification, and reconciliation note.
2. Review the existing (already-implemented) diff for both fixes and their tests.
3. Run focused verification on the affected release/runtime paths and tests.
4. Run repository validation (`./tests/validate.sh` / `./tests/run.sh`).
5. Complete remaining delivery gates (commit planning files, PR, release).

## Process note

This change is a **process correction, not a new implementation**. The two fixes
described above were already implemented and are present in the current
uncommitted diff, because the plan-build-flow workflow was accidentally bypassed
during implementation. This planning package was written afterward to truthfully
document the already-authorized scope and implementation. No RED evidence and no
prior explicit authorization should be inferred from the existence of planning
files — they are produced retroactively to regularize the change.
