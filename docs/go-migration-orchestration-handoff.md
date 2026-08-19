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
3. **`worker_done` never implies terminal closure by itself.** The human has answered retain-or-close in advance as standing policy: release once the worker completes its change cycle. Release only after an accepted `worker_done` — never on a timeout, idle TUI, heartbeat, question, escalation, or stale report. Retain anything that escalated, asked, or failed pending inspection. See §5 of the delegation procedure.
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

### Baseline — GREEN

`./tests/run.sh` exits **0** on this branch at `cf973d2`. That is the reference every worker's
result is judged against: a failure after a worker's change is that worker's, not pre-existing.
The whole auto-merge criterion rests on this distinction.

Re-establish it yourself if you doubt it, and capture the real exit code — do not pipe to
`tail`, which makes `$?` report tail's status instead of the suite's:

```bash
cd .worktrees/epic-go-single-binary
./tests/run.sh > /tmp/baseline.log 2>&1; echo "EXIT: $?"
```

The unit suite takes several minutes. Run it in the background rather than blocking on it.

## The work queue

### Card 02 — test conversion — **START HERE, ahead of the defect cards**

https://trello.com/c/POh1vmd6

The user reordered this ahead of tranche 1: convert the tests first, so the defect fixes are
verified against a behavioral harness rather than the other way round.

73 of 103 test files are coupled to Python via `spec_from_file_location`. Every assertion in
them falls into one of three categories, and only the first is safely automatable:

1. **Direct observable equivalent** — e.g. a test calling `doctor._check_manifest()` and
   asserting severity `ERROR` becomes: run `ai-specs doctor`, assert `ERROR manifest` in output
   and exit 1. Mechanical. **Automate fully.**
2. **Indirect equivalent** — e.g. a test asserting `_resolve_order()` returns `[a, b, c]`. That
   order is never printed, but it does determine the order files are materialized in. Requires
   inferring which observable effect captures it. **Needs judgement.**
3. **No observable equivalent** — e.g. a test that mocks a function and asserts it was called.
   Nothing about it is visible from outside the process. Options are delete it or approximate
   it. **Needs judgement.**

**The risk is category 3.** Aggressively deleting everything without an observable equivalent
leaves the harness with less coverage than it claims, and cards 07–13 then verify against that
weaker harness. Nobody finds out until something breaks in production weeks later, by which
point the harness is no longer evidence of anything.

**Required protocol, agreed with the user:**

- Convert every category-1 file autonomously. No approval needed.
- Collect categories 2 and 3 into **one single list**, with a per-file justification for the
  proposed treatment (which observable effect replaces it, or why it should be deleted).
- Present that list to the user **once**. They approve it in one pass. Do not drip-feed
  approvals file by file — the user explicitly agreed to one list, not a stream.
- Only after approval, apply the category 2 and 3 decisions.

Success criterion for the card: `./tests/validate.sh` passes against the **unmodified**
Bash/Python implementation, and zero test files still import `lib/_internal` modules via
`spec_from_file_location` or `load_module`.

### Tranche 2 — 4 defect cards, parallel, auto-merge to the epic

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

### Tranche 3 — card 03, parity harness

https://trello.com/c/jV2WUOGq — after the defect cards. Single worker.

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

**No card worktrees exist yet, deliberately.** You create and hydrate every one of them
yourself. The skill puts worktree lifecycle under the canonical orchestrator, so a worktree
created by someone else leaves you governing something you did not provision — and you would be
reading a handed-down baseline instead of the one you captured. The only worktrees that exist
are this one (your home) and `go-parity-contract` (card 01, already in PR #227).

```bash
# from the canonical checkout, NOT from inside a worktree
git worktree add .worktrees/<card-slug> -b change/<card-slug> epic/go-single-binary
cd .worktrees/<card-slug>
git status --short                      # PRE-SYNC baseline — capture it
ai-specs sync .                         # full sync, never sync-agent
git status --short                      # POST-SYNC baseline — capture it
```

Suggested slugs for tranche 1, not created: `fix-manifest-lock-writes`, `fix-false-success`,
`fix-deps-layer`, `fix-docs-drift`.

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

### 5. Release on completion — standing human decision

The human has decided this in advance, as policy: **close the worker once it finishes the
change cycle.** The PR lifecycle — create, verify, merge — is yours, not the worker's, and it
does not need the worker's terminal alive.

```bash
orca orchestration worker-release --dispatch <dispatch_id> --json
```

This is not the agent defaulting to release, which the skill forbids. It is the human's
standing answer to the retain-or-close question. Two limits still apply:

- Release only after an accepted `worker_done`. Never release on a timeout, TUI idle state,
  heartbeat, status, question, escalation, or a rejected/stale report.
- If the worker escalated, asked a question, or failed in a way you have not finished
  inspecting, keep it and use `worker-retain` — the standing decision covers a completed change
  cycle, not an unfinished one.

If release returns `release_pending` or `release_unknown`, follow the exact recovery action in
the receipt. Never substitute `terminal close`.

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
| Retain-or-close per worker | **Decided**: release on completed change cycle; PR lifecycle is the coordinator's |
| Card 02 automation depth | **Decided**: category 1 fully automated; categories 2+3 in ONE list for a single user approval |
