# Proposal: Recipe Init Prompts

## Problem

Recipes currently declare reusable primitives and sync-time materialization, but they do not have a standard way to guide per-project setup before materialization. This limits recipes that need project-specific choices such as tracker board/list mappings, MCP server selection, memory provider details, generated template overrides, or workflow conventions.

The existing recipe contracts intentionally keep catalog recipes general. Project-specific values already belong in `ai-specs/ai-specs.toml`, especially under `[recipes.<id>.config]`, but there is no agent-facing initialization contract that tells an agent what to ask, what context to inspect, what MCPs may be queried, or how to propose durable changes without hardcoding one project.

As a result, reusable recipes such as future Trello, OpenMemory, and canonical memory recipes either need manual documentation outside the recipe contract or bespoke implementation paths. Both options make idempotency, reviewability, and safe human confirmation harder to enforce.

## Goals

- Add a minimum recipe-level contract for declaring an interactive init prompt or workflow in `recipe.toml`.
- Add a CLI entry point for recipe initialization that an agent can invoke, inspect, or use to launch guided configuration.
- Allow init workflows to read the current manifest context and discover available MCP configuration without exposing secrets.
- Allow init workflows to propose changes to `[recipes.<id>.config]` in `ai-specs/ai-specs.toml` without applying unconfirmed behavioral changes.
- Allow init workflows to materialize or update templates/overrides in a reviewable, idempotent way.
- Ensure a second init run detects existing configuration and offers update paths instead of duplicating config or files.
- Support Trello dogfooding by allowing a recipe init prompt to guide board/list/label mapping through the configured Trello MCP.
- Document the pattern so future OpenMemory and canonical-memory recipes can use the same init mechanism.

## Non-Goals

- Implement the full Trello recipe tracked separately by card #62.
- Implement an OpenMemory recipe or canonical memory recipe.
- Define a final rigid capability taxonomy such as `tracker` or `canonical-memory`.
- Modify project flows, generated runtime files, or external systems without human confirmation.
- Run `ai-specs sync` in this repository as part of this change before the runtime brief/registry split in #65 is resolved.
- Replace existing recipe add/sync behavior or make init mandatory for V1 recipes.
- Build a general-purpose prompt execution engine beyond the minimum contract needed for agent-guided initialization.

## Affected Modules

- `bin/ai-specs`: expose the new recipe initialization subcommand in top-level help text if needed.
- `lib/recipe.sh`: dispatch `ai-specs recipe init <id> [path]` alongside existing `list` and `add` subcommands.
- `lib/_internal/recipe_schema.py`: parse and validate the new optional init declaration in `recipe.toml`.
- `lib/_internal/recipe-read.py`: include init metadata in recipe JSON output for CLI and tests.
- `lib/_internal/recipe-add.py`: decide whether `recipe add` should surface init availability in its preview without running init.
- New or existing internal recipe init helper: inspect recipe metadata, current `ai-specs.toml`, available MCP declarations, and existing recipe config to produce an agent-readable initialization plan or reviewable patch guidance.
- `lib/_internal/toml-read.py`: reuse existing manifest readers and extend only if init needs normalized recipe/MCP/project context not already exposed.
- `lib/_internal/recipe-materialize.py`: remain sync-focused; changes should be limited to shared helpers only if needed for idempotent template preview logic.
- `docs/recipe-schema.md`: document the recipe init declaration, manifest config relationship, idempotency expectations, and examples.
- `openspec/specs/recipe-schema/spec.md`: define the new `recipe.toml` init schema.
- `openspec/specs/recipe-cli/spec.md`: define `ai-specs recipe init` behavior and failure modes.
- `openspec/specs/recipe-config/spec.md`: clarify that init may propose `[recipes.<id>.config]` values but sync remains responsible for config merge validation.
- `openspec/specs/recipe-sync-materialization/spec.md`: clarify that init preview/update of templates is separate from sync materialization and must be reviewable/idempotent.
- `openspec/specs/recipe-manifest-contract/spec.md`: clarify durable storage of per-recipe init output in `[recipes.<id>.config]` when appropriate.
- `openspec/specs/mcp-preset-merge/spec.md`: clarify init-time inspection of MCP declarations does not alter sync-time MCP merge precedence.

## Planned Spec Deltas

### `recipe-schema`

- Add an optional init declaration to `recipe.toml`.
- Define the minimum supported fields for an init prompt/workflow, including an identifier or prompt path, optional description, and optional declared context needs such as manifest and MCP inspection.
- Require init prompt paths to be relative to the recipe directory and validate referenced files exist.
- Preserve backward compatibility: recipes without init declarations remain valid and behave exactly as before.

### `recipe-cli`

- Add `ai-specs recipe init <id> [path]`.
- Require initialized projects with `ai-specs/ai-specs.toml`.
- Validate the recipe exists and its init declaration is valid.
- If no init declaration exists, fail clearly or report that the recipe has no init workflow without mutating files.
- Emit an agent-readable initialization brief that includes current recipe install/config state, relevant manifest context, declared MCP availability, and reviewable next actions.
- Enforce idempotency by detecting existing `[recipes.<id>.config]` and existing materialized/override targets.

### `recipe-config`

- Specify that init workflows may propose values for `[recipes.<id>.config]` and should align proposals with the recipe `[config]` schema.
- Specify that init must not bypass sync-time required-field validation.
- Specify unknown or out-of-schema proposed config should be surfaced as warnings or rejected according to the final CLI contract.

### `recipe-manifest-contract`

- Clarify that per-project init output intended to survive sync belongs under `[recipes.<id>.config]` unless another existing manifest section is explicitly responsible.
- Clarify that init must update existing recipe config keys rather than appending duplicate recipe declarations.

### `recipe-sync-materialization`

- Clarify separation between init-time reviewable template/override proposals and sync-time recipe materialization.
- Add idempotency scenarios for rerunning init after config or override files already exist.

### `mcp-preset-merge`

- Clarify that init-time MCP discovery reads currently configured MCPs and recipe-declared MCP presets for guidance only.
- Preserve project manifest precedence for sync-time MCP merging.

## Risks

- The init contract could become too broad and accidentally become a workflow engine. Mitigation: keep the first contract prompt-oriented and agent-facing, with small validated metadata and explicit non-goals.
- Init may blur the line between proposing and applying changes. Mitigation: require reviewable output and human confirmation before mutating flow-critical configuration or templates.
- MCP discovery may accidentally expose secret-backed environment values. Mitigation: reuse normalized MCP metadata and redact env-backed values in init output.
- Idempotent updates to TOML can be fragile if implemented as text appends. Mitigation: define precise behavior before implementation and test duplicate prevention around `[recipes.<id>]` and `[recipes.<id>.config]`.
- Existing baseline tests are already red in this worktree. Mitigation: record baseline failures and require focused tests plus final validation evidence during apply/verify rather than treating the proposal phase as a fix opportunity.
- The dependencies (#63, #64, #65) may affect exact file locations, MCP merge behavior, and runtime brief generation. Mitigation: keep this proposal aligned with existing specs and avoid implementing or syncing until those contracts are stable enough for this change.

## Rollback Plan

- Because the init declaration is planned as optional, rollback can remove parser support, CLI dispatch, docs, and spec deltas without affecting existing V1 recipes.
- If partially implemented behavior proves unsafe, disable `ai-specs recipe init` at the dispatcher level while leaving existing `recipe list`, `recipe add`, and `sync` behavior untouched.
- Revert any added recipe init examples or fixtures independently from recipe schema/config/sync code.
- Do not migrate existing manifests automatically; any proposed `[recipes.<id>.config]` values remain human-reviewable TOML changes that can be manually reverted.

## Dependency And Baseline Notes

- Trello card #66 depends on #63 external dirs, #64 MCP preset merge, and #65 AGENTS runtime brief/registry split.
- This proposal assumes the existing recipe schema/config/manifest/sync contracts remain authoritative for recipe declaration, per-recipe config, and materialization.
- This proposal preserves the #64 direction that project manifest MCP values take precedence during sync-time MCP merging.
- A pre-artifact `./tests/run.sh` in this worktree exits 1 with three existing sync-related errors across 205 discovered tests. This is a baseline risk for later apply/verify phases and is not addressed by the proposal phase.
- This phase intentionally creates only `openspec/changes/motor-recipe-init-prompts/proposal.md`; design, spec deltas, tasks, and implementation are deferred to later phases.
