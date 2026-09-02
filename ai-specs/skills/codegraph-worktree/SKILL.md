---
name: codegraph-worktree
description: >
  Initialize and refresh the CodeGraph symbol index for the current worktree or
  workspace. Trigger: entering or creating a worktree where code will be edited,
  after large refactors, or when codegraph query/explore results look stale for
  files changed on a branch.
license: MIT
metadata:
  author: ai-specs
  version: "1.0"
  scope: [root, runtime]
  auto_invoke:
    - "Starting work in a fresh worktree where code will be edited"
    - "Reindexing code after a large refactor on a branch"
    - "Codegraph query or explore returned stale or missing symbols"
---

# CodeGraph Worktree Index

## Why this exists

CodeGraph indexes **per workspace**: the `.codegraph/` index is machine-local and
git-ignored, so `git worktree add` never brings it over. A fresh worktree has **no
index**, and the main checkout's index does **not** cover worktree paths — querying it
from a worktree session silently misses files created or changed on the branch.

Verified facts (2026-09-02, ai-specs-cli):

- `.codegraph/.gitignore` (written by the tool) ignores everything except itself:
  the database, daemon files, and logs are never committed.
- The Pi-native `codegraph` tool (registered by gentle-pi's `codegraph-tools.ts`
  extension) resolves the index from the current workspace only and never accepts
  external paths or shell commands.
- If the `codegraph` binary is missing, the tool reports `unavailable` and falls back
  to `read` / `grep` / `find` — install it with `npm install -g @colbymchenry/codegraph`.

## When to run `codegraph init`

| Situation | Action |
|---|---|
| Fresh worktree created; you will edit code there | Run `codegraph init` in the worktree root |
| Entering an existing worktree with no `.codegraph/` | Run `codegraph init` before symbol queries |
| Large refactor landed on the branch (new/renamed symbols) | Re-run `codegraph init` to refresh the graph |
| Read-only exploration only | Skip — the main checkout's index is fine |

## How

1. Confirm you are in the right workspace root:
   `git rev-parse --show-toplevel` (must equal the directory you will index).
2. In Pi, use the native tool: `codegraph` with `operation: "init"`. Outside Pi or
   from a script, use the binary: `codegraph init` (run from the worktree root).
3. Verify: `.codegraph/codegraph.db` exists in that worktree.

## Freshness rules (do not skip)

- `query`/`explore` re-read **source** from disk live, but **symbol/call-path data
  (callers, blast radius) is only as fresh as the last index run**.
- For any critical decision based on blast radius ("who calls this?"), corroborate
  with `grep` before acting — grep cannot be stale.
- CodeGraph does not index shell scripts or prose: for `.sh` logic and docs, use
  `grep`/`git log -S` directly.

## Never

- Never commit `.codegraph/` — it is a regenerable, machine-local build artifact
  (the tool's own `.gitignore` enforces this).
- Never run `codegraph init` in `$HOME` or a temp directory — the tool refuses
  (it requires a real Git project root).
- Never treat a stale index as proof of absence: "no callers found" may only mean
  "not re-indexed since the caller was added".
