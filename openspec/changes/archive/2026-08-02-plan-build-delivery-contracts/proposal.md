# Proposal: plan-build-delivery-contracts

## Intent

The `plan-build-flow` recipe currently declares how a project plans and builds, but it does
not declare where its planning artifacts should live. A runtime therefore has to infer that
default from its own session behavior instead of reading a repository-owned contract.

This proposal adds one declarative, per-project contract to `plan-build-flow` and carries it
into the generated project brief. The recipe remains an orchestrator-agnostic bundle: it
declares policy and materializes instructions, while an external runtime may consume those
instructions during its own preflight. External runtime behavior is not controlled by this
repository and is not made a recipe dependency.

## Goal

Make `plan-build-flow` expose one optional recipe configuration field:

- `artifact_store_default` — the project's default planning-artifact store, defaulting to
  `openspec`, with accepted values `openspec`, `engram`, and `both`.

Materialize the resolved value as a brief workflow rule using the existing
`provides.brief.workflow_rules` mechanism and `{config.artifact_store_default}` interpolation.
After `ai-specs sync`, generated `AGENTS.md` and supported brief consumers must contain the
resolved project value, including manifest overrides.

The review budget is intentionally left to the external session preflight by design. The
preflight asks for and respects that session decision; it is not a plan-build-flow contract
or recipe configuration field. Enforcement/discipline follow-ups remain outside this change.

The implementation must not import, invoke, modify, or depend on the external preflight
package. It must use the existing schema, config merge, and brief-rendering paths; no
recipe-specific renderer is needed.

## Scope

### In scope

1. **Recipe configuration**
   - Add `[config.artifact_store_default]` to the `plan-build-flow` recipe.
   - Use the existing `string` schema type, allowed values `openspec`, `engram`, and `both`,
     and default `openspec`.
   - Provide non-empty help text describing the repository-owned delivery default.
   - Preserve optional-field behavior: projects that do not configure the field resolve to
     the default and remain backwards compatible.

2. **Brief workflow rule**
   - Append one concise string-form rule to the existing
     `provides.brief.workflow_rules` list.
   - Interpolate `{config.artifact_store_default}` through the generic existing renderer.
   - State that the rendered value is the project's default when a session asks where
     planning artifacts should live.
   - Preserve existing plan-before-build, production-edit, PR, archive, and merge rules.

3. **Documentation**
   - Update the `plan-build-flow` recipe README with the field name, type, accepted values,
     default, override example, and sync materialization.
   - Update `docs/recipes-catalog.md` so the catalog entry exposes the same field and default.
   - Keep documentation clear that external preflight consumption is runtime behavior this
     repository does not control.

4. **Focused tests**
   - Replace the current assertion that `plan-build-flow` has no configuration schema.
   - Add schema coverage for field name, optionality, type, default, and exact enum.
   - Add manifest-override and enum-rejection coverage.
   - Add brief interpolation and end-to-end materialization coverage.
   - Keep the negative surface explicit: no `chained_pr_default`, execution-mode configuration,
     or recipe-level review-budget token.

5. **Recipe version and generated surfaces**
   - Bump the recipe version from `1.2.0` to `1.3.0` for the additive configuration and
     materialized contract behavior.
   - Update version-pinned examples and fixtures that describe this recipe as part of the
     documentation/test change, without hand-editing generated harness shims.
   - Generated outputs remain products of `ai-specs sync`.

### Out of scope / NON-goals

- **No `chained_pr_default`.** Chained PR strategy remains an external session decision.
- **No execution-mode contract.** The recipe must not declare `interactive`/`auto` or an
  equivalent field.
- **No preflight implementation.** Do not add preflight questions, preference collection,
  orchestration logic, or runtime answer parsing to the recipe.
- **No external package changes.** The recipe only renders instructions an external runtime
  may consume.
- **No recipe dependency on an orchestrator.**
- **No change to the planning gate's blocking contract.** `plan-build-gate.sh` continues to
  block production edits when the required active planning artifact is absent.
- **No new interpolation or renderer path.** Reuse existing `{config.KEY}` substitution.
- **No broad policy rewrite.** Existing plan/build classification, worktree requirements,
  minimum artifacts, PR readiness, archive-tail, and merge-guardian rules remain unchanged.
- **No project-wide suite requirement.** Verification is limited to focused recipe, schema,
  rendering, brief-baseline, and gate tests.

## Decisions

| ID | Decision | Consequence |
|----|----------|-------------|
| **D1** | Declare `artifact_store_default` as an optional string contract with default `openspec`; accepted values are `openspec`, `engram`, and `both`. | Existing projects preserve current behavior while projects may choose a repository-owned artifact-store default. |
| **D2** | Leave review-budget handling entirely to the external session preflight by design; it is not a recipe contract. | No recipe field, placeholder, warning section, or validation surface is added for review workload. Enforcement/discipline follow-ups remain external to this change. |
| **D3** | Materialize the store contract through the existing brief `workflow_rules` and `{config.artifact_store_default}` interpolation. | No new renderer, orchestrator adapter, or recipe-specific substitution logic is introduced. |
| **D4** | Do not declare `chained_pr_default` or an execution-mode default. | Session-interactive decisions outside this recipe remain outside its schema and brief contract. |
| **D5** | Bump recipe version `1.2.0` → `1.3.0` and synchronize documentation/version-pinned fixtures. | Consumers can identify the additive contract surface; generated outputs continue to be refreshed by sync. |

## Approach

1. **Schema-first default.** Add the standard optional `[config.artifact_store_default]` table,
   using existing enum validation and schema-default-before-manifest-override precedence.
2. **Ambient brief propagation.** Append one string-form workflow rule whose placeholder resolves
   through the existing config namespace and materializer; preserve enabled-order and unrelated
   project/recipe brief semantics.
3. **Docs and regression coverage.** Update the recipe README/catalog and focused tests for
   defaults, overrides, interpolation, generated brief output, and excluded-surface assertions.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `catalog/recipes/plan-build-flow/recipe.toml` | Modified | Add one config field, one brief rule, and version `1.3.0`; preserve existing hooks and rules. |
| `catalog/recipes/plan-build-flow/README.md` | Modified | Document the store field, enum, default, overrides, and materialization. |
| `docs/recipes-catalog.md` | Modified | Add the store delivery-contract field and `1.3.0` catalog information. |
| `tests/test_plan_build_flow_recipe.py` | Modified | Replace the no-config assertion and add schema/materialization/brief contract coverage. |
| `tests/test_agents_render_brief_fragments.py` | Modified | Verify store config interpolation and resolved fragments. |
| `tests/test_brief_render_policy.py` | Unchanged unless an intentional additive-rule assertion fails | Preserve generic renderer and policy contracts. |
| `tests/test_runtime_brief_baseline.py` | Unchanged unless an intentional additive-rule assertion fails | Preserve synthetic fixture and baseline contracts. |
| Generated harness outputs | Sync-only | Refresh through `ai-specs sync` if required; never hand-edit generated shims. |
| External SDD preflight/runtime | Unchanged | It may consume rendered project instructions, but remains outside this repository's ownership and dependency graph. |

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Exact brief baselines change when one rule is added. | Low | Update only intentional additive expectations; retain exact ordering and unrelated fragments. |
| Config defaults or merge precedence drift. | Medium | Test omitted field, manifest override, enum rejection, and materialization through existing paths. |
| `openspec` enum text trips the recipe vocabulary guard. | Certain | Exempt exactly the store config table and intended README section, with a positive enum assertion. |
| New rule collides with another recipe. | Low | Use string-form fragment and verify enabled-order and deduplication behavior. |

## Rollback Plan

1. Revert the `plan-build-flow` recipe version, store config table, and delivery workflow rule to
   `1.2.0`.
2. Revert corresponding README/catalog entries and focused test expectations.
3. Refresh generated outputs with `ai-specs sync` so removed managed brief fragments are no
   longer propagated.
4. Leave existing project manifests without the field valid; behavior returns to the original
   plan-build artifact gate and external session defaults.
5. No data migration is required.

## Dependencies

- Existing recipe config schema support for optional string fields, defaults, enum validation,
  and manifest override merging.
- Existing `provides.brief.workflow_rules` materialization and `{config.KEY}` substitution.
- Existing `plan-build-flow` skill and `plan-build-gate.sh` contracts.
- Existing focused recipe, renderer, brief-policy, and runtime-baseline test conventions.
- `ai-specs sync` for propagating source recipe changes to supported harness outputs.
- An external runtime may consume generated instructions during its own session preflight; that
  behavior is outside this repository and is not an implementation dependency.

## Success Criteria

- [ ] The recipe exposes `artifact_store_default` as an optional string field with accepted
      values `openspec`, `engram`, and `both`, and default `openspec`.
- [ ] With no manifest override, the field resolves to the locked default and existing projects
      remain backwards compatible.
- [ ] Manifest overrides resolve through the normal config merge path and invalid enum values
      are rejected by existing schema/config handling.
- [ ] `provides.brief.workflow_rules` contains one new string-form rule using
      `{config.artifact_store_default}` while preserving the existing five rules and order.
- [ ] `ai-specs sync` materializes the resolved store value into `AGENTS.md`/brief output.
- [ ] Focused tests cover schema, defaults, overrides, enum rejection, interpolation, and
      end-to-end brief materialization.
- [ ] Existing brief vocabulary, plan-before-build rules, PR/archive gates, and
      `plan-build-gate.sh` blocking behavior remain intact.
- [ ] The recipe README and `docs/recipes-catalog.md` document the same store field, default,
      accepted values, override behavior, and materialization.
- [ ] The recipe version and associated documentation/fixtures move consistently from `1.2.0`
      to `1.3.0`.
- [ ] No recipe code, documentation, or tests introduce a `chained_pr_default`, execution-mode
      field, preflight implementation, or external runtime dependency; the recipe surface also
      contains no review-budget token.
- [ ] The final change modifies only the intended recipe/docs/tests and this proposal's planned
      surfaces; no external package is changed.

## Planning depth

**Classification: `domain_change` → full chain** after this proposal:

1. `design.md` — lock TOML field shape, store brief rule wording, ordering, and baseline update strategy.
2. Delta spec under `openspec/changes/2026-08-02-plan-build-delivery-contracts/specs/` — describe
   schema defaults, store brief materialization, and explicit exclusions.
3. `tasks.md` — sequence schema/tests first, then recipe/brief/docs changes, then sync and focused verification.

This proposal is planning-only. It does not modify production recipe, documentation, test, or
external runtime files.

## Proposal question round

The supplied exploration and locked constraints resolve the product questions; no answer blocks
this proposal. Design must preserve these assumptions:

1. **Pain and users:** Projects need a repository-owned artifact-store default when an SDD session starts.
2. **Invariants:** Existing planning-artifact gates stay blocking; chained PR strategy and execution mode remain external session decisions.
3. **Boundary:** Review-budget handling intentionally remains in the external session preflight, not in this recipe change.

## Resolved open questions

- **Store default:** `openspec` aligns with the current external preflight default.
- **Materialization path:** Existing brief workflow rules and `{config.KEY}` interpolation are sufficient; no renderer change is required.
- **External boundary:** No external preflight package changes, imports, or dependencies are part of this change.

## Artifact path

`openspec/changes/2026-08-02-plan-build-delivery-contracts/proposal.md`

## Tracker

- Card: [SDD] plan-build-flow: contratos declarativos de entrega (artifact store + review budget) — https://trello.com/c/OdQPZZa3
- **card_id**: `OdQPZZa3`

