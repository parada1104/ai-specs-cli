# Archive Report: trello-card-24

## Summary
Fixed a false negative in `worktree-cleanup.sh` where branches merged via regular merge commits on remote/base (but not yet fetched to local base) were incorrectly reported as unmerged. The fix adds candidate-base resolution that checks the exact `--base` ref, the base branch's configured upstream ref, and the remote-tracking ref `origin/<base>` before falling back to `git cherry` patch-id equivalence. All 12 new hermetic tests pass (10 initial + 2 added post-archive to lock down the WARNINGs), 700 pre-existing tests pass, shellcheck clean, manual PR #93 repro passes.

## Trello
- Card: `[bug] worktree-cleanup.sh false negative on regular merge commits`
- URL: https://trello.com/c/QX6xY3mQ
- Card ID: 6a2b7bc31402ac92f4bd2627

## Commits (on feat/trello-card-24)
- f631ff1 test(recipe): add hermetic tests for worktree-cleanup candidate base resolution
- e28b342 fix(recipe): resolve upstream and remote base candidates in worktree-cleanup heuristic
- 1a602aa chore(spec): archive trello-card-24 candidate base resolution
- acd1b9f test(recipe): lock down fast-forward and no-fetch behavior (closes 2 WARNINGs)

> The archive commit (`1a602aa`) landed before the post-verify WARNINGs were closed. The follow-up commit (`acd1b9f`) is on the same feature branch and is included in the final PR diff; the archive folder is a local bookkeeping view, not the source of truth for what ships.

## Files Changed
- `catalog/recipes/worktree-flow/templates/worktree-cleanup.sh` (+88, -13)
- `tests/test_worktree_cleanup.py` (+216, new across 2 commits: 134 + 82)
- `openspec/specs/worktree-flow/spec.md` (new, promoted from delta spec)
- `openspec/changes/archive/2026-06-12-trello-card-24/` (archived SDD artifacts)

## Spec Conformance
The delta spec at `openspec/changes/trello-card-24/specs/worktree-flow/spec.md` was promoted to the main spec at `openspec/specs/worktree-flow/spec.md` following project convention. The delta spec remains in the archived folder as part of the audit trail.

## Verify Findings Forwarded
- 3 WARNINGs issued at the original verify pass; 2 of 3 closed by `acd1b9f`:
  - ~~Fast-forward-only test not committed~~ — closed by `test_removes_fast_forward_merged_worktree`
  - ~~Missing-remote/no-fetch test not committed~~ — closed by `test_does_not_invoke_git_fetch_when_remote_missing`
  - Materialized copy rollout boundary (expected by design — `condition = "not_exists"`) — remains a forward-only note for a future card; not a code defect.
- T16 (materialized copy refresh) is intentionally skipped because `ai-specs/recipes/worktree-flow/bin/worktree-cleanup.sh` is absent in this worktree.

## Total Lines Changed
317 (implementation + tests only across both test commits, excluding OpenSpec artifacts)

## Open Items / Follow-ups
- Recipe `condition = "not_exists"` may require explicit version bump or migration for existing consumers; this is a separate concern from the cleanup heuristic fix. Worth a follow-up card.
