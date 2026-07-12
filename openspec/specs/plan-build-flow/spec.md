# Plan-Build-Flow Specification

## Purpose

Defines the `plan-build-flow` catalog recipe: an **ambient**, skill-only workflow
over the existing multi-phase change ceremony. No slash commands; the bundled
skill auto-invokes on substantial change work. Additive, opt-in, coexists with
classic flows.

## Requirements

### Requirement: Recipe manifest and command naming

`catalog/recipes/plan-build-flow/recipe.toml` SHALL declare one bundled skill,
**zero** slash commands, and `on-sync = ["validate-config"]` only. Command and
skill names MUST NOT use `sdd`, `openspec`, or `spec-driven` in any
user-facing identifier.

#### Scenario: Materialization produces skill only

- GIVEN the recipe is enabled and synced
- WHEN materialization completes
- THEN the bundled skill exists
- AND no `/plan`, `/build`, or `/archive` command files are generated

#### Scenario: No new schema or materializer surface

- GIVEN the recipe's `recipe.toml`
- WHEN validated against the current manifest schema
- THEN it requires zero new fields, on-sync actions, or materializer branches

### Requirement: Ambient planning trigger

The bundled skill SHALL auto-invoke on substantial change requests and run
explore → proposal → spec → design → tasks, stopping for human authorization.
Planning MUST NOT require slash commands or a dedicated worktree.

#### Scenario: Plan stops before implementation

- GIVEN a developer requests a substantial change
- WHEN the planning phase chain completes
- THEN tasks.md (or equivalent) exists and no production code files were modified

### Requirement: Ambient build trigger

After authorization, the skill SHALL run apply → verify → archive-tail in one
flow without exposing slash commands.

#### Scenario: Build implements, verifies, and closes after authorization

- GIVEN authorized tasks from a prior planning pass
- WHEN the developer approves implementation
- THEN implementation, verification, and change-folder close complete without a separate archive command

### Requirement: Archive channel degradation

The automatic close step SHALL gracefully no-op vault and tracker outputs when
integrations are absent, while still completing the change-folder close.

#### Scenario: Close without vault/tracker recipes

- GIVEN neither `vault-canonical-store` nor `trello-mcp-workflow` is enabled
- WHEN the close step runs
- THEN it emits a note that vault/tracker output was skipped
- AND the change folder still closes successfully

### Requirement: Orchestrator-absence degradation

When no gentle-ai orchestrator is available, the bundled skill SHALL instruct
the single agent to run mapped phases inline as one conversation.

#### Scenario: Inline execution without orchestrator

- GIVEN gentle-ai is not present
- WHEN planning or build phases run
- THEN the skill runs equivalent phases inline and no phase is silently skipped

### Requirement: Artifact store degradation and default

When Engram is unavailable, the skill SHALL fall back to file artifacts. When
Engram is present but no preflight resolved a store, the default SHALL be file
artifacts under `openspec/changes/<slug>/`.

#### Scenario: Default store with Engram but no preflight

- GIVEN Engram is available and no artifact-store preflight ran
- WHEN planning starts producing artifacts
- THEN artifacts are written as files, not memory-only

### Requirement: Vocabulary hygiene in generated output

Generated `[provides.brief]` fragments and the recipe README MUST NOT contain
the strings "SDD", "OpenSpec", or "spec-driven", and MUST NOT reference
`/plan` or `/build`.

#### Scenario: Brief and README are vocabulary-clean

- GIVEN the recipe is synced
- WHEN brief fragments and README are scanned
- THEN forbidden vocabulary and slash-command names are absent

### Requirement: Worktree-flow cross-reference

Brief fragments SHALL cross-reference worktree usage for implementation work
when `worktree-flow` is enabled, without a hard `requires` dependency.

#### Scenario: Cross-reference present without hard dependency

- GIVEN both recipes are enabled
- WHEN the generated brief is inspected
- THEN it references worktree usage for implementation
- AND the recipe syncs standalone without `worktree-flow` enabled

### Requirement: Coexistence with classic SDD

Enabling `plan-build-flow` MUST NOT modify, remove, or rename any existing
classic SDD command, skill, or recipe outside this recipe's own surface.

#### Scenario: Classic flow unaffected

- GIVEN a project with classic SDD commands already synced
- WHEN `plan-build-flow` is enabled and synced
- THEN all pre-existing non-plan-build-flow commands and skills remain unchanged

## Acceptance Criteria (test map)

| AC | Test | Req |
|----|------|-----|
| AC1 | `test_recipe_materializes_skill_only` | manifest |
| AC2 | `test_recipe_adds_no_schema_surface` | manifest |
| AC8 | `test_brief_and_readme_vocabulary_clean` | vocabulary |
| AC9 | `test_implementation_brief_references_worktree_flow` | worktree |
| AC10 | `test_classic_sdd_commands_unchanged` | coexistence |
