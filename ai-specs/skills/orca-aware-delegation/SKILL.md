---
name: orca-aware-delegation
description: "Trigger: Orca CLI, Orca orchestration, Orca worktree, explicit Orca delegation intent. Hydrate ai-specs worktrees before Orca launches scoped workers."
license: MIT
metadata:
  author: ai-specs
  version: "1.2"
  scope: [root]
  auto_invoke:
    - "Explicitly naming Orca CLI, Orca orchestration, or an Orca worktree"
    - "Explicitly requesting delegation through Orca"
---

## Activation Contract

Activate only when the request explicitly names Orca CLI, Orca orchestration, an Orca worktree, or equivalent explicit Orca delegation intent. Do not activate because `orca` exists, an environment variable exists, worktree-flow is enabled, or the request says only "delegate". Keep the normal non-Orca route unchanged.

## Hard Rules

- The canonical/main agent is human-facing and owns the request, decomposition, decisions, worker dispatch decisions, verification/integration, staging/commit, and worktree lifecycle. `ai-specs` creates and hydrates the worktree; Orca owns runtime, Run, Task, Dispatch, and terminal control. The canonical orchestrator keeps lifecycle ownership from Run binding through release.
- A worker owns change content only inside its assigned worktree. It must not create, remove, reassign, or independently manage that worktree or its lifecycle, and must not stage, commit, push, or merge.
- Launch every Orca agent as a visible interactive TUI session created through Orca (`worker-start` or an Orca terminal surface). Never launch a worker with `claude -p`, `opencode run`, a provider API call, a background/headless provider process, or any equivalent programmatic invocation.
- `worker_done` reports task completion only; it never implies terminal closure. Ask the human whether the TUI stays retained for inspection or continuation, record the answer, and use `worker-retain` when it does. Use `worker-release` only for an explicit close/cleanup decision the human-facing orchestrator makes after judging the worker finished. Never release automatically as the default after `worker_done`.
- For standalone/direct work, use the existing worktree-flow `/worktree-new` procedure, then run full `ai-specs sync <absolute-worktree>` before dispatch; do not substitute `sync-agent`.
- Capture both a pre-sync baseline and a post-sync baseline. Sync rewrites provisioning-owned files such as `AGENTS.md`, `ai-specs/.ai-specs.lock`, managed recipe overrides, and other generated runtime/recipe files; classify those separately from worker-owned change content, and preserve unrelated pre-existing changes.
- Before any commit or staging, revert or exclude only the sync-generated provisioning paths back to their pre-sync state, then stage only the worker-owned change paths. Never commit sync output merely because it is tracked or changed. Only an explicit separate authorization can make a provisioning change part of the deliverable, and the canonical orchestrator owns the final staging and commit decision.
- Never copy generated files or secrets from the canonical checkout. Verify the exact target root and readiness before launch; never launch an ambiguous or unhydrated target.
- Standalone/direct worktrees created by `ai-specs` are discoverable by Orca. Pass the existing `path:<absolute-worktree>`; never ask Orca to create a second worktree for the same change. `worker-start` reuses that exact path and rejects `--setup`, so omit `--setup` — setup is `not_applicable`.
- Instruct the worker to verify the exact root, branch, and worktree list (`git rev-parse --show-toplevel`, `git branch --show-current`, `git worktree list`) before its first write. Hooks are defense in depth only, with delegated/subprocess coverage gaps on OpenCode subagent/MCP, Pi/OMP, and Cursor.
- Dispatch state `ready` or stage `input_accepted` is not proof the worker executed the task. Require meaningful readiness/activity plus a final `worker_done`.
- For `monorepo-submodules`, fail closed unless a human explicitly approves subrepo handling. Superrepo discovery does not imply subrepo worktree discovery; never present subrepo worktrees as automatically discoverable.

## Decision Gates

| Gate | Action |
| --- | --- |
| Explicit Orca intent | Activate this route; otherwise use the normal route. |
| Standalone/direct topology | Create and hydrate with `/worktree-new` and full `ai-specs sync`. |
| `monorepo-submodules` topology | Stop or obtain explicit human-approved subrepo handling. |
| Root, hydration, and readiness checks | Dispatch only when all pass; otherwise preserve the worktree and report the failed phase. |
| Existing hydrated worktree | Pass `path:<absolute-worktree>` and omit `--setup`. |
| Launch surface | Use an Orca-created interactive TUI only; refuse headless or programmatic launches. |
| `worker_done` received | Keep the terminal open, ask retain-or-close, and retain unless the orchestrator decides to close. |
| Staging or commit | Restore sync-generated paths to the pre-sync baseline and stage only worker-owned paths. |
| Dispatch stalled or failed | Inspect worker state, then retry the same Task and path with `--retry-of`; never create a duplicate worktree. |

## Execution Steps

1. Resolve the project topology and target change. For standalone/direct work, use `/worktree-new` from the canonical agent, then capture the pre-sync baseline with `git -C <absolute-worktree> status --short`.
2. Run `ai-specs sync <absolute-worktree>` from the canonical route. Confirm success, verify `git -C <absolute-worktree> rev-parse --show-toplevel` resolves to the exact intended root, then capture the post-sync baseline with `git -C <absolute-worktree> status --short` and diff it against the pre-sync baseline to classify sync-generated provisioning paths.
3. Hand Orca the existing selector `path:<absolute-worktree>`: bind or create the Run, create the Task, then start the worker as a visible interactive TUI session.
4. Observe heartbeat and `worker_done`, keep the TUI open, and settle retain-or-release with the human. The canonical agent owns verification, integration, the commit boundary, and cleanup.

Exact command shapes: [Orca lifecycle commands](references/orca-lifecycle-commands.md).

## Output Contract

Return status, exact worktree path/root, hydration result, pre-sync and post-sync baselines, Orca Run/Task/Dispatch selectors, launch surface, worker boundary, `worker_done` outcome, retain-or-release decision, and verification/integration status. Attribute provisioning-owned generated files to sync, not to the worker, and name the paths excluded from staging. On hydration or launch failure, preserve the worktree for inspection and report the exact path and failed phase. Never silently create a duplicate, claim execution without proof, release a terminal the human did not close, or commit sync output.

## References

- [Orca lifecycle commands](references/orca-lifecycle-commands.md)
- [Worktree creation command](../../../catalog/recipes/worktree-flow/commands/worktree-new.md)
- [Worktree flow skill](../../../catalog/recipes/worktree-flow/skills/worktree-flow/SKILL.md)
- [Project runtime brief](../../../AGENTS.md)
