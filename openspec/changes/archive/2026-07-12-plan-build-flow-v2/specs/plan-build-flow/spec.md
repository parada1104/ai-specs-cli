# Delta for plan-build-flow

## MODIFIED Requirements

### Requirement: Recipe manifest and command naming

`catalog/recipes/plan-build-flow/recipe.toml` SHALL declare one bundled skill and **zero** slash commands. Command and skill names MUST NOT use `sdd`, `openspec`, or `spec-driven` in any user-facing identifier.

#### Scenario: Materialization produces skill only

- GIVEN the recipe is enabled and synced
- WHEN materialization completes
- THEN the bundled `plan-build-flow` skill exists
- AND no `/plan`, `/build`, or `/archive` command files are generated

(Previously: exactly two commands `plan` and `build` were materialized.)

### Requirement: Ambient planning trigger

The bundled skill SHALL auto-invoke on substantial change requests and run explore → proposal → spec → design → tasks before implementation, stopping for human authorization. The skill MUST NOT require slash commands.

#### Scenario: Plan stops before implementation

- GIVEN a developer requests a substantial change
- WHEN the planning phase chain completes
- THEN tasks.md (or equivalent) exists and no production code files were modified

(Previously: triggered by `/plan`.)

### Requirement: Ambient build trigger

After authorization, the skill SHALL run apply → verify → archive-tail in one continuous flow without exposing slash commands.

#### Scenario: Build implements, verifies, and closes after authorization

- GIVEN authorized tasks from a prior planning pass
- WHEN the developer approves implementation
- THEN implementation, verification, and change-folder close complete without a separate archive command

(Previously: triggered by `/build`.)

### Requirement: Worktree-flow cross-reference

Generated brief fragments SHALL state that file-writing implementation runs in a dedicated worktree when `worktree-flow` is enabled, without referencing `/build`.

#### Scenario: Cross-reference present without hard dependency

- GIVEN both recipes are enabled
- WHEN the generated brief is inspected
- THEN it references worktree usage for implementation work
- AND the recipe still syncs standalone without `worktree-flow` enabled

(Previously: referenced `/build`.)
