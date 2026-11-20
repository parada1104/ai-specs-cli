---
name: dev-sync
description: >
  Sync this repo's own ai-specs project with the CLI built from the current
  development branch instead of the stale installed release. Use when asked to
  "sync this repo with the development build", "ai-specs sync con la versión
  development", or "sync after git pull of development".
license: MIT
metadata:
  author: ai-specs
  version: "1.0"
  scope: [root]
  auto_invoke:
    - "Syncing this repo's ai-specs project with the development build"
    - "Running ai-specs sync after git pull of development"
---

# Dev Sync

Refresh `ai-specs/` (lock, AGENTS.md, generated agent dirs, skill fan-out) for
this project using the CLI built from `development` in this checkout.

## Why the dev build

The `ai-specs` on `PATH` is a released binary and can be stale (0.24.5 has been
observed failing on this project). The repo's own launcher is the only safe
entry point: `./bin/ai-specs` resolves `$AI_SPECS_GO_BIN` before
`$AI_SPECS_HOME/dist/ai-specs`.

Running a freshly built binary directly fails with `template not found`: the
binary looks for templates next to itself. Always go through `./bin/ai-specs`
with `AI_SPECS_GO_BIN` pinned.

## Procedure

Run from the **main checkout** (never a `.worktrees/*` copy — its `development`
is what you want to fast-forward):

```bash
main="$(git worktree list --porcelain | awk '/^worktree /{print $2; exit}')"
cd "$main"

# Only switch if the tree is clean; never stash or discard user work.
git status --short          # must be empty before the next two lines
git checkout development
git pull --ff-only
```

Build and sync:

```bash
bin_dir="$(mktemp -d)"
go build -o "$bin_dir/ai-specs" ./cmd/ai-specs
AI_SPECS_GO_BIN="$bin_dir/ai-specs" ./bin/ai-specs sync
```

Then inspect the result (next section) and verify before removing `$bin_dir`.

## Reading the result

- **`AGENTS.md` left unchanged (user_modified …)** — the brief has local edits.
  `ai-specs sync --adopt-brief` only records the current file as baseline; a
  second plain `sync` then renders it. Do that only when the intent is to
  accept the local brief as the new baseline.
- **`ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh`
  refreshed** — this user override is not part of a dev refresh. Restore it and
  reset the matching `[managed."…worktree-cleanup.sh"]` sha in
  `ai-specs/.ai-specs.lock` to the `HEAD` value (`git checkout -- <path>` and
  `git checkout --` the lock if only that stanza moved).
- Generated agent dirs (`.claude/`, `.cursor/`, `.opencode/`, `.omp/`,
  `.mcp.json`, `CLAUDE.md`, `cache/`) are gitignored output — never stage them
  because a sync produced them (see `dogfood-verification-isolation`).

## Delivering the refresh

Writes to protected `development` in the main worktree are blocked by the
worktree gate. Pick one, with the user:

- **Local refresh only** — leave the tracked changes (`AGENTS.md`,
  `ai-specs/.ai-specs.lock`) uncommitted in the main checkout.
- **Deliver it** — make the sync in a dedicated worktree/branch and open a PR
  into `development`.

## Verify

```bash
AI_SPECS_GO_BIN="$bin_dir/ai-specs" ./bin/ai-specs sync   # exits 0
git status --short                                       # unchanged by the 2nd run
rm -rf "$bin_dir"                                         # scratch build dir
```

For the project's Python tests (no pytest installed):

```bash
PYTHONPATH=tests "$(mise which python3)" -m unittest
```

## Non-goals

- No edits to `lib/`, `cmd/`, or `bin/` — this skill uses the CLI, it does not
  change it.
- Never stage, commit, push, or `git pull` with a dirty tree.
