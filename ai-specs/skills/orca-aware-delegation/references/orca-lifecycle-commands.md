# Orca lifecycle commands

Exact command shapes for `orca-aware-delegation`. Every launch below is a visible
interactive TUI session created through Orca. Never substitute `claude -p`,
`opencode run`, a provider API call, or a background/headless provider process.

## Supervised lifecycle

```bash
orca orchestration run-create --objective "<objective>" --json
orca orchestration task-create --spec "<worker task>" --json
orca orchestration worker-start --task <task_id> --worktree path:<absolute-worktree> --agent codex --json
orca orchestration check --wait --types worker_done,escalation,question --timeout-ms 900000 --json
orca orchestration worker-retain --dispatch <dispatch_id> --json # default after worker_done; keeps the TUI open
orca orchestration worker-release --dispatch <dispatch_id> --json # only on an explicit human close decision
```

`worker_done` ends the task, not the terminal. Retain unless the human-facing
orchestrator explicitly decides to close the worker.

## Stalled or failed dispatch

```bash
orca orchestration worker-show --dispatch <dispatch_id> --json
orca orchestration worker-start --task <task_id> --retry-of <dispatch_id> --worktree path:<absolute-worktree> --agent codex --json
```

Retry the same Task and the same path. Never create a duplicate worktree.

## Terminal / full handoff

```bash
orca terminal send --terminal <handle> --text "Work only in path:<absolute-worktree>; verify root, branch, and worktree list before writing; never manage the worktree; never stage, commit, push, or merge." --enter --json
```
