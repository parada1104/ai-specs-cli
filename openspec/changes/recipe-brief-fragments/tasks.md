# Tasks: `recipe-brief-fragments`

Enable recipe authors to declare `[provides.brief]` fragments in `recipe.toml`. The harness
collects, deduplicates, substitutes `{config.KEY}` placeholders, and merges fragments into
the rendered runtime brief — with append/replace mode and mcp_descriptions override-fills-gap.

---

## Batch Overview

| Batch | Content | Can run after | Parallelizable within? |
|-------|---------|---------------|------------------------|
| B1 | `recipe_schema.py` — dataclasses + parser + validation (tests first) | — | No (TDD sequential) |
| B2 | `recipe-materialize.py` — `_fragments_to_json` + both attach paths (tests first) | B1 done | No (TDD sequential) |
| B3 | `agents-render.py` — collect/substitute/merge/mcp/mode (tests first) | B2 done | No (TDD sequential) |
| B4 | Catalog `[provides.brief]` blocks + scaffold reduction | B1 done | Yes — recipes independent; scaffold independent |
| B5 | Documentation — `docs/recipe-schema.md` + `docs/ai-specs-toml.md` | B3 done | Yes — two files independent |
| B6 | Regression + idempotency checks + final validation | B3, B4, B5 done | No (sequential verification) |

**Critical path**: B1 → B2 → B3 → B6  
**Parallel opportunity**: B4 and B5 can overlap with B3 work (schema stable after B1).

Estimated batch count: **6**

---

## Batch 1 — `recipe_schema.py`: Dataclasses, Parser, Validation

> **TDD**: write failing tests (RED) → implement (GREEN) for each component.

### 1.1 RED — test dataclass structure

- [x] 1.1 Write failing tests in `tests/test_recipe_schema.py` for `BriefFragment` and `BriefFragments` dataclasses:
  - `BriefFragment(text="Do X.")` has `key=None`, `text="Do X."`
  - `BriefFragment(text="Do Y.", key="foo")` has `key="foo"`, `text="Do Y."`
  - `BriefFragments()` has `None` for all six sections (`runtime_flow`, `context_sources`, `conflict_policy`, `workflow_rules`, `useful_commands`, `mcp_descriptions`)
  - `Recipe.brief_fragments` field exists and accepts `None` or `BriefFragments`
  - **Spec scenarios**: recipe-schema §"BriefFragment normalized correctly for simple string", §"BriefFragment normalized correctly for inline-table"
  - **Target file**: `tests/test_recipe_schema.py`

### 1.2 GREEN — implement dataclasses

- [x] 1.2 Add `BriefFragment`, `BriefFragments` dataclasses and `CONTRIBUTABLE_SECTIONS` / `PROJECT_ONLY_SECTIONS` constants to `lib/_internal/recipe_schema.py`; add `brief_fragments: BriefFragments | None = None` to the `Recipe` dataclass.
  - **Target file**: `lib/_internal/recipe_schema.py`

### 1.3 RED — test `_parse_brief_fragments` — absent + simple array + inline-table + both sections

- [x] 1.3 Write failing tests for `_parse_brief_fragments`:
  - Absent `[provides.brief]` → returns `None`
  - Simple array form → each string normalized to `BriefFragment(key=None, text=s)`, order preserved
  - Inline-table form → each entry normalized to `BriefFragment(key=k, text=t)`, key preserved
  - Both sections present → both lists populated
  - Empty array `[]` → valid, zero fragments for that section
  - **Spec scenarios**: recipe-schema §"[provides.brief] absent", §"[provides.brief] present with simple array form", §"[provides.brief] present with inline-table form", §"[provides.brief] with both contributable sections"; recipe-brief-fragments §"Empty section array accepted"
  - **Target file**: `tests/test_recipe_schema.py`

### 1.4 GREEN — implement `_parse_brief_fragments` (happy path)

- [x] 1.4 Implement `_parse_brief_fragments(raw, context) -> BriefFragments | None` in `lib/_internal/recipe_schema.py` — absent returns `None`; simple string array → `BriefFragment(text=s, key=None)`; inline-table list → `BriefFragment(text=..., key=...)`.
  - **Target file**: `lib/_internal/recipe_schema.py`

### 1.5 RED — test validation errors (reject intro/purpose, unknown section, missing fields, mixed forms)

- [x] 1.5 Write failing tests for `_parse_brief_fragments` validation errors:
  - `intro` in `[provides.brief]` → validation error naming "project-only section"
  - `purpose` in `[provides.brief]` → validation error naming "project-only section"
  - Unknown section name (e.g., `custom_section`) → error naming the key and listing valid sections
  - Inline-table entry missing `text` → error naming missing field
  - Inline-table entry missing `key` → error naming missing field
  - Mixed string-array and inline-table in same section → error naming the section and "mixed forms"
  - **Spec scenarios**: recipe-schema §"intro section in [provides.brief] causes validation failure", §"purpose section ...", §"Unknown section name ...", §"Inline-table with missing text field ...", §"Inline-table with missing key field ..."; recipe-brief-fragments §"Mixed-form declaration rejected", §"intro declared in [provides.brief] rejected", §"purpose declared in [provides.brief] rejected", §"Unknown section name rejected"
  - **Target file**: `tests/test_recipe_schema.py`

### 1.6 GREEN — implement validation errors in `_parse_brief_fragments`

- [x] 1.6 Add validation branches to `_parse_brief_fragments` for project-only sections, unknown sections, missing `text`/`key` fields, and mixed forms; wire `_parse_brief_fragments` into `validate_recipe_toml` via `provides.get("brief")`.
  - **Target file**: `lib/_internal/recipe_schema.py`

### 1.7 Run B1 tests and confirm GREEN

- [x] 1.7 Run `./tests/run.sh` (or `python -m pytest tests/test_recipe_schema.py`) and confirm all B1 tests pass.

---

## Batch 2 — `recipe-materialize.py`: `brief_fragments` in resolved-config.json

> **TDD**: write failing tests (RED) → implement (GREEN).

### 2.1 RED — test `_fragments_to_json` helper

- [x] 2.1 Write failing tests in `tests/test_recipe_materialize.py` for the new `_fragments_to_json` helper:
  - `None` input → `{}`
  - `BriefFragments` with only `workflow_rules` populated → `{"workflow_rules": [{"key": null, "text": "..."}]}`
  - `BriefFragments` with `key` set → `{"context_sources": [{"key": "foo", "text": "..."}]}`
  - Sections with `None` value → omitted from output dict
  - **Spec scenarios**: runtime-brief-rendering §"brief_fragments included for recipe with [provides.brief]", §"brief_fragments absent for recipe without [provides.brief]"
  - **Target file**: `tests/test_recipe_materialize.py`

### 2.2 GREEN — implement `_fragments_to_json`

- [x] 2.2 Add `_fragments_to_json(bf) -> dict[str, list[dict]]` to `lib/_internal/recipe-materialize.py` — omit `None` sections, `{}` if `bf` is `None`.
  - **Target file**: `lib/_internal/recipe-materialize.py`

### 2.3 RED — test `brief_fragments` attached in both materialize paths

- [x] 2.3 Write failing integration tests that call the full materialize pipeline and assert:
  - A recipe with `[provides.brief].workflow_rules` → its entry in resolved output contains `brief_fragments.workflow_rules = [{key: null, text: "..."}]`
  - A recipe without `[provides.brief]` → its entry either lacks `brief_fragments` key or has `{}` (no error)
  - Both `materialize_recipes` path and `build_resolved_config_only` path emit `brief_fragments`
  - **Spec scenarios**: runtime-brief-rendering §"brief_fragments included for recipe with [provides.brief]", §"brief_fragments absent for recipe without [provides.brief]"
  - **Target file**: `tests/test_recipe_materialize.py`

### 2.4 GREEN — attach `brief_fragments` in both `materialize_recipes` and `build_resolved_config_only`

- [x] 2.4 In `lib/_internal/recipe-materialize.py`, after building `recipes_out[rid]` in `materialize_recipes` (recipe already loaded in the enabled loop) and in `build_resolved_config_only` (existing catalog block), call `_fragments_to_json(recipe.brief_fragments)` and assign to `recipes_out[rid]["brief_fragments"]`. Recipes without `[provides.brief]` → omit key or assign `{}`.
  - **Target file**: `lib/_internal/recipe-materialize.py`

### 2.5 Run B2 tests and confirm GREEN

- [x] 2.5 Run `./tests/run.sh` focusing on `test_recipe_materialize.py` and confirm all B2 tests pass.

---

## Batch 3 — `agents-render.py`: Collection, Substitution, Merge, MCP, Mode

> **TDD**: write failing tests in the new test file (RED) → implement (GREEN).

### 3.1 RED — test `substitute_config` pure function

- [x] 3.1 Create `tests/test_agents_render_brief_fragments.py` (use the existing `load_module()` helper to load `agents-render.py` by path). Write failing tests for `substitute_config(text, cfg_ns)`:
  - `{config.integration_branch}` with key present → resolves to value
  - `{config.base_branch}` with key absent → verbatim (no error, no crash)
  - Bare `{integration_branch}` (no `config.` prefix) → verbatim
  - `{{config.KEY}}` → literal `{config.KEY}` (brace escape)
  - Mixed escape + substitution: `"Run \`{config.test_command}\` (not {{skip}})."` → `"Run \`./run.sh\` (not {skip})."`
  - Lone unbalanced `{` in prose → returned untouched (no crash)
  - **Spec scenarios**: recipe-brief-fragments §"{config.KEY} substituted with resolved value", §"Missing config key leaves placeholder verbatim", §"Bare key reference (without config. prefix) is not substituted", §"{{ and }} render as literal braces", §"Mixed escape and substitution in the same string"; runtime-brief-rendering §"Missing config key leaves placeholder verbatim", §"{{ and }} escape to literal braces in fragments"
  - **Target file**: `tests/test_agents_render_brief_fragments.py`

### 3.2 GREEN — implement `substitute_config`

- [x] 3.2 Add `substitute_config(text, cfg_ns) -> str` to `lib/_internal/agents-render.py` using a custom `_M(dict)` with `__missing__` that re-emits any unknown placeholder verbatim; wrap in `try/except (ValueError, IndexError)`.
  - **Target file**: `lib/_internal/agents-render.py`

### 3.3 RED — test `collect_recipe_brief_fragments` — ordering, key-dedup, exact-string dedup

- [x] 3.3 Write failing tests for `collect_recipe_brief_fragments(resolved, section)`:
  - Single recipe → fragments returned in declaration order, substituted
  - Two recipes in `enabled` order → first recipe's fragments appear first
  - Reversed `enabled` order → order reversed
  - Key-based dedup: same `key` across two recipes → first-wins, second discarded
  - Exact-string dedup: same text (key=None) from two recipes → appears exactly once
  - Exact-string dedup: recipe fragment matches manifest addition (via outer code) → combined output contains it once
  - Recipe without `brief_fragments` key → contributes nothing, no error
  - Recipe with `brief_fragments = {}` → contributes nothing
  - Disabled recipe (not in `enabled`) → contributes nothing
  - **Spec scenarios**: runtime-brief-rendering §"Recipe fragments ordered by enabled declaration", §"Reordering enabled list changes fragment order", §"Key-based dedup — second occurrence silently discarded", §"Exact-string dedup — duplicate text from two recipes discarded", §"Exact-string dedup — manifest addition not repeated", §"Recipe without [provides.brief] produces no fragments"
  - **Target file**: `tests/test_agents_render_brief_fragments.py`

### 3.4 GREEN — implement `collect_recipe_brief_fragments`

- [x] 3.4 Add `collect_recipe_brief_fragments(resolved, section) -> list[dict]` to `lib/_internal/agents-render.py` — iterate `resolved["enabled"]`, skip recipes not in `resolved["recipes"]`, apply `substitute_config`, key-dedup (seen_keys set), exact-string dedup (seen_text set).
  - **Target file**: `lib/_internal/agents-render.py`

### 3.5 RED — test section merge (APPEND default, REPLACE opt-in, manifest prose never substituted)

- [x] 3.5 Write failing tests for the merged output of individual `_section_*` functions after they are updated:
  - APPEND default: recipe fragments appear before manifest additions
  - REPLACE mode: only manifest bullets appear, recipe fragments suppressed
  - REPLACE for one section does not affect another section (isolation)
  - `manifest_items` with `{config.test_command}` text → rendered verbatim (never substituted)
  - Empty manifest `[brief]` → sections populated entirely by recipe fragments
  - Recipe w/o fragments → output identical to rendering without that recipe
  - Idempotent: running collection twice on same input → same output
  - **Spec scenarios**: runtime-brief-rendering §"REPLACE mode suppresses recipe fragments for one section", §"REPLACE mode for one section does not affect other sections", §"Default APPEND mode when no _mode key present"; recipe-brief-fragments §"Manifest [brief] prose is never substituted"; recipe-manifest-contract §"workflow_rules_mode = 'replace' suppresses recipe contributions", §"Replace mode for one section does not affect others", §"APPEND is the default when no _mode key is present"; runtime-brief-rendering §"Recipe fragments prepended before manifest additions (APPEND default)"
  - **Target file**: `tests/test_agents_render_brief_fragments.py`

### 3.6 GREEN — thread `resolved` into `_section_*` functions and apply merge logic

- [x] 3.6 Update `lib/_internal/agents-render.py`: thread `resolved` as new argument into `_section_runtime_flow`, `_section_context_sources`, `_section_conflict_policy`, `_section_workflow_rules`, `_section_useful_commands`; inside each, read `<section>_mode` (default `"append"`), call `collect_recipe_brief_fragments` when mode is `"append"`, build `bullets` list (recipe items first, then deduplicated manifest items).
  - **Target file**: `lib/_internal/agents-render.py`

### 3.7 RED — test `_validate_brief_modes` / unknown `_mode` value

- [x] 3.7 Write failing test: manifest declares `workflow_rules_mode = "merge"` → validation raises error naming the key and listing valid values (`"append"`, `"replace"`).
  - **Spec scenarios**: recipe-manifest-contract §"Unknown _mode value causes validation failure"; runtime-brief-rendering §"Unknown _mode value → error"
  - **Target file**: `tests/test_agents_render_brief_fragments.py`

### 3.8 GREEN — implement `_validate_brief_modes`

- [x] 3.8 Add `_validate_brief_modes(brief)` to `lib/_internal/agents-render.py` that iterates all `*_mode` keys and raises for any value not in `{"append", "replace"}`; call from `render()` before section rendering.
  - **Target file**: `lib/_internal/agents-render.py`

### 3.9 RED — test `mcp_descriptions` override-fills-gap

- [x] 3.9 Write failing tests for `mcp_descriptions` merge in `_render_lines`:
  - Recipe declares `mcp_descriptions.trello` → gap-filled when manifest has no entry for `trello`
  - Manifest `[brief].mcp_descriptions.trello` → overrides recipe default
  - Multiple recipes, non-overlapping keys → both appear
  - No `mcp_descriptions` from any source → server still rendered (no crash), no description line
  - **Spec scenarios**: runtime-brief-rendering §"Project mcp_descriptions override wins", §"Recipe fills mcp_descriptions when project has no entry", §"No mcp_descriptions anywhere renders server without description"; recipe-brief-fragments §"Recipe fills mcp_descriptions gap", §"Project overrides recipe mcp_descriptions", §"Multiple recipes, non-overlapping mcp_descriptions"; recipe-manifest-contract §"Manifest mcp_descriptions overrides recipe default", §"Manifest mcp_descriptions for one server does not affect others"
  - **Target file**: `tests/test_agents_render_brief_fragments.py`

### 3.10 GREEN — implement mcp_descriptions override-fills-gap in `_render_lines`

- [x] 3.10 In `lib/_internal/agents-render.py` `_render_lines` (before calling `_section_mcp`): build `eff` dict from `collect_recipe_brief_fragments(resolved, "mcp_descriptions")` keyed by fragment `key`; `eff.update(brief.get("mcp_descriptions", {}) or {})` so manifest wins; pass `eff` as the effective `mcp_descriptions` to `_section_mcp` via a local `brief` copy (no mutation of caller's dict).
  - **Target file**: `lib/_internal/agents-render.py`

### 3.11 Run B3 tests and confirm GREEN

- [x] 3.11 Run `./tests/run.sh` focusing on `test_agents_render_brief_fragments.py` and confirm all B3 tests pass.

---

## Batch 4 — Catalog `[provides.brief]` Blocks + Scaffold `[brief]` Reduction

> B4 can begin once B1 (schema) is GREEN. Tasks within B4 are independent of each other.

### 4.1 Add `[provides.brief]` to `worktree-flow` recipe

- [x] 4.1 Add generic `[provides.brief]` block to `catalog/recipes/worktree-flow/recipe.toml`:
  - `workflow_rules` (simple array form): at minimum one bullet about creating worktrees for code changes and one about the `{config.integration_branch}` / `{config.base_branch}` PR guard (using whichever config key the recipe exposes). Use `{config.KEY}` placeholders — NO hardcoded project-specific literals.
  - **Target file**: `catalog/recipes/worktree-flow/recipe.toml`

### 4.2 Add `[provides.brief]` to `git-pr-flow` recipe

- [x] 4.2 Add `[provides.brief]` block to `catalog/recipes/git-pr-flow/recipe.toml`:
  - `workflow_rules` entries referencing `{config.base_branch}` and/or `{config.provider}` where the recipe config exposes them. Generic prose about PR-based merge workflow — no project-specific literals.
  - **Target file**: `catalog/recipes/git-pr-flow/recipe.toml`

### 4.3 Add `[provides.brief]` to `tdd-flow` recipe

- [x] 4.3 Add `[provides.brief]` block to `catalog/recipes/tdd-flow/recipe.toml`:
  - `workflow_rules` and/or `useful_commands` entries referencing `{config.test_command}` (the recipe's own config key). Generic TDD discipline prose.
  - **Target file**: `catalog/recipes/tdd-flow/recipe.toml`

### 4.4 Add `[provides.brief]` to `trello-mcp-workflow` recipe (if applicable)

- [x] 4.4 Add `[provides.brief]` block to `catalog/recipes/trello-mcp-workflow/recipe.toml` if it has context or workflow prose worth contributing:
  - `context_sources` and/or `workflow_rules` entries — generic prose about the tracker MCP usage. MCP descriptions should go in `mcp_descriptions` if the recipe owns the trello MCP.
  - **Target file**: `catalog/recipes/trello-mcp-workflow/recipe.toml`

### 4.5 Add `[provides.brief]` to `vault-canonical-store` recipe (if applicable)

- [x] 4.5 Add `[provides.brief]` block to `catalog/recipes/vault-canonical-store/recipe.toml`:
  - `context_sources` entry referencing vault as the canonical note-taker; optionally `mcp_descriptions` for the vault MCP. Use inline-table form with a stable `key` for the context_sources entry to enable semantic dedup.
  - **Target file**: `catalog/recipes/vault-canonical-store/recipe.toml`

### 4.6 Add `[provides.brief]` to `session-context` recipe (if applicable)

- [x] 4.6 Review `catalog/recipes/session-context/recipe.toml`; if it contributes context worth adding to the runtime brief (e.g., context_sources or workflow_rules), add a `[provides.brief]` block with generic prose and `{config.KEY}` placeholders.
  - **Target file**: `catalog/recipes/session-context/recipe.toml`

### 4.7 Reduce scaffold `[brief]` to intro + purpose only

- [x] 4.7 Find the manifest scaffold template (e.g., `ai-specs.toml.tmpl` or the file emitted by `recipe-init.py` / `ai-specs init`). Reduce the `[brief]` section to `intro` and `purpose` only. Add an explanatory comment directing authors to enable recipes for contributable sections and mentioning that `<section>_mode = "replace"` can override recipe fragments.
  - **Target files**: whichever scaffold/init file generates the `[brief]` template (check `lib/_internal/recipe-init.py` and any `.tmpl` files)

---

## Batch 5 — Documentation

> B5 can begin once B3 is GREEN (implementation stable). The two doc files are independent.

### 5.1 Update `docs/recipe-schema.md` — `[provides.brief]` author contract

- [x] 5.1 Add a new `### [provides.brief]` subsection under the `[provides]` section in `docs/recipe-schema.md` covering:
  - Both forms (simple string array and inline-table with `key`/`text`) with concrete TOML examples for each
  - Contributable sections table: `runtime_flow`, `context_sources`, `conflict_policy`, `workflow_rules`, `useful_commands`, `mcp_descriptions`
  - Explicit callout: `intro` and `purpose` are project-only and MUST NOT be declared in `[provides.brief]`
  - `{config.KEY}` substitution: namespace syntax, resolution source (merged recipe config), best-effort behavior (missing key → verbatim)
  - `{{`/`}}` brace escape rule with a worked example
  - Cross-reference to `docs/ai-specs-toml.md` for the manifest-side `<section>_mode` and `mcp_descriptions` override
  - **Target file**: `docs/recipe-schema.md`

### 5.2 Update `docs/ai-specs-toml.md` — reduced `[brief]`, modes, override-fills-gap

- [x] 5.2 Update `docs/ai-specs-toml.md` for the `[brief]` section:
  - Update the `[brief]` field table: note that contributable sections now augment recipe fragments rather than being the sole source; `intro` and `purpose` remain exclusively here
  - Add `<section>_mode` rows: `workflow_rules_mode`, `context_sources_mode`, etc. — document append (default) and replace semantics
  - Add `mcp_descriptions` override-fills-gap rule: project entry wins; recipe fills gap when project has no entry for a server
  - Cover both append and replace with manifest-level TOML examples
  - Note that a `[brief]` with only `intro` and `purpose` is valid — contributable sections come from recipes
  - **Target file**: `docs/ai-specs-toml.md`

---

## Batch 6 — Regression, Idempotency, and Final Validation

> B6 is strictly sequential and runs after B3, B4, and B5 are all complete.

### 6.1 RED — regression: own manifest `<!-- ai-specs:runtime-brief -->` marker suppresses regeneration

- [x] 6.1 Write a failing test confirming that `agents-render.py` invoked with `--preserve-if-runtime-brief` on an `AGENTS.md` that contains `<!-- ai-specs:runtime-brief -->` does NOT modify the file — even when enabled recipes now have `[provides.brief]` fragments.
  - **Spec scenarios**: runtime-brief-rendering §"Manifest with runtime-brief marker not regenerated"
  - **Target file**: `tests/test_agents_render_brief_fragments.py` (covered by `EndToEndRenderTests.test_runtime_brief_marker_suppresses_regeneration` from B3 + `B6RegressionTests.test_marker_suppresses_regeneration_with_recipe_fragments`)

### 6.2 GREEN — confirm marker suppression still works

- [x] 6.2 Verify the existing `--preserve-if-runtime-brief` guard in `agents-render.py` is not broken by the new `resolved` threading. Guard is at `render()` line 506–508, before any resolved threading; confirmed intact and all marker tests pass.
  - **Target file**: `lib/_internal/agents-render.py`

### 6.3 RED — idempotency: running sync twice with recipe fragments produces byte-identical output

- [x] 6.3 Write a failing test: render with a resolved-config containing `brief_fragments` twice in succession → output bytes are identical (no ordering drift, no duplicate bullets).
  - **Spec scenarios**: runtime-brief-rendering §"Output with fragments is idempotent"
  - **Target file**: `tests/test_agents_render_brief_fragments.py` (covered by `EndToEndRenderTests.test_idempotent_render_with_fragments` + `B6RegressionTests.test_idempotency_with_config_substitution` + `B6RegressionTests.test_exact_string_dedup_idempotency`)

### 6.4 GREEN — confirm idempotency

- [x] 6.4 Run the idempotency test and ensures it passes. All three idempotency tests pass; collection logic is deterministic (enabled-order iteration, dedup via seen_keys/seen_text sets).
  - **Target files**: `lib/_internal/agents-render.py` (no fixes needed — already deterministic)

### 6.5 Regression: ai-specs-cli's own manifest not corrupted or duplicated

- [x] 6.5 Performed simulation render of ai-specs-cli's own manifest with full recipe fragments:
  - AGENTS.md has NO `<!-- ai-specs:runtime-brief -->` marker — regeneratable; migrated cleanly.
  - No duplicate bullets in output — exact-string dedup handles identical recipe+manifest pairs; near-dups resolved by removing redundant manifest bullets.
  - Migrated `ai-specs/ai-specs.toml`: removed bullets now covered by recipes; added `useful_commands_mode = "replace"` to suppress tdd-flow's "Run tests: ./tests/validate.sh" duplicate of the auto-generated line.
  - Updated `AGENTS.md` with clean regenerated content.
  - **Target files**: `ai-specs/ai-specs.toml` (migrated), `AGENTS.md` (regenerated clean)

### 6.6 Final: `./tests/run.sh` all green

- [x] 6.6 Run `./tests/run.sh` (full test suite) and confirm all tests pass — 512 tests, all green (507 existing + 5 new B6 regression tests).

### 6.7 Final: `./tests/validate.sh` green

- [x] 6.7 Run `./tests/validate.sh` (syntax validation) — exits 0, all files valid.

### 6.8 Manual dry-run verification

- [x] 6.8 Simulated render of minimal manifest (intro+purpose only) with tdd-flow recipe enabled — output contained populated `## Workflow Rules` and `## Useful Commands` sections with `{config.test_command}` resolved to `./tests/run.sh` (fixture value). `B6RegressionTests.test_minimal_brief_with_config_key_substitution` covers this scenario as an automated test.

---

## Task Summary

| Batch | Task count | RED tasks | GREEN tasks | Notes |
|-------|-----------|-----------|-------------|-------|
| B1 | 7 | 3 (1.1, 1.3, 1.5) | 3 (1.2, 1.4, 1.6) + 1 run | Sequential TDD |
| B2 | 5 | 2 (2.1, 2.3) | 2 (2.2, 2.4) + 1 run | Sequential TDD |
| B3 | 11 | 5 (3.1, 3.3, 3.5, 3.7, 3.9) | 5 (3.2, 3.4, 3.6, 3.8, 3.10) + 1 run | Sequential TDD |
| B4 | 7 | — | 7 implementation | Parallelizable within B4 |
| B5 | 2 | — | 2 doc edits | Parallelizable within B5 |
| B6 | 8 | 2 (6.1, 6.3) | 2 (6.2, 6.4) + 4 verification | Sequential |
| **Total** | **40** | **10** | **19** + infra | |

**TDD confirmed**: every component in B1–B3 has an explicit RED task before its GREEN task.  
**Parallelizable**: tasks within B4 (4.1–4.7) and within B5 (5.1–5.2) are independent of each other.  
**Critical path**: B1 → B2 → B3 → B6 (cannot be parallelized; each depends on the previous batch).
