# Proposal: Runtime Brief Baseline for Fresh Init

## Intent

PR #75 added `[provides.brief]` so ENABLED recipes contribute prose to `AGENTS.md`. But `ai-specs init` enables ZERO recipes (template ships every `[recipes.*]` and `[brief]` commented out), so a new adopter gets a near-empty brief: H1 + optional intro + `## Project` only. Every behavioral section (Runtime Flow, Workflow Rules, Context Sources, Conflict Policy, Useful Commands) is suppressed. Secondary gap: `init.sh:181` writes a one-line placeholder and never renders, so the first real brief appears only on a later `sync`.

## Scope

### In Scope
- Pre-enable `session-context` in `templates/ai-specs.toml.tmpl` (uncomment a `[recipes.session-context]` block, `enabled = true`).
- Make `init.sh` run `recipe-materialize.py` + `agents-render.py` after writing the TOML, so a fresh `AGENTS.md` is meaningful immediately (with `--preserve-if-runtime-brief`).
- Keep all baseline prose GENERIC — sourced only from `session-context`'s existing universal fragments (workflow_rules + conflict_policy).

### Out of Scope
- Creating a new dedicated `foundation` recipe (decided against — see Approach).
- Adding new fragment content to `session-context` or any other recipe.
- Changing the render layering contract, section order, or `[provides.brief]` schema.
- Backfilling existing projects (they already have populated manifests/briefs).

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- `runtime-brief-rendering`: add requirement that `init` produces a non-empty behavioral brief via a default-enabled baseline recipe (delta — no renderer logic change, only the init-time guarantee).

> Init has no dedicated capability spec today; the init-renders-brief behavior is folded into `runtime-brief-rendering` as the closest owner. sdd-spec to confirm during spec phase.

## Approach

Option A from exploration. **Decision: REUSE `session-context`, do NOT create a new `foundation` recipe.**

Justification:
- `session-context` already declares `[provides.brief]` with universal, zero-required-config fragments (one workflow_rules bullet, two conflict_policy bullets). Its capabilities (`session-bootstrap`, `conflict-policy`) only CONSUME provider bindings by convention — they render brief prose without any bound `tracker`/`memory`/`canonical-store`, so it works in a bare project.
- Reusing it avoids a near-duplicate content-only recipe, keeps a single source for foundational prose, and auto-updates the baseline as the recipe evolves — preserving the layering contract (project `[brief]` voice → recipe fragments → render order).
- A new `foundation` recipe would duplicate exactly these fragments and add catalog surface for no behavioral gain.

Init flow change: after step 3 (write TOML), call `recipe-materialize.py` to produce a resolved-config JSON (now non-empty: `enabled = ["session-context"]`), then `agents-render.py ... --preserve-if-runtime-brief --resolved-config <tmp>` — mirroring `sync.sh:108-116`. Placeholder write at line 181 is replaced/guarded by the render.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `templates/ai-specs.toml.tmpl` | Modified | Uncomment + enable `[recipes.session-context]` (`enabled = true`, pinned `version`). |
| `lib/init.sh` (~150-181) | Modified | Run materialize + render after TOML write; render replaces bare placeholder, idempotent via `--preserve-if-runtime-brief`. |
| `lib/_internal/agents-render.py` | Unchanged | Reuses #75 fragment merge/dedupe as-is. |
| `lib/_internal/recipe-materialize.py` | Unchanged | `build_resolved_config` now returns `enabled:["session-context"]` from the new template default. |
| `catalog/recipes/session-context/recipe.toml` | Unchanged | Fragments already universal; no edits. |
| `tests/` | Modified | Add init-renders-baseline test(s). |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Stale baseline prose shipped to adopters | Low | Prose lives in `session-context` (versioned recipe), not frozen into the template `[brief]`; updates flow on next `sync`. |
| Duplication when adopter later enables more recipes | Low | Render already key- + exact-string dedupes fragments (`collect_recipe_brief_fragments`); `session-context` is foundational so its keys are stable. |
| This-repo-specific content leaking into defaults | Low | Only `session-context`'s generic fragments are used; no Trello/vault/board values rendered without bindings. |
| Idempotency break on re-init / first sync | Med | Render uses `--preserve-if-runtime-brief`; if user adds the `<!-- ai-specs:runtime-brief -->` marker, render is skipped. Verify init→sync produces stable output. |
| `materialize`/`render` failure aborts init | Low | Wrap in best-effort guard; fall back to placeholder write on non-zero exit (init must stay robust offline). |

## Rollback Plan

Revert the two edits: re-comment `[recipes.session-context]` in the template and restore the `init.sh:181` placeholder-only behavior. No data migration, no generated state to clean (AGENTS.md is regenerated on next `sync`). Single-commit revert on `feat/runtime-brief-baseline`.

## Dependencies

- PR #75 (`[provides.brief]` + `collect_recipe_brief_fragments`) — already merged (commit `2668028`).

## Success Criteria

- [ ] Fresh `ai-specs init` produces an `AGENTS.md` with non-empty `## Workflow Rules` and `## Conflict Policy` sections.
- [ ] Rendered baseline contains NO this-repo-specific values (board IDs, vault scopes, project-specific commands).
- [ ] `init` then `sync` yields byte-stable `AGENTS.md` (idempotent); marker-bearing user briefs are preserved.
- [ ] Focused tests (`./tests/run.sh`) and `./tests/validate.sh` pass.

## Test Strategy

Strict TDD — RED first, runner `./tests/run.sh` (focused), `./tests/validate.sh` (full).
1. RED: test that `init` on a temp dir writes an `AGENTS.md` whose rendered text includes the `session-context` workflow_rules + conflict_policy fragment strings (asserts non-empty behavioral sections).
2. RED: test idempotency — `init` then `sync` (or second render) leaves `AGENTS.md` unchanged; a brief containing the runtime-brief marker is left untouched.
3. RED: test that the default template parses and `build_resolved_config` returns `enabled` containing `session-context`.
4. RED: negative — assert no this-repo-specific tokens (e.g. the dogfood board id) appear in a freshly rendered baseline.
5. GREEN: enable recipe in template + wire render into `init.sh`. REFACTOR as needed.
