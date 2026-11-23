# Worktree Flow recipe

Isolated git worktrees under `.worktrees/` for ai-specs change work, with safe
post-merge cleanup.

## What it provides

- **Skill `worktree-flow`** — when to create a worktree (file-writing work) vs.
  stay outside one (pure exploration), naming conventions, and cleanup rules.
- **Commands `/worktree-new`, `/worktree-clean`** — agent-facing flows to create
  a worktree for a change and to reclaim merged worktrees.
- **`ai-specs worktree-cleanup`** — the CLI verb (no materialized script): removes
  only merged + clean worktrees, preserves dirty and unmerged ones, never touches
  protected heads, deletes merged remote branches from the main worktree, verifies
  remote absence with `git ls-remote --heads`, and closes the matching tracker-ledger
  item at `archive-close` before any destructive removal.
- **Managed `.git/hooks/post-merge` trigger** — a one-line CLI hook that runs that
  cleanup automatically at a merge boundary. Sync always regenerates it.
  It is workflow-agnostic: no SDD/ODD/OpenSpec dependency and no provider mutation. Cleanup may perform the documented read-only PR merge-commit acquisition when local proofs are inconclusive.

## Enable

```toml
[recipes.worktree-flow]
enabled = true
version = "1.6.0"

[recipes.worktree-flow.config]
worktrees_dir = ".worktrees"
integration_branch = "main"
auto_remove_merged = true
gate_scope = "auto"
gate_impl = "auto"
```

Repository topology is CLI-owned and declared once on the project:

```toml
[project]
repo_topology = "auto"   # auto | standalone | monorepo-apps | monorepo-submodules
```

Planning, sync stamping, the brief, `doctor`, and `status` all read that one
value, and it keeps resolving even when this recipe is disabled.
`recipes.worktree-flow.config.repo_topology` stays readable as a deprecated
alias for one migration window (the project field wins; a recipe-only value is
reported with a deprecation marker), and it is never written by new flows.

Then run `ai-specs sync`. Sync writes the resolved cleanup policy (`worktrees_dir`,
`integration_branch`, `[project].repo_topology`, protected branches) into the
per-project cache snapshot, and `ai-specs worktree-cleanup` reads that snapshot —
never `ai-specs.toml`. Without a snapshot the verb removes nothing and exits 2,
telling you to run `ai-specs sync` first. There is no launcher script and no
`WORKTREE_CLEANUP_BIN` escape hatch: cleanup always runs through the installed
CLI.

## Worktree-gate modes

`worktree-flow` also gates writes to the main worktree on protected branches via
`gate_mode`:

| Mode | Behavior |
|---|---|
| `always` | Current strict behavior: block writes to the main worktree on protected branches. |
| `ask` | Block, and direct the agent to ask the user to choose a destination: a dedicated worktree (recommended), a feature branch in the current checkout, or an explicit protected-branch override. The agent must not self-bypass. |
| `off` | Disable the gate entirely; writes are allowed even on protected branches. |

Default: `always`.

## Gate implementation (`gate_impl`)

The gate ships inside the **single zero-dependency Go CLI**. `ai-specs sync`
materializes a thin bash-3.2 launcher at the unchanged path
`ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh`; there are no separate
gate release assets, cache downloads, or gate checksum trust root. `gate_impl`
accepts `auto` (default) or `go`, both selecting the CLI-provided gate.
`gate_impl = bash` is rejected at sync.

The launcher accepts an executable `WORKTREE_GATE_BIN` per invocation. With
`WORKTREE_GATE_VERIFY=1`, an override that fails `--selftest` is ignored and
falls through to the installed CLI. The old project-local pin and cached
standalone binary are no longer used. Handoff is `exec`, so stdin and the exit
code pass through untouched. If the CLI route is unavailable, the launcher
warns and fails open. The process `$PWD` remains the gate's fallback for an
invalid event cwd.

**Doctor severities (worktree-gate):** `gate_impl = bash` (manifest or stamp) is
ERROR — set `auto` or `go`, then `ai-specs sync`. A leftover
`hooks/worktree-gate-legacy.sh` on disk is INFO (inert; `rm
ai-specs/recipes/worktree-flow/hooks/worktree-gate-legacy.sh`). A missing or
non-executable CLI is ERROR — reinstall ai-specs via `install.sh`. A failing
CLI selftest is ERROR — run `ai-specs upgrade`. Doctor is read-only.

**Recovery:** per invocation, `WORKTREE_GATE_MODE=off` or
`WORKTREE_GATE_BIN=/path/to/binary`. Full revert is install the previous CLI
and run `ai-specs sync`. The gate ships inside the CLI release and is covered
by the CLI trust root; no separate gate binaries are published.

## Topology-aware gate scope

`gate_scope` is independent from both `gate_mode` (whether enforcement runs)
and `repo_topology` (where worktrees are created and cleaned). The hook stamps
both values and accepts a per-invocation `WORKTREE_GATE_SCOPE` override; invalid
overrides or stale stamps warn and fall back safely to `auto`.

| Scope | Protected owner enforced | Policy |
|---|---|---|
| `auto` | Proven superrepo and subrepo | Topology-derived behavior; canonical superrepo planning paths are the only exception. |
| `superrepo` | Proven superrepo only | Subrepo writes are outside this selected enforcement scope; central planning remains the explicit superrepo exception. |
| `subrepo` | Proven initialized subrepo only | Superrepo writes are outside this selected scope for the Melón workflow; this is intentional and explicit. |
Topology classification requires effective `repo_topology=monorepo-submodules`.
Explicit `standalone` or `monorepo-apps` never gains a central bypass merely
because initialized modules are present. For a proven initialized-submodule
topology, the only protected superrepo planning exception is the component-aware
canonical descendant `<superrepo>/openspec/changes/**` (including archive and
nonexistent descendants). Symlink escapes, prefix lookalikes, unrelated
repositories, and ambiguous Git relationships remain blocked or fail open
conservatively. Linked worktrees stay allowed before scope evaluation.
Production authorization remains owned by the `plan-build-flow` gate; a central
plan does not authorize subrepo code writes.

**Delegation caveat:** the gate is a `pre-tool-use` hook. On opencode/pi/omp it
may not see tool calls made inside a delegated subagent/task (separate process
or host gap — see `docs/runtime-hooks.md`). Before dispatching write-capable
subagents, verify worktree and branch yourself; do not rely on the hook alone.

**Shell-write coverage:** the same gate also best-effort blocks shell/bash
commands (`>`, `>>`, `tee`, `sed -i`/`perl -i`, `cp`/`mv`, interpreter
heredoc/`-c` write calls) that would write into the protected main worktree —
closing the gap where an agent falls back to bash after a blocked or errored
Edit/Write. This is a **heuristic, not a sandbox**: obfuscated or multi-stage
writers (`awk`, `dd`, base64-piped content, opaque `bash -c "$(...)"`) can
still evade it by design (fail-open on ambiguity), and coverage is uneven by
harness — see the coverage matrix in `docs/runtime-hooks.md`.

## Config

| Key | Default | Meaning |
|---|---|---|
| `worktrees_dir` | `.worktrees` | Directory that holds per-change worktrees. |
| `integration_branch` | `main` | Branch worktrees are created from and merged into. |
| `auto_remove_merged` | `true` | Whether merged worktrees are eligible for cleanup. |
| `gate_mode` | `always` | Main-worktree gate mode: `always`, `ask`, or `off`. |
| `gate_scope` | `auto` | Scope policy: `auto`, `superrepo`, or `subrepo`; only proven superrepo `openspec/changes/**` planning paths receive an exception. |
| `gate_impl` | `auto` | Gate implementation: `auto` / `go`. |
| `WORKTREE_GATE_SCOPE` | — | Optional per-invocation override of the stamped scope; invalid values warn and fall back safely. |
| `repo_topology` | — | **Deprecated alias** of `[project].repo_topology`, read-only for one migration window. Declare topology on the project: `auto` (initialized `.gitmodules` → `monorepo-submodules`, else `standalone`), `standalone`, `monorepo-apps` (naming-only; same mechanics as standalone), or `monorepo-submodules`. |
| `WORKTREE_GATE_PROTECTED` | `main development` | Space-separated branch names where the `worktree-gate` hook blocks Edit/Write in the main worktree. Passed to the rendered hook as the `WORKTREE_GATE_PROTECTED` env var. |


## Repo topologies

| Resolved topology | Create | Clean |
|---|---|---|
| `standalone` | `git worktree add <worktrees_dir>/<slug> …` | Single-repo scan (unchanged) |
| `monorepo-apps` | Same as standalone (naming-only) | Same as standalone |
| `monorepo-submodules` | `git -C <subrepo> worktree add <absolute>/<worktrees_dir>/<subrepo>-<slug> …` | Enumerate each initialized submodule; never superproject `worktree list` alone |

Shared layout: worktrees always live under the **superproject**
`<worktrees_dir>/` (default `.worktrees/`). Under submodules the directory name
is `<subrepo>-<slug>`.

## Cleanup ownership

The old `ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh` launcher
is retired. It was an override, so a user could edit it; `ai-specs sync` now
removes every copy — including a modified one, with a one-line notice — and never
writes it again. Cleanup lives in the CLI verb, whose config comes from the sync
snapshot, so a local edit would have no effect anyway.

Runtime hook scripts follow gate provenance instead of template policy. Sync
records a baseline of the exact bytes the CLI last rendered for each generated
hook (`ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh`):

- baseline matches current bytes → unmodified; sync may force-update the gate
  and re-record the baseline;
- bytes differ from the baseline → user-modified; sync preserves the gate and
  warns;
- no baseline → unknown provenance; sync preserves the gate and warns, and
  records a baseline only when the CLI itself renders the gate.

Runtime hook scripts are no longer rewritten unconditionally.

To explicitly replace a customized gate (after its exact pre-refresh bytes are
saved to a cache-only immutable backup):

```bash
ai-specs sync --refresh-gates
```

or remove the gate and resync: `rm <gate-path> && ai-specs sync`.

After a user-modified warning, re-apply any local customizations to the refreshed
gate as needed.

## Post-merge lifecycle close

Sync writes a managed `.git/hooks/post-merge` hook that is exactly one shebang
plus `exec ai-specs worktree-cleanup`. At a merge boundary it invokes the CLI,
which closes the matching open
`tracker-ledger` item at `archive-close` (matched on the Git common dir + branch,
ignoring any stored change slug) **before** removing a provably merged worktree or
local branch. The close is idempotent and never reopens a closed row (D17), and it
performs no provider or network call — it writes only the local ledger store under
`<git-common-dir>/ai-specs/ledger/`.

The hook is fail-open for the merge itself: it reports cleanup failures on stderr
and exits `0`, so it never changes the already-sealed merge outcome. The
destructive cleanup it triggers fails closed: if the ledger close cannot be
persisted, the candidate worktree/branch is preserved instead of removed. Without
the hook, the direct tracker host
(`tracker-card-gate.sh --root <root> --checkpoint archive-close`) remains the
fallback.

The hook is always regenerated by `ai-specs sync`, and it carries no policy: the
verb reads the project's snapshot, so a customized project never silently falls
back to `.worktrees`/current HEAD during automatic cleanup. A local edit of the
hook is overwritten on the next sync. The hook keeps its git resolution
(`git rev-parse --git-path hooks`), so in a linked worktree it lands in the
shared hooks directory of the main repository.

## Post-merge remote cleanup

GitHub's `delete_branch_on_merge` setting is intentionally false because it is
repo-wide and cannot exempt the long-lived `development` branch. GitHub exposes
no per-PR override. Likewise, `gh pr merge --delete-branch` cannot delete the
local head in this multi-worktree layout when the base branch is already checked
out in the main worktree. Run cleanup as a separate step from the main worktree:

```bash
cd <main-repository-root>
ai-specs worktree-cleanup
```

The Go cleanup command repeats the protected-name check immediately before
worktree removal, local branch deletion, and remote branch deletion. It deletes
a remote only after merge proof and worktree release, then verifies the remote
ref is absent with `git ls-remote --heads <remote> <branch>`. A surviving ref or
verification failure is reported as a non-success; cleanup never claims remote
removal without this proof.

The cleanup implementation lives in the CLI (`worktree-gate --cleanup`), and its
policy comes from the sync snapshot.

## Worktree ledger port

Cleanup classification is a **domain port** of the autonomous Go ledger core, not an
ad-hoc merge check in the actuator. `ledger.EvaluateWorktree` is a pure evaluator over
one normalized observation — `detached`, `dirty`, `localMerged`, `prMergeCommit`,
`mergeCommitInBase` — returning one outcome (`detached` / `dirty` / `merged` /
`unmerged`) in safety order: detached, then dirty, then merge proof. It never shells
out, reads the clock, touches the filesystem, or calls a provider, and it reuses no
Tracker witness, store, or checkpoint. The Worktree and Tracker ports are separate;
no universal artifact schema is introduced.

The accepted observations are pinned by a JSON golden corpus under
`gate/ledger/testdata/worktree-ledger-corpus/` (detached, dirty, local merged, PR merge
commit in base, PR commit outside base, no evidence). Loading it from Go and requiring
at least one preserve and one merged case keeps the corpus from passing vacuously.

**Acquisition stays in cleanup.** `cleanup.go` is the single Go actuator: it gathers
git facts, feeds the port a normalized observation, and owns every destructive check
(protected branches, worktree-held branches, remote-deletion ordering, and the
tracker-ledger close before removal). The optional provider seam is **read-only**:
after local ancestry/patch/tree proofs are inconclusive, cleanup may run
`gh pr list --head <branch> --state all --json mergeCommit` and accept a merge commit
only when it is reachable from an already-resolved local base candidate. Base
resolution never fetches or touches the network, and the seam fails closed — a
missing, failing, or malformed `gh` yields no evidence, so the candidate is preserved.
Cleanup performs no provider API (create/update/move/comment/label) mutation; its
destructive actions remain the host git operations and run only after a proven merge.

## Cleanup contract

| Worktree state | Action |
|---|---|
| Branch merged into base (regular **or** squash/rebase) or PR merge commit proven reachable from a resolved base candidate, clean | removed |
| Uncommitted changes | preserved (`dirty`) |
| Branch not merged | preserved (`unmerged`) |
| Main / detached HEAD | never touched |

Squash/rebase merges are detected by patch-id (`git cherry`), since the squashed
commit is not an ancestor of the base branch. When no local proof succeeds, cleanup
may fall back to the read-only `gh pr list` merge-commit evidence described above;
missing or out-of-base evidence preserves the candidate.

Run with `--dry-run` to preview before removing anything.
