# Proposal: plan-build-flow — a two-verb catalog recipe over hidden SDD ceremony

## Why

- **Problem**: The full SDD lifecycle (explore → proposal → spec → design → tasks → apply → verify → archive) is powerful but exposes eight phases and OpenSpec vocabulary to developers who only want two things: *plan the change*, then *build it*.
- **Current-state gap**: `ai-specs-cli` deliberately removed user-facing SDD/OpenSpec vocabulary (archived decision `2026-05-18-docs-remove-sdd-refocus`) and now owns only "the spec layer and tool integrations (recipes)"; orchestration lives in gentle-ai. There is no packaged, opt-in way to give a project a simplified plan/build surface without re-teaching SDD.
- **Why now**: Trello card #29 requests this abstraction as a recipe that must coexist with classic SDD.

## What Changes

New foundational catalog recipe. Additive and opt-in — no core CLI logic, no changes to the manifest schema, materializer, or hook dispatcher.

| Area | Impact | Description |
|------|--------|-------------|
| `catalog/recipes/plan-build-flow/recipe.toml` | New | Recipe manifest: bundled skill, two commands, `on-sync validate-config`, `[provides.brief]` fragments (plan/build vocabulary only) |
| `catalog/recipes/plan-build-flow/skills/plan-build-flow/SKILL.md` | New | Auto-invoked skill mapping plan/build to the underlying ceremony, degradation policy, auto artifact-store/slug selection |
| `catalog/recipes/plan-build-flow/commands/plan.md`, `commands/build.md` | New | Prompt files materialized as `/plan` and `/build` slash commands |
| `catalog/recipes/plan-build-flow/README.md` | New | Materialized doc via `[[provides.docs]]` |
| `docs/recipes-catalog.md` | Modified | Add catalog entry |

## Capabilities

### New Capabilities
- `plan-build-flow`: the recipe contract — recipe.toml shape, the `/plan` and `/build` command surface, the auto-invoked skill's phase mapping, degradation policy, and worktree-flow cross-reference.

### Modified Capabilities
- None.

## Approach

Adopt exploration **Approach 1 (skill/instructions-only recipe)**. It is the only option that adds zero schema/materializer surface, so it cannot violate the "catalog recipe, not core logic" constraint nor the 2026-05-18 boundary. Approaches 2 (config + new on-sync action) and 3 (shell out to the gentle-ai binary) are deferred until a concrete failure mode justifies the added coupling.

**Decisions taken:**
- **Name**: `plan-build-flow` (not the card's `sdd-plan-mode`). Card fidelity loses to vocabulary hygiene — the recipe's whole purpose is hiding SDD, and the archived decision stripped SDD terms from user-facing surfaces. Commands are `/plan` and `/build`, never `/sdd-plan`.
- **Phase mapping**: `/plan` = explore → proposal → spec → design → tasks (produces artifacts, dev reviews/authorizes). `/build` = apply → verify. **Archive** runs as the automatic closing step at the tail of `/build` (change-folder close + vault summary + tracker comment) — kept inside the two-verb UX rather than adding a visible third verb, matching the card's three-stage sketch without leaking a third command.
- **On-sync hook**: use `validate-config` only. The card's "verify spec/design/tasks dirs" is infeasible today (`validate-config` checks required config fields, not filesystem dirs); a dir-validation action is out of scope, not silently faked.
- **Degradation**: no gentle-ai orchestrator → the skill instructs the single agent to run the phases inline as one conversation. No Engram → fall back to OpenSpec file artifacts (or inline `none` if the user wants no files). Never fail or silently skip ceremony.
- **Worktree interaction**: `/plan` needs no worktree; `/build` writes files and must run in a worktree when `worktree-flow` is enabled. Expressed as a `workflow_rules` brief cross-reference, not a hard dependency (recipe schema has no `requires`).
- **Reconciliation with 2026-05-18**: this is opt-in catalog content that *wraps* an external orchestrator, not reintroduced core SDD product logic. No ceremony logic ships in `ai-specs-cli`; the recipe is a thin naming/UX layer whose generated brief text says only "plan"/"build".

## Non-Goals

- Reintroducing `[sdd]`-shaped manifest sections, `docs/ai/sdd.md`, or any SDD/OpenSpec vocabulary in generated brief/doc text.
- New recipe-schema fields, new on-sync hook actions, or new materializer branches.
- A hard dependency on gentle-ai's CLI binary or its JSON status contract.
- Replacing classic SDD — the two flows coexist.

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| SDD/OpenSpec vocabulary leaks into generated brief/docs | Med | Spec must assert plan/build-only wording; review generated `AGENTS.md` output |
| gentle-ai absent → `/plan`,`/build` are empty promises | Med | Degradation policy: run phases inline as a single-agent conversation |
| Correctness depends on external orchestrator behavior, untestable here | Med | Scope repo tests to materialization ("files materialize correctly"); document the boundary |
| Template fixes never reach synced consumers (issue #104) | Low | Prefer skill/command content over templates where possible |

## Rollback Plan

Fully additive and opt-in. Rollback = delete `catalog/recipes/plan-build-flow/` and revert the `docs/recipes-catalog.md` entry. Projects that never enabled the recipe are unaffected; projects that synced it lose only the `/plan` and `/build` commands and the bundled skill on their next sync after removal.

## Success Criteria

- [ ] Recipe materializes cleanly via `ai-specs sync` with zero schema/materializer changes.
- [ ] Generated `AGENTS.md` brief and README contain no "SDD"/"OpenSpec"/"spec-driven" vocabulary.
- [ ] `/plan` produces reviewable artifacts; `/build` implements + verifies + closes; both degrade gracefully without gentle-ai/Engram.
- [ ] Classic SDD continues to work unchanged alongside the recipe.

## Open Questions

1. **Archive channels**: the card assumes a vault canonical store and a Trello tracker for the closing step. Should `/build`'s archive gracefully no-op those outputs when `vault-canonical-store` / `trello-mcp-workflow` are not enabled, or should archive be documented as requiring them? (Assumed: graceful no-op with a note.)
2. **Default artifact store when Engram is present but no orchestrator preflight ran**: default to `hybrid`, or `openspec` files only? (Assumed: `openspec` files, since files are the reviewable deliverable the card centers on.)
