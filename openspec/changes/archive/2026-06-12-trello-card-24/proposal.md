# Proposal: trello-card-24 cleanup heuristic fix

## Intent

Fix a false negative in `worktree-cleanup.sh` after regular GitHub merge commits: local `development` may be stale while `origin/development` already contains the merge commit, so the script incorrectly marks a merged clean worktree as `unmerged`. The fix keeps the conservative cleanup philosophy: add only positive proof sources before declaring unmerged, never remove a worktree unless ancestry or patch-id equivalence proves the branch landed.

## Scope — In

- Expand the merge heuristic around `is_merged` to evaluate ordered base candidates.
- Add candidate-base resolution for exact `--base`, configured upstream, and remote-tracking `<remote>/<base>`.
- Preserve existing `git cherry` patch-id fallback for squash/rebase merges.
- Add hermetic tests for stale local base with remote regular merge, plus edge cases from exploration.
- Update changelog or recipe release notes if the recipe version changes for rollout.

## Scope — Out

- Broad refactors of the cleanup script or worktree workflow.
- Network fetches or cleanup strategies with new side effects.
- Changing the conservative bias: dirty or genuinely unmerged worktrees remain preserved.
- New PR, branch deletion, or tracker automation behavior.

## Approach

Modify `is_merged` and add a small helper such as `base_candidates` / `base_candidate_refs`. Candidate order: exact `--base`; upstream ref for a local base when configured via `git rev-parse --verify --quiet "<base>@{upstream}"`; then configured remote-tracking ref, defaulting to `origin/<base>` when present. Run `git merge-base --is-ancestor "$sha" "$candidate"` across candidates first; if none proves ancestry, run existing `git rev-list "$candidate..$sha"` and `git cherry "$candidate" "$sha"` patch-id logic across valid candidates.

## Affected files

- `catalog/recipes/worktree-flow/templates/worktree-cleanup.sh` — canonical script and merge heuristic source.
- `tests/test_worktree_cleanup.py` — hermetic regression and edge-case coverage.
- `catalog/recipes/worktree-flow/recipe.toml` — possible version bump/rollout metadata; template uses `condition = "not_exists"`.
- `ai-specs/recipes/worktree-flow/bin/worktree-cleanup.sh` — expected materialized consumer path, but not tracked/present in this worktree; sync impact must be handled explicitly if generated.

## Risks

- False positive regression could remove a branch whose changes did not land; mitigate with positive-proof-only tests.
- Drift between canonical template and any materialized copy can leave consumers on old behavior.
- `condition = "not_exists"` means existing consumer scripts may not update automatically.
- ai-specs-cli dogfoods `worktree-flow`; rollout must not assume local generated files changed unless sync actually materializes them.

## Acceptance criteria

- Regular merge commits are detected via remote-tracking base ancestry when local base is stale.
- Squash merges remain detected by `git cherry` patch-id equivalence.
- Rebase merges remain detected by patch-id equivalence.
- Fast-forward and local-base ancestry behavior still works.
- Local-only, remote-deleted, and genuinely ahead branches remain `unmerged` unless positive proof exists.
- Dirty worktrees are skipped before removal.
- Conflict-resolution merge commits are detected as regular merges.
- Tests cover the card regression and preserve existing dry-run/removal behavior.

## Test strategy outline

Use `tests/test_worktree_cleanup.py` with temporary repositories only. Reproduce the card by creating a feature worktree, synthesizing or performing a regular merge into `refs/remotes/origin/main` while leaving local `main` stale, then asserting `--base main --dry-run` prints `would remove`.

Also cover patch-id paths, dirty skip ordering, branch-ahead safety, and conflict-resolution merge commits. Avoid live GitHub/remotes; use `git update-ref`, deterministic author env, and the canonical template script.

## Migration / rollout notes

Because the recipe template target uses `condition = "not_exists"`, existing consumers may not receive the script change from `ai-specs sync` if their materialized file already exists. For ai-specs-cli dogfood, confirm whether sync materializes the missing local `ai-specs/recipes/worktree-flow/bin/worktree-cleanup.sh`; if it does not, the canonical template and recipe version/release note are the rollout mechanism, and downstream users may need a chore bump or manual migration.
