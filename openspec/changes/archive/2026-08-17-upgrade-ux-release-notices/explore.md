# Exploration: upgrade output, release notices, and install footprint

## Trigger

The `0.21.0 -> 0.22.0` upgrade printed 243 file names, a raw `git fetch`
transfer log, and a fast-forward diffstat. The only two lines a user could act
on were the last two (`Upgraded: 0.21.0 -> 0.22.0`, symlink verified), buried
under ~250 lines of git output.

## What `ai-specs upgrade` actually does

`lib/upgrade.sh` is not a thin `git pull` wrapper. Before touching anything it:

| Check | Line | Exit code |
|---|---|---|
| `AI_SPECS_HOME` is set | 82 | 1 |
| Resolved binary lives inside `~/.ai-specs` (dev-channel guard) | 88 | 2 |
| `AI_SPECS_HOME` matches the expected path | 93 | 1 |
| `~/.ai-specs/.git` exists | 98 | 1 |
| `~/.local/bin/ai-specs` is a symlink resolving inside `~/.ai-specs` | 103, 129 | 1 |
| `HEAD` is an ancestor of `origin/main` (divergence guard) | 149, 211 | 3 |
| Working tree clean, with mode-only dirt auto-remediated | 171-182 | 3 |
| Post-upgrade symlink integrity re-verified | 258-279 | 5 |

The mutation itself is two lines: `git fetch origin main` (206) and
`git merge --ff-only origin/main` (224).

**Conclusion: the logic is sound. The defect is purely presentational — nothing
captures git's stdout/stderr.**

## The pattern already exists in this repo

`sync` had the identical problem and it was fixed in #166. `lib/sync.sh:119`
defines `run_step`:

- prints one `  syncing <label>` line,
- captures stdout and stderr to temp files,
- dumps them **only** on failure,
- `-v/--verbose` restores full detail,
- a failing step always prints its full unfiltered output.

`lib/sync-agent.sh:235` carries the same helper. `upgrade.sh` never received
the treatment. This change is mostly *applying an established in-house
convention to the one command that was skipped*, not inventing an approach.

## Why a version-keyed notice is a real capability, not decoration

Some releases require a post-upgrade action that no diffstat can express.
`0.22.0` is the concrete case:

- the worktree gate became a Go binary acquired at `ai-specs sync` time;
- a user who upgrades and never syncs stays on the Bash fallback silently;
- `ai-specs doctor` reports that state as ERROR ("gate is silently failing
  open"), but only if the user thinks to run it.

There is a second, narrower action: `ai-specs sync --refresh-gates`. It applies
**only** when sync preserved a *customized* gate hook.
`lib/_internal/recipe-materialize.py:572` states `refresh=True` is "never set by
ordinary sync", and `lib/_internal/doctor.py:1075` already emits it as guidance.

### Decisive constraint

`upgrade` runs against `~/.ai-specs`. It has **no consumer project in scope** —
it cannot know whether any given project has a customized gate. Therefore a
notice cannot carry conditional logic; it must be unconditional prose that
points at the command which *does* have project state (`doctor`, `sync`).

This constraint is what keeps the feature small: no evaluation engine, no
project scanning, no new state.

## Install footprint

`install.sh:88` runs `git clone --branch "$AI_SPECS_REF"` with no `--depth`,
no `--filter`, and no sparse-checkout. The full tree lands in `~/.ai-specs`.

Runtime reads of `$AI_SPECS_HOME`, counted across `lib/` and `bin/`:

| Subtree | References |
|---|---|
| `lib` | 41 |
| `VERSION` | 4 |
| `bundled-skills` | 3 |
| `templates` | 2 |
| `cache` (generated, not tracked) | 2 |
| `catalog` | 1 |
| `.git` | 1 |

Measured against `origin/main` (1842 tracked files, 16131 KiB):

| Subtree | Files | Size | Needed at runtime |
|---|---|---|---|
| `openspec/` | 647 | 4147 KiB | no |
| `tests/` | 220 | 1655 KiB | no |
| `.github/` | 1 | 5 KiB | no |
| `tmp/` | 1 | 16 KiB | no |

`openspec/` and `tests/` alone are **867 of 1842 files (47%) and 35% of tracked
bytes**. They are also the bulk of what made the 0.22.0 diffstat unreadable, so
narrowing the checkout attacks noise and footprint with one mechanism.

### Shallow clone is the wrong tool

`--depth` truncates history. `upgrade.sh:149` and `:211` both call
`git merge-base --is-ancestor HEAD origin/main` — the divergence guard. Against
a shallow clone that check is unreliable and can abort a legitimate upgrade or,
worse, misclassify divergence.

`--filter=blob:none` (partial clone) keeps the full commit graph, so every
ancestry check keeps working, and skips blob transfer for paths the sparse
checkout excludes. That is the combination this change should use.

### Compatibility floor

- `--filter=blob:none` requires Git 2.19+.
- `git sparse-checkout` cone mode requires Git 2.25+.

Both must degrade to today's full clone rather than fail. Existing full installs
must be narrowable idempotently, and narrowing must never be a precondition for
a successful upgrade.

## Open questions carried into the proposal

1. Where are notices authored so the release ritual cannot forget them?
2. How does an existing full install migrate to a narrowed one, and is that
   migration reversible?
3. Does anything read `~/.ai-specs/openspec/**` at runtime that the reference
   count above did not surface (for example, a skill resolving a spec path)?

## Out of scope

- Runtime-brief / `AGENTS.md` ownership (tracked as a separate change).
- Changing the release channel from git checkout to packaged artifacts.
- Any change to the upgrade safety checks themselves.
