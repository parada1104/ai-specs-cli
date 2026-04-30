# Proposal: trello-mcp-workflow-recipe

Change: `trello-mcp-workflow-recipe`
Card: "Formalizar recipe trello-mcp-workflow" (#62)
Trello URL: https://trello.com/c/QYK5JphS/62

---

## Why

Today the Trello project-management flow is scattered across multiple sources:
- The `trello-pm-workflow` skill defines card templates and PM conventions, but it is purely informational — no automation.
- The `session-bootstrap` skill manually queries Trello during the 3-level consensus check, but the logic is ad-hoc and not reusable.
- MCP Trello is already wired in `ai-specs.toml`, yet every agent session must rediscover board IDs, card states, and linking conventions from scratch.

This fragmentation means:
1. **No reproducibility**: A second ai-specs project cannot copy the flow without re-implementing the conventions.
2. **Manual coordination**: Moving a card when an OpenSpec change advances from `proposal` → `apply` → `verify` is a human step.
3. **Context drift**: Cards and OpenSpec changes can diverge because nothing binds them automatically.

A formal `trello-mcp-workflow` recipe solves this by packaging the convention *and* the automation into an installable, versioned bundle that any ai-specs project can enable with `ai-specs recipe add trello-mcp-workflow`.

## What Changes

### 1. New recipe package: `catalog/recipes/trello-mcp-workflow/`
A complete recipe directory containing:
- `recipe.toml` — V2 recipe manifest declaring 4 capabilities, 4 hooks, 3 config fields, and provides declarations.
- `skills/trello-mcp-workflow/SKILL.md` — Bundled skill implementing runtime automation via Trello MCP.
- Card templates (`templates/card-feature.md`, `card-bug.md`, `card-spike.md`, `card-epic.md`, `card-handoff.md`).
- `commands/trello-workflow.md` — Quick-reference command for agent usage.
- `docs/README.md` — Installation, configuration, and rollback instructions.

### 2. Extension of `lib/_internal/recipe-materialize.py` hook system
Add 4 new recognized hook actions to `execute_hooks`:
- `bootstrap-board` — Validates `board_id` presence and writes `.recipe/<recipe-id>/bootstrap-ready` marker.
- `link-trello-card` — Deferred to agent runtime (prints notice at sync time).
- `sync-card-state` — Deferred to agent runtime (prints notice at sync time).
- `comment-verification` — Deferred to agent runtime (prints notice at sync time).

### 3. Config schema validation for `board_id`
Add validation in materialize: `board_id` required, `default_list`/`epic_list` optional with defaults.

### 4. Tests
- Unit tests for new hook actions in `recipe-materialize.py`.
- Integration tests for recipe materialization of `trello-mcp-workflow`.

### 5. Documentation
- Recipe `docs/README.md` explains minimum `ai-specs.toml` config.

## Capabilities

### New Capabilities
- `trello-session-bootstrap`: Read marker, query board via MCP, detect active card by list position + labels, feed structured primitive to consensus check.
- `trello-card-linking`: Detect linked card from session context, post structured comment with change folder path and artifact list, offer card creation from template when absent.
- `trello-state-sync`: Resolve target list from phase mapping, move card, replace single phase label, post phase-transition comment with metadata.
- `trello-progress-comment`: Collect changed files from apply metadata, read test results from verify-report, assemble structured comment with verdict and archive link.

### Modified Capabilities
- `sync-hooks`: Add 4 new recognized hook actions (`bootstrap-board`, `link-trello-card`, `sync-card-state`, `comment-verification`) beyond the existing `validate-config`.

## Impact

**Affected modules/packages:**
- `lib/_internal/recipe-materialize.py` — `execute_hooks()` function extended with 4 new action branches.
- `catalog/recipes/` — New `trello-mcp-workflow/` recipe directory.
- `ai-specs/skills/trello-pm-workflow/` — Coexists; not modified, not replaced.

**Affected specs:**
- `sync-hooks` — Delta spec adding 4 new hook actions.
- New specs: `trello-session-bootstrap`, `trello-card-linking`, `trello-state-sync`, `trello-progress-comment`.

**Dependencies:**
- Trello MCP must be configured in the project's `ai-specs.toml` for runtime capabilities.
- `recipe-schema` and `capability-declaration` specs are *not* modified — the existing V2 schema (`Capability`, `Hook`, `ConfigSchema`) already supports all needed declarations.

**Risk assessment:**
- Low risk: New recipe package is additive; coexists with legacy skill.
- Medium risk: Modifying `execute_hooks()` — but existing hook actions are preserved, new actions are additive branches.
- The 3 deferred hooks (link, sync, comment) are no-ops at sync time, so they cannot break sync.

**Rollback plan:**
- Remove `catalog/recipes/trello-mcp-workflow/` directory.
- Revert `execute_hooks()` branches for the 4 new actions.
- Remove config schema entries from recipe.toml.
- The legacy `trello-pm-workflow` skill remains operational throughout.