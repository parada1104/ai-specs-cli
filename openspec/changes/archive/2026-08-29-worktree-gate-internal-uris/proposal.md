# Proposal: make worktree-gate URI and cwd resolution safe

## Why

The worktree gate protects filesystem writes on protected branches, but it must
distinguish the hook process cwd (`$PWD`) from the tool event cwd (`event.cwd`).
The latter is the directory where a Bash/Shell command is expected to execute and
is therefore the correct base for relative candidates. Confusing them caused a
real false positive while writing external OMP configuration under
`~/.omp/agent/agents` from a runtime launched in this repository.

The gate also receives non-filesystem internal protocol URIs such as
`xd://resolve`, `artifact://<id>`, `local://<name>.md`, and `vault://<path>`.
These are tool interfaces, not Git destinations, and must not enter path
classification.

## What changes

1. Define and test cwd precedence: use event `cwd` for relative candidates,
   process `$PWD` only as fallback, and preserve absolute candidates unchanged.
2. Add an explicit allowlist for known internal URI schemes before filesystem
   classification.
3. Keep unknown schemes (`https://`, `file://`, `custom://`) on normal gating.
4. Add regressions for external and protected relative destinations, including
   process cwd versus event cwd.

## Shell cwd boundary

The event cwd is the base directory supplied by the runtime. This change does
not implement a shell interpreter or claim to resolve all dynamic control flow;
`cd <dir> && ...` remains a documented heuristic boundary and ambiguity stays
fail-open.

## Non-goals

- Do not add new runtime URI schemes or make arbitrary URI-like strings safe.
- Do not weaken protected-branch enforcement for filesystem paths.
- Do not redesign shell write heuristics or implement shell execution.

## Tracker

- **card_id**: TBD (optional, can add later)
- **url**: TBD

## Plan

1. Update the worktree-flow hook with the URI allowlist and cwd contract.
2. Extend gate integration tests with path/shell cwd and URI cases.
3. Update the worktree-flow delta spec with requirements and scenarios.
4. Run focused gate tests, then `./tests/run.sh`.
***