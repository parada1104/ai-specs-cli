# Proposal: Make the change-artifact guard archive-aware

## Why

`tests/test_bitbucket_pr_flow_recipe.py::BitbucketPrFlowGoldenContentTests::test_apply_progress_omits_absolute_host_and_worktree_paths`
guards that a change's `apply-progress.md` never embeds absolute host paths. It resolves the artifact
from the **active** change folder only:

```python
progress = ROOT / "openspec" / "changes" / "bitbucket-bb-cli-alignment" / "apply-progress.md"
```

That change was archived to `openspec/changes/archive/2026-09-07-bitbucket-bb-cli-alignment/`, so the
active path no longer exists and the guard fails permanently — for every contributor, on every checkout,
regardless of their work.

## Evidence

- `./tests/validate.sh` and `./tests/run.sh` on this change's base: `FAILED (failures=1)`, the single
  failure being this test (`AssertionError: False is not true : missing
  .../openspec/changes/bitbucket-bb-cli-alignment/apply-progress.md`).
- Reproduced on the untouched `development` checkout at `6f5bf73` with no other change in play: the
  single test fails there too, so the defect predates this branch.
- The test file is committed and unmodified since `cf09e1d`; the folder it requires is archived at
  `openspec/changes/archive/2026-09-07-bitbucket-bb-cli-alignment/` in both checkouts.
- **Count caveat (raised by both blind Judgment Day judges):** an absolute test count is not portable
  across worktrees. Discovery on this machine is 1821 on `development`, 1829 here (1821 plus the new
  tests) and 1903 in the `jinna-mcp-recipe` worktree, whose branch adds roughly 74 tests; the runner
  reports 1826 here. An earlier draft of this section quoted `Ran 1900 tests`, which belongs to that
  other worktree and not to `6f5bf73`. The gate for this change is a green suite and exit code 0, never
  a borrowed count.
- Discovered while verifying the `jinna-mcp-recipe` change, which recorded it as a known,
  non-regressive limitation instead of absorbing an unrelated fix into its own PR.

## What changes

- Add `tests/_change_paths.py`, an archive-aware resolver for change artifacts, following the existing
  `tests/_cache_paths.py` convention: active folder first, then the date-prefixed archive entry, then the
  legacy undated `archive/<slug>/` fallback.
- Use the resolver in the guard so it keeps verifying the archived artifact instead of failing on its
  absence. The assertion itself (no absolute host paths in the artifact) stays exactly as it is.

## Non-goals

- No production code, CLI, schema, recipe, or documentation behavior changes: this is a test-only fix.
- No change to the guard's intent, nor to which artifact it verifies — only to how its path is resolved.
- Archiving behavior itself is out of scope. Guards must survive archiving; they must not prevent it.
- The same latent pattern in `tests/test_jinna_provider_recipe.py` (it points at the still-active
  `jinna-mcp-recipe` folder and will break once that change is archived) is recorded here as a known
  follow-up rather than fixed here: that file lives on another branch, and this change introduces the
  helper that makes the follow-up a one-line adoption.

## Risk and rollback

- Risk: low. Test-only, no runtime surface, no user-facing behavior.
- Rollback: revert the commit. The guard returns to its current permanently-failing state; nothing else
  is affected.

## Tracker

- card_id: `6aa3cb78e51fefeffe5b11fc`
- url: https://trello.com/c/8v4lnhzg
- list: In Progress
