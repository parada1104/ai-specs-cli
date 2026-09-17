# Proposal: migrate worktree cleanup to Go and delete merged remote branches

## Intent

Replace the Bash implementation behind the `worktree-flow` cleanup recipe with
an executable Go cleanup mode in the existing zero-dependency `worktree-gate`
module, while closing the post-merge remote-branch cleanup gap. The materialized
cleanup path remains stable, but it becomes a thin launcher for the same
version-keyed, digest-verified binary distribution already used by the recipe's
worktree gate.

## Problem

Remote branch deletion is currently missing from `worktree-cleanup.sh`. GitHub's
repo-wide `delete_branch_on_merge` cannot be enabled because it would delete the
long-lived `development` head, and GitHub exposes no per-PR override. `gh pr
merge --delete-branch` also cannot solve this layout: it switches to the base
branch before deleting the local branch, but `development` is already checked
out in the main worktree. The cleanup step must therefore run from the main
worktree after merge and explicitly own remote deletion.

The current cleanup implementation also remains in Bash even though the recipe
already distributes a Go module with pinned multi-architecture release assets,
version-keyed caches, digest verification, self-test receipts, and fail-open
behavior for the non-destructive gate. The cleanup path is destructive and
cannot use an unverified or silently missing implementation.

## Goals

- Port the current merge proof without weakening it: ordered base-ref search,
  ancestry, `git cherry` patch-id, combined patch-id, final-tree-entry
  equivalence with NUL-delimited paths, and reverted-squash rejection.
- Refuse dirty worktrees and unmerged branches.
- Check protected names immediately before every destructive call. The protected
  set is `main`, `master`, `development`, `staging`, configured `base_branch`,
  and configured `integration_branch`; a protected name reaching deletion is a
  loud refusal.
- Delete the merged local branch/worktree only when safe, then delete its remote
  branch from the main worktree and verify absence with
  `git ls-remote --heads <remote> <branch>`.
- Preserve topology behavior and make batch iteration explicit in Go slices,
  proving that every candidate/module is evaluated.
- Reuse the existing Go module's build, cache, digest, and self-test distribution
  model without adding third-party dependencies or a second binary asset.
- Keep the materialized cleanup path and CLI flags compatible.

## Non-goals

- Enabling GitHub repository-wide branch deletion.
- Inventing a per-PR GitHub API override.
- Running cleanup from a feature worktree or changing worktree ownership rules.
- Weakening the gate policy or changing unrelated provisioning outputs.
- Replacing the existing trusted release asset with an ad-hoc local build in a
  normal cleanup invocation.

## Affected areas

| Area | Impact |
|---|---|
| `catalog/recipes/worktree-flow/gate/` | Add cleanup command, merge proof, topology scan, safety checks, remote deletion verification, and Go tests. |
| `catalog/recipes/worktree-flow/templates/worktree-cleanup.sh` | Replace the Bash algorithm with a thin, verified-binary launcher. |
| `catalog/recipes/worktree-flow/README.md` and `commands/worktree-clean.md` | Document main-worktree invocation, remote deletion, verification, and fail-closed cleanup behavior. |
| `catalog/recipes/worktree-flow/recipe.toml` | Expose configured base/integration/protected-name inputs needed by cleanup. |
| `tests/test_worktree_cleanup.py` and new Go/integration tests | Preserve existing proof cases and add safety/remote/batch regressions. |
| `scripts/build-gate.sh`, release/checksum documentation/tests | Ensure the single Go asset remains the distribution unit and cleanup mode is covered by self-test. |

## Design summary

The existing binary receives a new explicit cleanup subcommand. The cleanup
launcher resolves, in order, an explicit executable override, project-local
verified pin, and the existing version-keyed cache. It requires a current
verification receipt and executes the binary; it does not fall back to the old
Bash algorithm for destructive operations. The cleanup command accepts the
existing `--dir`, `--base`, `--dry-run`, `--topology`, and repeated
`--submodule`/`--subrepo` options, plus explicit remote/configuration inputs
where needed.

The Go implementation models worktree records and repository/module passes as
slices, scans each pass exactly once, and performs a final protection check
immediately before each `git worktree remove`, `git branch -d/-D`, and
`git push ... --delete`. A protected branch/name produces a non-zero exit and a
message containing the operation and branch. Remote deletion is attempted only
when a configured remote is known and the branch is proven merged; absence is
then asserted with `git ls-remote --heads`.

## Rollback plan

1. Set the cleanup launcher override to a previously verified binary containing
   the prior cleanup behavior, or restore the prior materialized managed copy
   through the normal recipe sync/rollback mechanism.
2. Disable automatic cleanup (`auto_remove_merged = false`) and use dry-run
   inspection while the release asset is investigated.
3. If the Go asset cannot be acquired or verified, cleanup fails closed and makes
   no destructive change; users can inspect and remove worktrees manually from
   the main worktree under ordinary repository policy.
4. Revert the Go cleanup source and launcher changes as one reviewed change;
   no GitHub repository setting or remote state migration is required.

## Tracker

- **card_id**: `86`
- **url**: https://trello.com/c/ZiRie66n

## Success criteria

- [ ] Existing regular, fast-forward, rebase, squash, partial-squash,
      reverted-squash, dirty, detached, active-merge, stale-base, dual-remote,
      no-fetch, and newline-path merge-proof tests remain green.
- [ ] Protected names are refused loudly immediately before every destructive
      entry point, including worktree removal, local branch deletion, and remote
      deletion; configured base/integration names are included.
- [ ] Unmerged and dirty worktrees are preserved, and a branch held by any
      worktree is never deleted remotely or locally.
- [ ] Multiple candidates/modules are iterated structurally; a regression to a
      scalar shell word-splitting loop is caught by a test that proves all batch
      members are visited.
- [ ] A merged remote branch is deleted from the main-worktree cleanup step and
      absence is proven with `git ls-remote --heads`; failed verification is a
      loud non-success.
- [ ] The cleanup template is only a launcher; destructive cleanup does not use
      an unverified or missing binary and does not silently fall back to Bash.
- [ ] The recipe uses the existing Go build matrix, version-keyed cache, digest
      trust root, and self-test/receipt contract without a second binary asset.
- [ ] `./tests/validate.sh` passes, including strict TDD RED/GREEN evidence and
      the full existing test suite.
