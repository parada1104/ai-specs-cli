## ADDED Requirements

### Requirement: Pre-delegation worktree/branch check in the always-on brief

The `worktree-flow` recipe SHALL publish an always-on `workflow_rules` brief
fragment requiring the orchestrator to verify the current worktree and branch
before dispatching a write-capable subagent or task, independent of whether a
runtime `pre-tool-use` hook will fire for delegated tool calls.

#### Scenario: Brief rule present in recipe declaration
- **GIVEN** the catalog `worktree-flow` recipe
- **WHEN** its `[provides.brief].workflow_rules` are read
- **THEN** at least one rule SHALL require verifying worktree/branch before
  dispatching write-capable subagents/tasks
- **AND** SHALL state that runtime pre-tool-use hooks must not be the sole guard
  for delegated work on harnesses where subprocess calls may bypass the hook
