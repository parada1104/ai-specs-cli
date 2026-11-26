# Plan Build as an ODD companion: one mode switch, one plan tree

When Organic Driven Development (ODD) owns a change, Plan Build contributes only
the product contract: a delta for every changed observable rule and its promotion.
When ODD is absent, Plan Build keeps today's OpenSpec-centered flow untouched.

## Tracker

- **Card**: `6b0458aac00e111e539bf60b` — [fFNNuMFY](https://trello.com/c/fFNNuMFY)
- Board: ai-specs-cli Roadmap. Labels: `core`, `repo:ai-specs-cli`, `runtime`, `breaking-risk`.

## Decision recorded

- **Mode per repository**, declared in `ai-specs/ai-specs.toml` under
  `[recipes.plan-build-flow.config]` as `mode = "auto" | "odd" | "openspec"`,
  default `auto`. An explicit value wins over the inference.
- **Inference in `auto`**: the presence of the Gentle AI state directory inside
  the Git common directory (`<git-common-dir>/gentle-ai/`) selects `odd`;
  its absence selects `openspec`. Gentle Shell always drives ODD, so a project
  worked from there is ODD-owned by construction.
- **Plan per change in `odd` mode**: `odd/tasks/<change>.md`, paired with the
  branch name after stripping prefixes (`feat/x`, `fix/x` → `x`). A regular file
  is required; when it is missing the gate blocks with a message naming the path.
  No OpenSpec `tasks.md`.
- **One tree under `odd/`**: canonical specs at `odd/specs/<domain>/spec.md`,
  plan at `odd/tasks/<change>.md`, deltas at
  `odd/changes/<change>/specs/<domain>/spec.md` only when an observable rule
  changes. No `odd/config.yml`.
- **Promotion reuses the existing code**: same composer, same blockers, same
  idempotence; only the source and canonical roots follow the mode.
- **`mode = "openspec"` is unchanged**, byte for byte. Bringing that flow closer
  to ODD is a separate change and is explicitly out of scope here.

## Why the state directory and not another signal

Signals inspected and rejected:

- `.pi/`, `.claude/`, `.cursor/`, `.omp/`, `.opencode/` — `ai-specs sync` writes
  them for any agent target, so they prove an agent is configured, not that
  Gentle Shell is present.
- `AGENTS.md` — generic; Pi, Claude and others read it without any Gentle AI.
- `ai-specs/.ai-specs.lock` hook records — prove ai-specs synced hooks, which a
  non-Gentle project also has.
- `$HOME/.gentle-shell` — describes the machine, not the project, so it would
  mark every repository on that machine as ODD.
- `PI_*` / Gentle environment variables — inherited only by agent subprocesses and
  absent when a human runs `ai-specs gates premerge` from a terminal; not
  verifiable at the point the decision is needed.
- `odd/tasks/*.md` presence — this repository has 154 versioned plan files, so
  presence would satisfy the gate forever and never ask for a plan again.

Chosen signal: `<git-common-dir>/gentle-ai/`. It is created by the native Gentle AI
binary, not by `ai-specs sync` (`grep` for `gentle` in `internal/**/*.go` is
empty), it lives in the common Git directory so it survives every worktree, and a
plain ai-specs project never has it. The gate already shells out for
`rev-parse --show-toplevel`, so one more `rev-parse --git-common-dir` is the same
class of cost.

Known limit: a project with Gentle AI installed but never used for a review has no
state directory yet, so `auto` resolves to `openspec` until the first native run.
The explicit `mode = "odd"` is the answer, and it wins.

## Non-goals

- No change to `mode = "openspec"` behavior, artifacts or tests.
- No `odd/config.yml`.
- No Light/Standard/Full simplification.
- No tracker ledger policy change (see the open points in the task file).
- No migration of the 55 existing canonical domains from `openspec/specs/`.
- No Go changes in the *preparation* session (superseded: the implementation followed, four commits).

## Consequences and risks

- **Writes on a long-lived branch block in `odd` mode.** `development` has no
  branch-derived plan, so a production write there blocks. The escapes are to
  write the plan for that branch or to declare `mode = "openspec"`; both are named
  in the block message.
- **Two canonical trees coexist.** In `odd` mode `odd/specs/` is authoritative and
  `openspec/specs/` is left untouched and unread, so the same domain can exist in
  both with different content. Deliberate, and the migration question is listed as
  an open decision.
- **The gate keeps requiring a plan in every mode.** The mode picks the tree; it
  never turns the precondition off, so the canonical "no mode bypass" rule
  survives.
- **`odd/` becomes a gate-recognized planning tree**, which is what stops the
  write-the-plan deadlock.
