# Archive Report: trello-card-24

## Summary
Fixed a false negative in `worktree-cleanup.sh` where branches merged via regular merge commits on remote/base (but not yet fetched to local base) were incorrectly reported as unmerged. The fix adds candidate-base resolution that checks the exact `--base` ref, the base branch's configured upstream ref, and the remote-tracking ref `origin/<base>` before falling back to `git cherry` patch-id equivalence. All 10 new hermetic tests pass, 700 pre-existing tests pass, shellcheck clean, manual PR #93 repro passes.

## Trello
- Card: `[bug] worktree-cleanup.sh false negative on regular merge commits`
- URL: https://trello.com/c/QX6xY3mQ
- Card ID: 6a2b7bc31402ac92f4bd2627

## Commits (on feat/trello-card-24)
- f631ff1 test(recipe): add hermetic tests for worktree-cleanup candidate base resolution
- e28b342 fix(recipe): resolve upstream and remote base candidates in worktree-cleanup heuristic
- 82afb09 chore(spec): archive trello-card-24 candidate base resolution

## Files Changed
- `catalog/recipes/worktree-flow/templates/worktree-cleanup.sh` (+88, -13)
- `tests/test_worktree_cleanup.py` (+134, new)
- `openspec/specs/worktree-flow/spec.md` (new, promoted from delta spec)
- `openspec/changes/archive/2026-06-12-trello-card-24/` (archived SDD artifacts)

## Spec Conformance
The delta spec at `openspec/changes/trello-card-24/specs/worktree-flow/spec.md` was promoted to the main spec at `openspec/specs/worktree-flow/spec.md` following project convention. The delta spec remains in the archived folder as part of the audit trail.

## Verify Findings Forwarded
- 3 WARNINGs (see verify-report.md):
  - Fast-forward-only test not committed (manual verified)
  - Missing-remote/no-fetch test not committed (manual verified)
  - Materialized copy rollout boundary (expected by design — `condition = "not_exists"`)
- These are tracked for follow-up but do not block archive.
- T16 (materialized copy refresh) is intentionally skipped because `ai-specs/recipes/worktree-flow/bin/worktree-cleanup.sh` is absent in this worktree.

## Total Lines Changed
235 (implementation + tests only, excluding OpenSpec artifacts)

## Open Items / Follow-ups
- Consider adding the 2 missing dedicated tests (fast-forward-only, missing-remote/no-fetch) in a follow-up card.
- Recipe `condition = "not_exists"` may require explicit version bump or migration for existing consumers; this is a separate concern from the cleanup heuristic fix.
