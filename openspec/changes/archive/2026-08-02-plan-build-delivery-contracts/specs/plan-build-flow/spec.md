# Delta for Plan-Build-Flow

## MODIFIED Requirements

### Requirement: Recipe manifest and command naming

`catalog/recipes/plan-build-flow/recipe.toml` MUST declare one bundled skill, zero slash
commands, and `on-sync = ["validate-config"]` only. It MAY declare the one delivery-contract
configuration field specified by this change, but MUST NOT require any additional schema fields,
on-sync actions, or materializer branches. Command and skill names MUST NOT use `sdd`,
`openspec`, or `spec-driven` in any user-facing identifier.

#### Scenario: Materialization produces skill and delivery contract only

- GIVEN the recipe is enabled and synced
- WHEN materialization completes
- THEN the bundled skill exists
- AND no `/plan`, `/build`, or `/archive` command files are generated
- AND only the declared delivery-contract configuration is resolved

#### Scenario: No recipe-specific materializer surface

- GIVEN the recipe contains its delivery-contract configuration field
- WHEN it is validated and materialized
- THEN the existing schema, config merge, and brief-rendering paths are used
- AND no new recipe-specific renderer or sync action is required

## ADDED Requirements

### Requirement: Project delivery contract configuration

The recipe MUST expose this optional configuration field with `required = false`, type,
default, enum, and non-empty `help_text`:

- `artifact_store_default`: type `string`, enum `openspec | engram | both`, default `openspec`.

The normal recipe schema and manifest merge MUST apply the default before a valid project
override. Invalid enum values MUST be rejected by recipe configuration validation rather than
silently accepted.

#### Scenario: Default resolves on sync

- GIVEN `plan-build-flow` is enabled with no value in the project manifest
- WHEN `ai-specs sync` resolves the recipe
- THEN `artifact_store_default` resolves to `openspec`

#### Scenario: Project override resolves through the manifest

- GIVEN the project manifest sets `artifact_store_default = "both"`
- WHEN the recipe configuration is resolved
- THEN that value replaces the schema default
- AND the resolved value remains available to brief interpolation

#### Scenario: Empty or absent configuration falls back safely

- GIVEN the recipe configuration table is absent or contains no field
- WHEN the recipe is validated and synced
- THEN sync succeeds
- AND the locked `openspec` default is used
- AND no configuration prompt or migration is required

#### Scenario: Invalid delivery value is rejected

- GIVEN a project sets `artifact_store_default` outside the accepted enum
- WHEN recipe configuration is validated
- THEN validation fails with the invalid field identified
- AND no invalid value is materialized into the project brief

### Requirement: Brief workflow-rule materialization

The recipe MUST carry the resolved store value through `provides.brief.workflow_rules` using
`{config.artifact_store_default}`. Existing generic config substitution MUST resolve the
present key to text and MUST leave an absent key verbatim; no recipe-specific render path may
be introduced. The new rule MUST state that the rendered value is the repository delivery
default to provide when an external session asks where planning artifacts should live, without
implementing session control flow. Existing plan-before-build, production-edit, PR, archive,
and merge rules MUST remain intact.

#### Scenario: Sync materializes the default rule

- GIVEN a project enables `plan-build-flow` without an override
- WHEN `ai-specs sync` renders the project `AGENTS.md` and supported briefs
- THEN the delivery rule contains `openspec`
- AND the placeholder is not present in the rendered output

#### Scenario: Sync materializes a project-specific rule

- GIVEN a project overrides the field with `engram`
- WHEN `ai-specs sync` renders the project brief
- THEN the delivery rule contains `engram`
- AND unrelated existing workflow rules remain present and ordered

#### Scenario: A recipe without delivery configuration remains renderable

- GIVEN an older or otherwise valid recipe has no delivery-contract key
- WHEN the generic brief renderer processes a rule containing an unknown `{config.KEY}` placeholder
- THEN rendering succeeds without a crash
- AND the unknown placeholder is preserved verbatim

### Requirement: Recipe surface excludes review-budget configuration

The recipe surface MUST NOT declare a `review_budget_lines` field, placeholder, validation
regex, or advisory warning section. Review-budget handling is intentionally left to the external
session preflight by design; it is not a plan-build-flow contract. No recipe-level warning or
budget token may be required for materialization, rendering, or validation.

#### Scenario: No review-budget token enters the recipe surface

- GIVEN the recipe schema, raw recipe source, brief rules, and bundled skill are inspected
- WHEN delivery-contract configuration and workflow rules are enumerated
- THEN no review-budget field, placeholder, regex, warning section, or budget token is present
- AND the external session preflight remains the sole owner of that session decision

### Requirement: Recipe version and documentation contract

The `plan-build-flow` recipe version MUST be bumped from `1.2.0` to `1.3.0`. The recipe README
and `docs/recipes-catalog.md` MUST document the store field, its type, accepted values, default,
project override behavior, and brief materialization through sync. Documentation MUST
 distinguish repository-declared policy from behavior of any external runtime that may consume
the rendered brief.

#### Scenario: Version and docs expose the same contract

- GIVEN the updated recipe and documentation are inspected
- WHEN the plan-build-flow entry is compared across the recipe README and catalog
- THEN the version is `1.3.0`
- AND the store field, default, enum values, and materialization behavior are described consistently

### Requirement: Delivery-contract scope excludes session controls

The plan-build-flow recipe surface MUST NOT declare `chained_pr_default` or any execution-mode
configuration. It MUST NOT implement preflight questions, preflight answer collection,
orchestration control flow, or a dependency on an external orchestrator. No recipe code,
documentation, or tests for this change may introduce a `gentle-ai` reference. Chained-PR
strategy and execution mode remain session decisions outside this recipe.

#### Scenario: Session-control fields are absent

- GIVEN the recipe schema and resolved manifest configuration are inspected
- WHEN delivery-contract fields are enumerated
- THEN only `artifact_store_default` is added
- AND neither `chained_pr_default` nor an execution-mode field is present

#### Scenario: Recipe remains orchestrator-agnostic

- GIVEN the recipe is validated, synced, or rendered
- WHEN its source, documentation, and focused tests are inspected
- THEN no preflight implementation or external orchestrator dependency is required
- AND the recipe surface contains no `gentle-ai` reference
