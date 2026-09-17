# Proposal: add an Orca-aware delegation skill

## Intent

Add a project-local ai-specs skill that lets the canonical, human-facing
orchestrator prepare direct worktrees and delegate scoped change work through
Orca without losing the project harness.

## Scope

- Activate only for explicit Orca CLI or Orca orchestration intent.
- Hydrate ai-specs-created direct worktrees with the existing full `sync` flow.
- Use visible Orca TUI sessions and preserve the Run/Task/Dispatch lifecycle.
- Keep worker ownership limited to change content; keep staging and commit
  decisions with the canonical orchestrator.
- Exclude sync-generated provisioning output from worker commits.
- Fail closed for automatically undiscoverable monorepo-submodule worktrees.

## Evidence

- End-to-end direct-worktree audit completed in `.worktrees/orca-flow-audit`.
- The skill worktree was dispatched through Orca and the worker TUI was retained
  for inspection.
- Focused contract tests: 19 passing.
- Full validation: `./tests/validate.sh` — 1831 tests passed, 116 skipped.

## Tracker

- GitHub issue: https://github.com/parada1104/ai-specs-cli/issues/223
- Trello card: https://trello.com/c/pTNWzrMN/85-add-orca-aware-delegation-skill
