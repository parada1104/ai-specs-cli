# Worktree Flow recipe

Isolated git worktrees under `.worktrees/` for ai-specs change work, with safe
post-merge cleanup.

## What it provides

- **Skill `worktree-flow`** — when to create a worktree (file-writing work) vs.
  stay outside one (pure exploration), naming conventions, and cleanup rules.
- **Commands `/worktree-new`, `/worktree-clean`** — agent-facing flows to create
  a worktree for a change and to reclaim merged worktrees.
- **Script `bin/worktree-cleanup.sh`** — conservative cleanup: removes only
  merged + clean worktrees, preserves dirty and unmerged ones, never touches the
  main worktree.

## Enable

```toml
[recipes.worktree-flow]
enabled = true
version = "1.5.0"

[recipes.worktree-flow.config]
worktrees_dir = ".worktrees"
integration_branch = "main"
auto_remove_merged = true
repo_topology = "auto"
gate_scope = "auto"
gate_impl = "auto"
```

Then run `ai-specs sync`. The cleanup script materializes to
`ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh`.

## Worktree-gate modes

`worktree-flow` also gates writes to the main worktree on protected branches via
`gate_mode`:

| Mode | Behavior |
|---|---|
| `always` | Current strict behavior: block writes to the main worktree on protected branches. |
| `ask` | Block, but surface a bypass hint: rerun with `WORKTREE_GATE_MODE=off` for that one invocation. |
| `off` | Disable the gate entirely; writes are allowed even on protected branches. |

Default: `always`.

## Gate implementation (`gate_impl`)

The gate ships as a **single zero-dependency Go binary** (the implementation of
record) plus a **frozen Bash reference** (`worktree-gate-legacy.sh`) kept for
one minor release as the rollback path. `ai-specs sync` materializes a thin
bash-3.2 launcher at the unchanged path
`ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh` and, when the binary is
wanted, acquires it into the version-keyed cache:

```
$AI_SPECS_HOME/cache/bin/worktree-gate/<cli-version>/<goos>-<goarch>/worktree-gate
```

| `gate_impl` | Behavior |
|---|---|
| `auto` (default) | Prefer the Go binary; fall back to the frozen Bash reference when no binary is usable. |
| `go` | Go binary only; when none is usable the gate fails open and `ai-specs doctor` reports an ERROR. |
| `bash` | Frozen Bash reference only; no binary, network, or Go toolchain required. |

The launcher resolves an implementation in order: `$WORKTREE_GATE_BIN` →
project-local `ai-specs/recipes/worktree-flow/bin/worktree-gate` → version-keyed
cache → frozen Bash reference (`auto`/`bash`) → one stderr warning and exit `0`
(fail open). Handoff is `exec`, so stdin and the exit code pass through
untouched; the gate never computes a digest on the invocation path unless
`WORKTREE_GATE_VERIFY=1` requests it.

**Offline behavior:** with `gate_impl = auto` and no cached binary, `ai-specs
sync` warns and the launcher falls back to the frozen Bash reference — the gate
keeps enforcing with no network. With a Go toolchain present, set
`AI_SPECS_GATE_BUILD=1 ai-specs sync` (or run offline with `go` installed) to
build the binary from the in-repo source into the same cache layout; a Go
toolchain is a contributor prerequisite only, never a user prerequisite.

**Rollback levers:** set `gate_impl = "bash"` and sync to pin the frozen Bash
reference (works fully offline); or set `WORKTREE_GATE_BIN=/path/to/binary` per
invocation to force a specific binary. `ai-specs doctor` reports the resolved
implementation, binary version, digest state, and any silent fallback
(`worktree-gate` check; OK / INFO / WARN / ERROR per the severity table in
`docs/runtime-hooks.md`). If the gate is not enforcing, `ai-specs doctor`
surfaces it as an ERROR.

**Digest trust root:** the expected SHA-256 of every published asset is
committed at `catalog/recipes/worktree-flow/bin/SHA256SUMS`; a downloaded
binary is verified against it before install and is deleted (never executed) on
mismatch. Binaries are never committed to the repository.

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
| `gate_impl` | `auto` | Gate implementation: `auto` (prefer Go binary, fall back to Bash), `go` (binary only, fail open when unusable), or `bash` (frozen Bash reference; no binary/network/Go required). |
| `WORKTREE_GATE_SCOPE` | — | Optional per-invocation override of the stamped scope; invalid values warn and fall back safely. |
| `repo_topology` | `auto` | Repository topology: `auto` (initialized `.gitmodules` → `monorepo-submodules`, else `standalone`), `standalone`, `monorepo-apps` (naming-only; same mechanics as standalone), or `monorepo-submodules`. |
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

## Stale cleanup override

The cleanup script uses `condition = "not_exists"` and is a governed template.
Sync records the bytes it last wrote in `[managed.*]` in
`ai-specs/.ai-specs.lock`, then classifies the target on later runs:

| State / policy | Sync behavior |
|---|---|
| Managed current | Leave unchanged and stay quiet. |
| Managed stale + `auto` (default) | Refresh from the catalog and update the lock. |
| Managed stale + `confirm` or `never-force` | Preserve, warn, and defer to explicit refresh. |
| User-modified or untracked custom | Preserve, warn, and never force an overwrite. |

To explicitly discard local content and seed a fresh managed copy:

```bash
rm ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh
ai-specs sync
```

Runtime hook scripts are outside the override surface and are always rewritten
by the CLI during sync. The policy applies to governed template overrides only.

After a user-modified warning, re-apply any local customizations to the refreshed
template as needed.

## Cleanup contract

| Worktree state | Action |
|---|---|
| Branch merged into base (regular **or** squash/rebase), clean | removed |
| Uncommitted changes | preserved (`dirty`) |
| Branch not merged | preserved (`unmerged`) |
| Main / detached HEAD | never touched |

Squash/rebase merges are detected by patch-id (`git cherry`), since the squashed
commit is not an ancestor of the base branch.

Run with `--dry-run` to preview before removing anything.
