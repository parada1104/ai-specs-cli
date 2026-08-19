# Orchestration Handoff — Go Single-Binary Migration

> **Read this first.** You are the canonical orchestrator for the Go migration epic,
> running from the worktree `.worktrees/epic-go-single-binary` on branch
> `epic/go-single-binary`. This document is the complete context transfer.
> Written 2026-08-18.

## Your role

You **coordinate**. You do not write card code.

Per the `orca-aware-delegation` skill (local, resolved in `.claude/skills/`), the split is:

| You own | Workers own |
|---|---|
| Decomposition, dispatch decisions | Change content inside their assigned worktree |
| Verification and integration | Nothing else |
| **Staging, commits, merges** | **Never stage, commit, push or merge** |
| Worktree lifecycle | Never create/remove/reassign a worktree |
| Run/Task/Dispatch lifecycle through release | Report `worker_done` exactly once |

## Non-negotiable rules

1. **Never launch a worker headless.** No `pi -p`, no `claude -p`, no `opencode run`, no provider API calls. Workers launch as **visible interactive TUI sessions** through `orca orchestration worker-start`. This is an explicit user instruction and a hard rule in the skill.
2. **`pi` is the worker agent.** It is the one the user has confirmed works reliably and has headroom. Claude has usage limits here.
3. **`worker_done` never implies terminal closure.** Never call `worker-release` automatically. Ask the human retain-or-close, and use `worker-retain` when they choose retain.
4. **Nothing reaches `development`.** Card PRs target `epic/go-single-binary`. Auto-merge to the epic is authorized. Any PR to `development` stays open for the human to review.
5. **Never "fix" behavior classified FROZEN** in `docs/go-migration-parity-contract.md`, even when it looks wrong. Recorded defects are separate cards.

## Where things stand

### Done

| Item | Evidence |
|---|---|
| Epic + 16 cards created | https://trello.com/c/qwlHQ7Xa |
| Integration branch `epic/go-single-binary` | pushed, tracking origin |
| `base_branch` scoped to this branch | `cf973d2` — travels with the branch, `development` unaffected |
| gh account preflight configured | `expected_owner`, `auto_switch_account` — was never configured before, which is why PR creation failed on permissions |
| Card 01 delivered | `docs/go-migration-parity-contract.md`, PR #227 open against this branch |
| 4 defect cards filed | see below |
| Epic contract encoded in the brief | `9960745` — `AGENTS.md` opens with the branch warning |
| Harness provisioned | 22 skills fanned out to claude/cursor/opencode/pi/omp |

### Blocking, unresolved

**The baseline test run has not completed.** Do not dispatch workers until you have a green
baseline of `./tests/run.sh` on this branch. Without it you cannot tell a test a worker broke
from one that was already broken, and the entire auto-merge criterion rests on that distinction.

Run it and capture the real exit code — do not pipe to `tail`, which masks it:

```bash
cd .worktrees/epic-go-single-binary
./tests/run.sh > /tmp/baseline.log 2>&1; echo "EXIT: $?"
```

If it is red, the failures are pre-existing. Record which ones, and treat that list as the
known-bad set rather than blocking the epic on them.

## The work queue

### Tranche 1 — 4 defect cards, parallel, auto-merge to the epic

Discovered by card 01. Two of them **must** land before card 03, or the parity harness will
freeze the defect as correct baseline behavior.

| Card | URL | Priority |
|---|---|---|
| Manifest/lock writes lose data | https://trello.com/c/lVB8gEin | **D1 before card 03** |
| Commands report false success, clobber user work | https://trello.com/c/sbYplwWF | **D5 before card 03** |
| Vendored deps unreproducible | https://trello.com/c/DflM3ppP | normal |
| Docs contradict behavior | https://trello.com/c/uKisnwrK | normal |

**File collisions** — they are not independent. `sbYplwWF` and `uKisnwrK` both touch `hub.py`;
`DflM3ppP` and `uKisnwrK` both touch `skills-list.sh`. Isolated worktrees prevent collision
during work, not at merge. Merge sequentially with rebase and resolve conflicts yourself.

**The TDD trap.** These cards' acceptance criteria are prose, and the verifying tests do not
exist yet — writing them is part of the card. If a worker writes both the test and the fix, the
test adapts to the fix instead of to correct behavior. Mitigation, required in every brief:
**the worker writes the failing test first, runs it, and reports the RED output before
implementing.** You verify the RED is genuine before accepting the GREEN.

### Tranche 2 — card 03, parity harness

https://trello.com/c/jV2WUOGq — after tranche 1. Single worker.

### Tranche 3 — card 02, test conversion — **needs the human**

https://trello.com/c/POh1vmd6 — 73 of 103 test files are coupled to Python via
`spec_from_file_location`. Converting them requires deciding *what intention each assert was
capturing*. This is the one place where getting it wrong poisons the harness everything else
depends on. Do not fully automate it.

### Tranche 4 — cards 04–15, the bulk

Critical path `04 → 05/06 → 07 → 09/10 → 13`; cards 08, 11, 12, 14, 15 run off it. With the
harness green, "does it work?" is binary, so auto-merge is safe here.

### Tranche 5 — card 16, cutover — **the human's**

https://trello.com/c/RexnLFr8 — includes the blocking `base_branch` revert and the single
promotion PR to `development`.

## Delegation procedure

Verified working. Orca 1.4.184, orchestration enabled, `orca` on PATH (macOS — no
`ORCA_CLI_COMMAND` or `ORCA_DEV_REPO_ROOT` set).

### 1. Hydrate a worktree per card — before any Orca call

```bash
# from the canonical checkout, NOT from inside a worktree
git worktree add .worktrees/<card-slug> -b change/<card-slug> epic/go-single-binary
cd .worktrees/<card-slug>
git status --short                      # PRE-SYNC baseline — capture it
ai-specs sync .                         # full sync, never sync-agent
git status --short                      # POST-SYNC baseline — capture it
```

The diff between the two baselines is **provisioning output**, not worker content. On this repo
it is reliably `AGENTS.md`, `ai-specs/.ai-specs.lock`, and
`ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh`. Revert those before staging
anything. `AGENTS.md` is the blind spot: it is generated *and* tracked, so it is commit-eligible
by accident. See the `dogfood-verification-isolation` skill.

### 2. Bind a Run and create Tasks

```bash
orca orchestration run-create --objective "Go single-binary migration — tranche N" --json
orca orchestration task-create --spec "<full card brief>" --json
```

`task-list` errors with `run_required` until a Run is bound. That error means orchestration is
working, not broken.

### 3. Start workers — TUI, existing worktree

```bash
orca orchestration worker-start --task <task_id> --worktree path:/absolute/path/to/.worktrees/<card-slug> --agent pi --json
```

Pass the **existing** hydrated path. `worker-start` reuses it and rejects `--setup`, so omit
`--setup` — setup reports `not_applicable`. Never ask Orca to create a second worktree for a
change that already has one.

Start every independent worker **before** waiting on any of them.

### 4. Wait, verify, integrate

```bash
orca orchestration check --wait --types worker_done,escalation,question --timeout-ms 900000 --json
orca orchestration worker-show --dispatch <dispatch_id> --json
orca orchestration worker-read --dispatch <dispatch_id> --limit 50 --json
```

`ready` or `input_accepted` is **not** proof the worker did the work. Require a real
`worker_done` plus evidence.

Then you — not the worker — revert provisioning paths, stage only worker-owned paths, commit,
open the PR against `epic/go-single-binary`, verify, and merge.

### 5. Retain or release — ask the human

```bash
orca orchestration worker-retain --dispatch <dispatch_id> --json    # human said keep it
orca orchestration worker-release --dispatch <dispatch_id> --json   # human said close it
```

Never release as an automatic consequence of `worker_done`.

## What every worker brief must contain

1. **Epic contract** — base branch `epic/go-single-binary`; never touch `development`; no new Python in `lib/_internal/`, no new Bash logic in `lib/`, no new vendored Python.
2. **Boundary** — work only inside the assigned worktree; never stage, commit, push, merge, or manage worktrees.
3. **Verify the root before the first write** — `git rev-parse --show-toplevel`, `git branch --show-current`, `git worktree list`. Hooks are defense in depth only and have known gaps on Pi/OMP subprocess calls.
4. **Card scope and acceptance criteria**, copied from Trello.
5. **RED-first requirement** — write the failing test, run it, report the RED output, then implement.
6. **Verification command** that must pass: `./tests/run.sh`.
7. **What to return** in `worker_done`: what changed, findings, what remains, files modified.

## Environment facts worth not rediscovering

- `gh` has two accounts. `parada1104` is the repo owner with ADMIN; `rparada1104` only has READ. If `gh pr create` fails with `must be a collaborator`, run `gh auth switch --user parada1104`.
- The user works on `development` **in parallel**. Never disturb it. It still reads `base_branch = "development"` and must keep doing so.
- Full suite is `./tests/validate.sh`; unit-only is `./tests/run.sh`. The unit suite alone takes many minutes.
- Piping a test run to `tail` masks the exit code — `$?` reports `tail`'s status. Redirect to a file instead.
- The shell working directory resets to the repo root between tool calls. Put `cd <worktree> &&` in the same command.

## Open decisions

| Decision | Status |
|---|---|
| Auto-merge to the epic | **Authorized** by the user |
| PRs to `development` | Stay open; the user reviews them |
| Retain-or-close per worker | **The user's**, every time |
| Card 02 automation depth | Unresolved — needs the user |
