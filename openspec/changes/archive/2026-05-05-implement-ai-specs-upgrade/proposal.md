# Proposal: implement-ai-specs-upgrade

## Why (motivation)

Today the only documented way to update the global `ai-specs` CLI is to re-run the
full `install.sh` curl pipe or to manually `cd ~/.ai-specs && git pull`. Neither
is ideal:

- Re-running the installer is opaque: users cannot see what will change before
  it happens.
- A manual `git pull` is error-prone (dirty tree, detached HEAD, local commits)
  and assumes the user knows where the repo lives.
- There is no way to tell whether the current shell is using a stable global
  install (`~/.ai-specs`) or a local development checkout (`ai-specs-dev`).
- No version diff is shown, so users cannot verify that an upgrade actually
  occurred.

A first-class `ai-specs upgrade` command fixes all of these: it is discoverable,
read-only until the user confirms, detects the installation channel automatically,
and prints a clear before/after version report.

## What Changes (scope)

1. **New CLI subcommand** `upgrade` dispatched from `bin/ai-specs`.
2. **New library script** `lib/upgrade.sh` implementing the upgrade logic.
3. **README update** documenting:
   - When to use `ai-specs upgrade` vs `install.sh`.
   - The difference between the stable channel and the `ai-specs-dev` local-dev
     channel.
4. **Test coverage** in `tests/test_upgrade.py` exercising:
   - Detection of a valid global install.
   - Detection of a missing / broken install.
   - Simulation of a successful upgrade (via a local test clone).
   - Refusal to upgrade a dev checkout.

Out of scope:
- Self-update via curl (the command delegates to `git`, just like `install.sh`).
- Downgrade or pinned-version upgrades (always fast-forwards to `main`).
- Windows-specific logic (the CLI is already bash/git-centric).

## Capabilities (new/modified)

### `ai-specs upgrade`

| Capability | Description |
|------------|-------------|
| **Detect installation** | Inspect `AI_SPECS_HOME`, the resolved symlink in `$PATH`, and `~/.local/bin/ai-specs` to determine whether this is a global install or a dev checkout. |
| **Detect dev channel** | If the resolved binary lives outside `~/.ai-specs` (e.g. `ai-specs-dev` symlink or a local clone), abort with a message telling the user to pull manually. |
| **Pre-flight checks** | Verify the directory is a git repo, `origin/main` is reachable, and the working tree is clean (or warn and require `--force` to proceed). |
| **Fast-forward pull** | `git fetch origin main && git merge --ff-only origin/main`. On failure, stop with actionable guidance. |
| **Post-flight verification** | Re-check the symlink still points to the upgraded `bin/ai-specs`; print old → new version diff. |
| **Dry-run mode** | `ai-specs upgrade --dry-run` prints what *would* change without touching the repo. |

### `bin/ai-specs`

- Add `upgrade)` case in the dispatcher.
- Update the inline help text and the `help` subcommand output.

### `README.md`

- Replace the sparse "Safe re-install / upgrade" section with a concise guide:
  - `ai-specs upgrade` for day-to-day updates.
  - `install.sh` for first-time installs or recovering a broken installation.
  - `ai-specs-dev` workflow for contributors (keep it separate from the stable
    symlink).

## Impact (affected modules, tests, rollback plan)

### Affected files

| File | Change |
|------|--------|
| `bin/ai-specs` | Add `upgrade` dispatch + help text. |
| `lib/upgrade.sh` | New script with detection, pre-flight, pull, and verification logic. |
| `README.md` | Rewrite "Safe re-install / upgrade" section. |
| `tests/test_upgrade.py` | New unittest file covering the upgrade path. |

### Test plan

- Unit-level: mock git state in a temporary clone to assert detection logic.
- Integration-level: run `ai-specs upgrade --dry-run` against the temp clone and
  verify stdout contains the expected version diff.
- End-to-end: run the full upgrade on a temp clone and assert the VERSION file
  changes (we can achieve this by creating a local bare repo with two commits).

### Rollback plan

1. **Git-level rollback**: `git -C "$AI_SPECS_HOME" reset --hard HEAD@{1}`
   restores the previous state because the upgrade is a pure fast-forward.
2. **Symlink-level rollback**: if the symlink was broken, `install.sh` recreates
   it idempotently.
3. **Worst case**: delete `~/.ai-specs` and re-run `install.sh` (always safe,
   projects do not live inside `AI_SPECS_HOME`).
