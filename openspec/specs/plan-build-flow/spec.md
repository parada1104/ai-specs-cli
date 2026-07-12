# Plan-Build-Flow Specification

## Purpose

NEW capability. Defines the `plan-build-flow` catalog recipe: a two-verb
(`/plan`, `/build`) UX over the existing multi-phase change ceremony, with no
new manifest schema, materializer branch, or on-sync action. The recipe is
additive, opt-in, and coexists with the classic multi-command flow.

## Requirements

### Requirement: Recipe manifest and command naming

`catalog/recipes/plan-build-flow/recipe.toml` SHALL declare one bundled skill,
exactly two commands (`plan`, `build`), and `on-sync = ["validate-config"]`
only. Command and skill names MUST NOT use `sdd`, `openspec`, or
`spec-driven` in any user-facing identifier or slash-command name.

#### Scenario: Materialization produces exactly two commands

- GIVEN the recipe is enabled and synced
- WHEN materialization completes
- THEN `/plan` and `/build` exist as slash commands and no third command (e.g. `/archive`) is generated

#### Scenario: No new schema or materializer surface

- GIVEN the recipe's `recipe.toml`
- WHEN validated against the current manifest schema
- THEN it requires zero new fields, on-sync actions, or materializer branches

### Requirement: `/plan` phase mapping

`/plan` SHALL run explore → proposal → spec → design → tasks and stop,
producing artifacts for developer review/authorization. `/plan` MUST NOT
require a dedicated worktree.

#### Scenario: Plan stops before implementation

- GIVEN a developer runs `/plan` for a new change
- WHEN the phase chain completes
- THEN tasks.md (or equivalent) exists and no code files were modified

### Requirement: `/build` phase mapping and automatic close

`/build` SHALL run apply → verify, then automatically run the archive/close
step (change-folder close, vault summary, tracker comment) as the tail of the
same command, without exposing a separate third verb.

#### Scenario: Build implements, verifies, and closes in one invocation

- GIVEN authorized tasks from a prior `/plan`
- WHEN a developer runs `/build`
- THEN implementation, verification, and change-folder close all complete without a separate archive command

### Requirement: Archive channel degradation

The automatic close step SHALL gracefully no-op vault and tracker outputs
when `vault-canonical-store` / `trello-mcp-workflow` are not enabled,
emitting an informative note, while still completing the change-folder close.

#### Scenario: Close without vault/tracker recipes

- GIVEN neither `vault-canonical-store` nor `trello-mcp-workflow` is enabled
- WHEN `/build`'s close step runs
- THEN it emits a note that vault/tracker output was skipped
- AND the change folder still closes successfully

### Requirement: Orchestrator-absence degradation

When no gentle-ai orchestrator is available, the bundled skill SHALL instruct
the single agent to run the mapped phases inline as one conversation. The
recipe MUST NOT fail or silently skip ceremony in this case.

#### Scenario: Inline execution without orchestrator

- GIVEN gentle-ai is not present in the environment
- WHEN `/plan` or `/build` is invoked
- THEN the skill runs the equivalent phases inline in the current conversation and no phase is silently skipped

### Requirement: Artifact store degradation and default

When Engram is unavailable, the skill SHALL fall back to OpenSpec file
artifacts (or inline-only `none` if explicitly requested). When Engram is
present but no orchestrator preflight resolved a store, the default SHALL be
OpenSpec file artifacts.

#### Scenario: Default store with Engram but no preflight

- GIVEN Engram is available and no artifact-store preflight ran
- WHEN `/plan` starts producing artifacts
- THEN artifacts are written as OpenSpec files, not Engram-only

### Requirement: Vocabulary hygiene in generated output

Generated `[provides.brief]` fragments and the recipe README MUST NOT contain
the strings "SDD", "OpenSpec", or "spec-driven".

#### Scenario: Brief and README are vocabulary-clean

- GIVEN the recipe is synced into a project
- WHEN the generated `AGENTS.md` brief fragment and `README.md` are scanned for "SDD", "OpenSpec", "spec-driven"
- THEN none of those strings are found

### Requirement: Worktree-flow cross-reference

`/build`'s brief fragment SHALL cross-reference `worktree-flow` as a
`workflow_rules` note (not a hard `requires` dependency), stating that
`/build` runs in a dedicated worktree when `worktree-flow` is enabled.

#### Scenario: Cross-reference present without hard dependency

- GIVEN both recipes are enabled
- WHEN the generated brief is inspected
- THEN it references worktree usage for `/build` and the recipe still syncs standalone without `worktree-flow` enabled

### Requirement: Coexistence with classic SDD

Enabling `plan-build-flow` MUST NOT modify, remove, or rename any existing
classic SDD command, skill, or recipe.

#### Scenario: Classic flow unaffected

- GIVEN a project with classic SDD commands already synced
- WHEN `plan-build-flow` is enabled and synced
- THEN all pre-existing SDD commands and skills remain unchanged

## Acceptance Criteria (test map)

| AC | Test | Req |
|----|------|-----|
| AC1 | `test_recipe_materializes_two_commands` | manifest/naming |
| AC2 | `test_recipe_adds_no_schema_surface` | manifest/naming |
| AC3 | `eval_plan_build_flow_live` / `ac3_plan_stops_before_apply` (live); materialization partial | /plan mapping |
| AC4 | `tests/evals/scenarios/plan-build-flow/ac4_*` (planned live) | /build mapping |
| AC5 | `tests/evals/scenarios/plan-build-flow/ac5_*` (planned live) | archive degradation |
| AC6 | transcript judge layer (deferred) | orchestrator absence |
| AC7 | `tests/evals/scenarios/plan-build-flow/ac7_*` (planned live) | artifact store default |
| AC8 | `test_brief_and_readme_vocabulary_clean` | vocabulary hygiene |
| AC9 | `test_build_brief_references_worktree_flow` | worktree cross-ref |
| AC10 | `test_classic_sdd_commands_unchanged` | coexistence |
