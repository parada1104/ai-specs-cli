# Exploration: migrate worktree cleanup to Go and close remote-branch cleanup

## Context

The `worktree-flow` recipe currently materializes
`catalog/recipes/worktree-flow/templates/worktree-cleanup.sh`. That script is a
large Bash implementation which scans linked worktrees, proves merge status,
and removes eligible local worktrees and branches. It does not delete the
corresponding remote branch after a merged pull request.

The same recipe already distributes the zero-dependency Go module at
`catalog/recipes/worktree-flow/gate`. The module is compiled with `CGO_ENABLED=0`,
`-trimpath`, and `-buildvcs=false`; release assets are selected by platform and
CLI-version cache key, verified against the committed `bin/SHA256SUMS`, and
self-tested before use. Acquisition failures preserve the existing fail-open
behavior for the gate implementation.

## Findings

1. **Merge proof is safety-critical and must be ported exactly.** The Bash
   reference resolves base candidates in this order: explicit base, configured
   upstream, configured remote-tracking ref, and conditional `origin/<base>`.
   It then checks ancestry, per-commit `git cherry` patch-id equivalence,
   combined patch-id equivalence, and combined final-tree-entry equivalence.
   The tree comparison uses NUL-delimited paths so filenames containing
   newlines are compared verbatim. A reverted squash must remain unmerged.
2. **Classification must precede every destructive action, but cannot be the
   only protection.** The requested protected-name set is `main`, `master`,
   `development`, `staging`, plus configured base/integration branch names.
   The set must be checked immediately before each `git worktree remove`, local
   branch deletion, and remote deletion. A protected name reaching a delete
   path is an explicit loud refusal, not a silent skip.
3. **The cleanup invocation starts from the main worktree.** `gh pr merge
   --delete-branch` cannot delete a local branch when the integration branch is
   already checked out by the main worktree. Cleanup therefore needs a distinct
   post-merge step that runs from the main worktree and owns remote deletion.
4. **The current submodule enumeration is a safety boundary.** The Go port must
   retain standalone/monorepo-apps single-pass behavior and per-initialized-
   submodule enumeration for monorepo-submodules. It must not trust the
   superproject worktree list for submodule-owned linked worktrees.
5. **The existing distribution seam can support cleanup without a second
   binary.** Keep one executable release asset (`worktree-gate-<platform>`) and
   add a cleanup command mode to the same Go module. The cleanup template becomes
   a small Bash launcher that resolves the same version-keyed verified cache (and
   optional project pin/explicit override), then `exec`s the Go binary with a
   cleanup subcommand. The existing gate launcher remains unchanged in behavior.
6. **Distribution must not make cleanup destructive on an unverified asset.** A
   cleanup launcher must never compile, download, or trust bytes itself. It must
   use the same verified-cache path and receipt contract as the gate launcher;
   when no verified binary exists, it must fail closed for cleanup with a loud
   actionable error rather than silently fall back to the old Bash implementation.
7. **Remote deletion must be verifiable and conservative.** After deleting a
   remote branch with `git push <remote> --delete <branch>`, the implementation
   must run `git ls-remote --heads <remote> <branch>` and only report success when
   the remote ref is absent. A failed or ambiguous verification must be loud and
   must not report the branch as cleaned.
8. **The batch iteration bug is real.** Any multi-module or multi-candidate list
   must be represented as arrays/slices and iterated structurally. A shell
   `for x in $VAR` loop is not an acceptable port because zsh does not word-split
   it by default and can turn a safety loop into one vacuous iteration.

## Options considered

### A. Keep Bash cleanup and add a separate remote-delete command

Rejected. It leaves the merge proof in Bash, duplicates distribution and trust
logic, and does not satisfy the whole-recipe Go migration.

### B. Add a second `worktree-cleanup` binary

Rejected. It duplicates release assets, platform/cache logic, digest entries,
self-test distribution, and user-facing upgrade behavior for a command that is
naturally a subcommand of the existing module.

### C. Add a cleanup subcommand to the existing Go module and make the template a launcher

Selected. It preserves the established release trust root and version-keyed
cache while keeping the materialized command path stable. The cleanup launcher
must be stricter than the gate fallback: missing/unverified Go cleanup cannot
silently re-enable a destructive legacy implementation.

## Test-first plan

The first implementation edit will be a failing test covering the cleanup
subcommand and protected-name refusal. Subsequent RED tests will cover unmerged,
dirty, protected-at-every-entry-point, batch iteration, remote deletion
verification, and the preserved merge-proof edge cases. The existing Python
integration tests remain the safety oracle for regular/squash/rebase/partial/
reverted/newline-path behavior.

## Scope boundary

In scope: the Go cleanup command, verified-cache cleanup launcher, remote branch
delete/verification, protected-name enforcement, Go tests, cleanup integration
tests, recipe docs/spec artifacts, and distribution/source checks needed to
ship the command.

Out of scope: changing GitHub `delete_branch_on_merge`, adding a per-PR API
field that does not exist, changing the gate policy, changing the materialized
path, changing the protected branch set semantics for the worktree gate, or
modifying provisioning-owned paths.
