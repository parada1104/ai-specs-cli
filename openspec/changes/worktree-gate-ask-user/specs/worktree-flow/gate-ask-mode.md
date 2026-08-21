# Spec delta: gate_mode=ask consults the user instead of self-bypassing

## Context

Today `gate_mode=always`, `ask`, and `off` behave as follows in both the Go
binary (`gate/main.go`) and the frozen Bash reference
(`hooks/worktree-gate-legacy.sh`):

- `always` → block the write (exit 2) with a "create a worktree first" message.
- `ask` → block the write (exit 2) **identically**, and additionally print to
  stderr an `AskHint()` telling the agent to re-run with
  `WORKTREE_GATE_MODE=off`.
- `off` → never gate.

`ask` and `always` are therefore byte-identical in hard behavior: both block
hard (exit 2). The only difference is that `ask` instructs the agent, via
stderr, to bypass the gate itself. Running under Claude (which surfaces hook
stderr back into the model context), that self-bypass instruction is read as a
fact the model can just follow — so worktrees "still get created" (or writes get
allowed) without the **user** ever making the call. That is exactly what the
project owner observed.

This change stops `ask` from self-bypassing and instead turns the block into a
real user decision among three destination options.

## Purpose

Provide a per-project, opt-in way for people who resist worktrees to still be
safe: when `gate_mode=ask` and a write targets the protected main-worktree
branch, the agent must **present the choice to the user and wait**, never decide
or bypass on its own.

## Behavior (target)

New `gate_mode=ask` contract when a candidate write is blocked:

1. The gate still refuses the protected-branch write (exit 2 — the write does
   not happen silently in the main checkout).
2. The stderr message presents three destination options, in order:
   1. **Create a dedicated worktree** (recommended) — `git worktree add` under
      `.worktrees/` and run the edit there.
   2. **Feature branch in place** — `git checkout -b <feature>` on the current
      checkout and run the edit there.
   3. **Write directly on the protected branch** (`{branch}`) — permitted only
      with the user's explicit choice; a conscious override of the guard, not a
      default.
3. The agent must **ask the user which option they want and stop** until they
   answer, using the harness's native user-question mechanism: pi →
   `ask_user_question`, Claude → `AskUserQuestion`, opencode/cursor/omp → their
   interactive user prompt. The hook itself does not implement an interactive
   TTY; it emits stderr and the host agent translates that into a real
   user-facing question. It must not pick a destination itself, and it must not
   use the old `WORKTREE_GATE_MODE=off` re-run as a self-authorization
   shortcut.
4. The stderr message does **not** advertise any bypass mechanism. The skill
   prose regulates authorizing option 3: only an explicit user choice may lead
   the agent to re-run the write against the protected branch, and the message
   must not reveal `WORKTREE_GATE_MODE=off` to the agent.
5. The old `AskHint()` (`to bypass for this invocation, re-run with
   WORKTREE_GATE_MODE=off`) is dropped from the `ask` branch. `always` keeps its
   current hard block + worktree guidance; `off` stays fully bypassed.

## Scenarios

| Event | Behavior |
|---|---|
| `ask`, write on protected main-worktree branch | Block (exit 2) + 3-option message; agent asks user and waits |
| `ask`, write in a linked worktree | Allow (unchanged) |
| `ask`, non-protected branch in main checkout | Allow (unchanged) |
| `always`, write on protected branch | Block (exit 2) + worktree guidance (unchanged) |
| `off` | Allow / never gate (unchanged) |
| unknown `gate_mode` value | Warn + fall back (unchanged) |

## Non-goals

- Do not change `always` or `off` semantics.
- Do not change the Go/Bash decision core (`decide.go`, scope/topology/URIs).
- Do not implement interactive TTY prompting in the hook itself; the hook is a
  pre-tool-use gate whose stderr is surfaced to the host agent, and each
  harness translates the block into its **native user-question mechanism**
  (pi `ask_user_question`, Claude `AskUserQuestion`, opencode/cursor/omp
  interactive prompts). The recipe skill documents that per-agent mapping.
- Do not touch PR #230 or introduce `creation_mode` here.
- Do not integrate with gentle-ai lifecycle (GGA) gates in this change.
- `WORKTREE_GATE_MODE` env override stays available to operators; the point is
  the agent no longer advertises it to itself in `ask` mode.