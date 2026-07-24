---
name: dogfood-verification-isolation
description: >
  Keeps product code changes and this repo's own dogfooded project state
  separate when using the in-progress CLI to smoke-test itself inside a
  worktree.
  Trigger: before running `ai-specs sync`/`init`/`doctor`/`refresh-bundled`
  against this repo's own `ai-specs/` project as a live verification step,
  and before staging/committing anything afterward.
license: MIT
metadata:
  author: parada1104
  version: "1.0"
  scope: [root]
  auto_invoke:
    - "Verifying an ai-specs CLI change by running it against this repo's own dogfood project"
    - "Deciding what to commit after a live sync/doctor smoke test in a worktree"
---

# Dogfood Verification Isolation

## Purpose

This repo is two things at once:

- **The product**: `lib/`, `bin/ai-specs`, `catalog/`, `bundled-skills/`,
  `bundled-commands/`, `tests/`, `openspec/` — the CLI's own source code.
- **A project**: `ai-specs/ai-specs.toml`, `ai-specs/.ai-specs.lock`,
  `ai-specs/commands/`, `ai-specs/skills/` (local ones), `AGENTS.md`, and every
  gitignored generated agent config (`.claude/`, `.cursor/`, `.opencode/`,
  `.omp/`, `.mcp.json`, `CLAUDE.md`, `cache/`) — this repo dogfooding the CLI
  on itself, same as any consumer project would.

Running the in-progress (unreleased) CLI from a worktree against this repo's
own dogfooded `ai-specs/` project is a legitimate, valuable smoke test — real
legacy state beats synthetic fixtures. But its OUTPUT is verification
evidence, not a deliverable of the change.

## The rule

- **Product changes** — anything you wrote by hand under `lib/`, `bin/`,
  `tests/`, `catalog/`, `bundled-skills/`, `bundled-commands/`, `openspec/` —
  belong in the feature branch. Commit them.
- **Project-state changes** — anything the CLI itself generates or mutates by
  running `sync`/`init`/`doctor`/`refresh-bundled` against `ai-specs/` (the
  lock, bundled-managed files under `ai-specs/commands/`, `AGENTS.md`, any
  agent-config output) — are the OUTPUT of *using* the product, not of
  *building* it. They must NOT be committed as part of the feature branch,
  even when the run is real, the behavior is exactly as expected, and it
  would make a nice piece of evidence to point at.
- This repo's own dogfood migration to new CLI behavior happens later, as its
  own ordinary, unrelated act: once the feature is released, a real
  `ai-specs sync` run against the merged code produces that state change, and
  it gets committed on its own terms — never smuggled into the feature PR
  that built the capability.

## How to verify safely inside a worktree

1. Do all live testing inside the change's dedicated worktree
   (`.worktrees/<change>/`) — never in the canonical `development` checkout.
2. Before running the CLI as a smoke test (`./bin/ai-specs sync .`,
   `doctor .`, etc.) against the worktree's own dogfooded `ai-specs/` project,
   confirm `git status --short` is clean.
3. Run the command. Quote the terminal output as verification evidence —
   citing it in `verify-report.md` is exactly the right use of a live run.
4. Run `git status --short` again. Anything now showing is project-state
   output, not something you authored.
5. Revert every one of those files (`git checkout -- <path>` /
   `git restore <path>`) before staging or committing anything else. Confirm
   `git status --short` is clean again.
6. Only `git add`/`git commit` files you deliberately wrote as part of the
   product change.

## The one blind spot: AGENTS.md

Most generated project output is gitignored (`.claude/`, `.cursor/`,
`.opencode/`, `.omp/`, `.mcp.json`, `opencode.json`, `CLAUDE.md`, `cache/`) —
structurally impossible to leak into a commit. `AGENTS.md` is the exception:
it is a generated file that IS tracked in git by design (so a fresh clone has
a usable brief without running sync first). If a product change alters what
`agents-render.py` produces, a verification sync run will regenerate it, and
it becomes commit-eligible like any other tracked file. Always
`git diff -- AGENTS.md` after a verification run before staging anything,
even when you don't expect it to have changed.

## Anti-pattern (do not repeat)

Treating "the sync ran and produced the expected file changes" as proof those
changes belong in the PR. Verification success and shipping eligibility are
separate questions. A passing smoke test earns a citation of its output in
`verify-report.md` — never a commit of its side effects.
