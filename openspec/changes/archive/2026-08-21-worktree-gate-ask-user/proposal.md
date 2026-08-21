# Proposal: consult the user when gate_mode=ask blocks a write

## Why

The `worktree-flow` recipe ships a guarded write hook with three gate modes.
Today `always` and `ask` are behaviorally identical — both hard-block writes on
the protected branch (exit 2). The only thing `ask` adds is a stderr hint the
**agent** can self-bypass (`re-run with WORKTREE_GATE_MODE=off`), which the
host model reads as permission to proceed or to implicitly create a worktree
**without the user deciding**. Observed live on Claude: asking should mean "the
user chooses", not "the agent chooses".

The owner wants worktrees to become optional for people who resist them, while
keeping even the resistant path safe. The lever is the `ask` mode semantics.

## What changes

1. In `gate_mode=ask`, a blocked write still exits 2 but its stderr message
   lists three destination choices for the **user**: create a worktree
   (recommended), feature branch in place, or write directly on the protected
   branch as an explicit override.
2. The agent must relay the question to the user and wait; it must not select a
   destination itself and must not self-bypass.
3. Remove the `AskHint()` self-bypass instruction from the `ask` branch in the
   Go binary and the frozen Bash reference.
4. Keep `always` (hard block + worktree guidance) and `off` (never gate)
   unchanged.

## Non-goals

- No change to the decision core or to `always`/`off`.
- No interactive TTY path; the hook stays a pre-tool-use gate whose stderr is
  surfaced to the host agent, which does the asking.
- No PR #230 touch, no `creation_mode`, no gentle-ai/GGA lifecycle integration
  in this change.

## Decisions

1. Option 3 (write on the protected branch) is **regulated by the skill**, not
   advertised by the gate message. The stderr text lists the three choices
   without revealing any bypass mechanism; only an explicit user choice lets the
   agent re-run the write, and the message must not mention `WORKTREE_GATE_MODE=off`.

## Tracker

Trello board not reachable from this session (`mcp connect trello` failed).
Card to be created by the owner before merge; see branch
`change/worktree-gate-ask-user`.