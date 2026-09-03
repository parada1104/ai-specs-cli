# Change: gate-cwd-fidelity

## Why

The worktree-flow shell gate produces false-positive blocks for commands that execute in a worktree but are invoked from a session whose process cwd is the main checkout. Verified live on 2026-09-02:

- `git -C <worktree> mv <relative-planning-paths>` blocked with reason `protected-branch` because the adapter reports `cwd: process.cwd()` (main checkout, branch `development`) while the command writes inside `change/gate-cwd-fidelity` worktree.
- `--explain` with the same command but event cwd set to the worktree → exit 0 (allowed).
- Absolute worktree paths in the command → allowed, proving the gate resolves candidates correctly when the path itself carries the context.

## What Changes

**Root cause (re-framed).** The gate infers write destinations by absolutizing candidate paths against the **event cwd**. But the event cwd is `process.cwd()` of the host session; it does **not** carry the cwd of the command being gated. For `git -C <wt> ...` and compound `cd X && ...`, the writable cwd lives in the **command text**, not in the event cwd. The adapters (Pi/OMP) correctly keep `process.cwd()` — and this is **pinned by spec** (`openspec/specs/workspace-context` requires Pi/OMP to keep process.cwd-only events and NOT claim a workspace root). So the fix is not to make the adapter report a "better" cwd; that would break a frozen contract and still cannot derive the write cwd from the host process.

**Selected direction (Alt 2 — gate degrades honestly).** The gate must recover the cwd that the **command itself** determines — `git -C <dir>` and `cd <dir> && ...` — and use that as the authoritative base for `Decide`. When the effective cwd is **not recoverable** (neither event cwd nor command text yields an existing writable root) and the candidate path is relative, the gate must **degrade to ask/warn** instead of block-on-guess against the host `process.cwd()`. That stops false positives, keeps block-on-trust where the cwd is honest, and does not touch the frozen workspace-context contract.

**Wrong-exit correction.** The current block message (`message.go::BlockMessage`, byte-identical to the legacy `worktree-gate-legacy.sh`) says "Create a dedicated worktree first (e.g. /worktree-new)". In this scenario the user is **already** in a worktree; the exit is wrong. It must name the actual command cwd (the `git -C`/`cd`/worktree destination) instead of insisting on creating another worktree.

## Capabilities

### New Requirements

- **Requirement: worktree-flow shell-gate cwd fidelity contract** — the gate determines the effective cwd from the command itself (`git -C`, `cd X &&`) before absolutizing candidates; where the effective cwd is not recoverable and the path is relative, it degrades honestly (ask/warn) instead of block-on-guess.
- **Requirement: correct block exit message** — the protected-branch block must name the actual command worktree cwd instead of a generic "create a dedicated worktree" hint.

### Modified Requirements

- (none)

### Design constraints (from explore)

- Do **not** change the Pi/OMP event cwd semantics: `openspec/specs/workspace-context` + `tests/test_hooks_render.py::_assert_process_cwd_event` pin process.cwd-only events. Changing that breaks the frozen spec/test and cannot derive the write cwd anyway.
- The worktree-flow gate Go binary is shared by both hook surfaces (`worktree-gate.sh` launcher serves `worktree-gate` and `worktree-gate-shell`). Fixing the gate covers both in one source.
- `stamped_gate_version`/doctor version-drift check (`doctor.py::_check_worktree_gate`) is **already implemented** and orthogonal — not part of this change.

### Impact

- `catalog/recipes/worktree-flow/gate/main.go` (+ `decide.go`, `gitfacts.go`, `tokenize.go` + tests)
- `catalog/recipes/worktree-flow/gate/message.go` (wrong-exit text)
- `lib/_internal/hooks-render.py` (do **not** change the process.cwd() semantics — leave as-is)
- `.pi/extensions/*.ts` (regenerated via sync, but cwd semantics unchanged)
- `ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh` (materialized, unchanged)

## Tracker

- card_id: `6a97c05e80015ef9ef90fb6c`
- url: https://trello.com/c/VkRZdgU6
