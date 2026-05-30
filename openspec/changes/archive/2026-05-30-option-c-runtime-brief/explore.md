# Exploration: option-c-runtime-brief

## Change Summary

Option C = enrich the `ai-specs.toml` manifest schema + `agents-render.py` renderer so the
GENERATED `AGENTS.md` equals the rich manual one, allowing the `<!-- ai-specs:runtime-brief -->`
marker to be removed and AGENTS.md to become 100% generated and reproducible.

---

## Current State

### What agents-render.py actually does today

`lib/_internal/agents-render.py` `_render_lines()` emits:
1. A heading `# AGENTS.md - Runtime context`
2. `## Project: {name}` (from `[project].name`)
3. `## How AI tooling is wired / ### MCP Servers` — loops `[mcp.*]` entries, prints
   `command`, `args`, `env` (with redaction of literal secrets).

That is the ENTIRE generated output. Nothing else. No recipes, no policy prose, no runtime flow,
no context sources, no workflow rules.

### The --preserve-if-runtime-brief escape hatch

`sync.sh` calls:
```
python3 "$AGENTS_RENDER_PY" "$TOML_PATH" "$ROOT_PATH/AGENTS.md" --preserve-if-runtime-brief
```

When the output file contains `<!-- ai-specs:runtime-brief -->`, the renderer silently returns
without overwriting. This is the mechanism that lets this repo maintain a rich manual brief.

`sync-agent.sh` calls `agents-render.py` WITHOUT `--preserve-if-runtime-brief` for subrepo
targets — so subrepos always get overwritten with the thin generated version.

### The manual AGENTS.md / CLAUDE.md sections (enumerated)

The current rich brief (`CLAUDE.md`, which is the manual `AGENTS.md`) has these sections:

| Section | Content | Data source for Option C |
|---------|---------|--------------------------|
| H1 title | `# ai-specs-cli Runtime Brief` | `[project].name` (EASY) |
| Intro blockquote | Explains purpose of this file, transition state | `[brief].intro` prose (HARD — free text) |
| `## Project` | name, manifest path, purpose, enabled runtimes, integration_branch | `[project].name` (easy); `[brief].purpose` (HARD); `[agents].enabled` (EASY); `[recipes.worktree-flow.config].integration_branch` (EASY via recipe) |
| `## Runtime MCPs` | trello, engram, vault-ai-specs + "Never expose secrets" rule | `[mcp.*]` names already rendered; human-friendly descriptions (HARD); safety rule is `[brief].rules` prose (MEDIUM) |
| `## Runtime Flow` | 5 bullet points about session/worktree conventions | Mostly `[brief].runtime_flow` prose — but `VCS/PR provider: GitHub through gh CLI` maps to `recipes.git-pr-flow.config.provider` + `base_branch` (MEDIUM) |
| `## Trello Tracking` | board_id, "Trello is source of truth" | `recipes.trello-mcp-workflow.config.board_id` (EASY); tracker name from capability binding (MEDIUM); prose context (HARD) |
| `## Context Sources` | 5 bullets about Trello/Vault/Engram/Engram/Skills authority | `[brief].context_sources` prose (HARD) |
| `## Conflict Policy` | 4 bullets about authority hierarchy | `[brief].conflict_policy` prose (HARD) |
| `## Workflow Rules` | 7 bullets with project-specific rules | Mix: integration_branch easy, most are HARD prose in `[brief].workflow_rules` |
| `## Current Transitional State` | Explains marker/manual status | TRANSIENT — goes away when Option C lands (remove from generated output) |
| `## Useful Commands` | `./tests/run.sh`, `./tests/validate.sh` | `recipes.tdd-flow.config.test_command` (EASY); both commands are already in config |

### Data already in the manifest (EASY wins)

Every single piece of structured runtime config is already in `ai-specs.toml`:
- `[project].name` → project identity
- `[agents].enabled` → enabled runtimes
- `[recipes.worktree-flow.config].integration_branch` → integration branch
- `[recipes.git-pr-flow.config].provider`, `.base_branch` → VCS/PR info
- `[recipes.tdd-flow.config].test_command` → test command
- `[recipes.trello-mcp-workflow.config].board_id` → tracker board ID
- `[recipes.vault-canonical-store.config].vault_scope` → vault scope
- `[mcp.*]` keys → which MCP servers are wired
- `[[capabilities]]` declarations via recipe catalog → capability roles

### What is MISSING from the manifest (HARD gaps)

- Free prose: purpose statement, runtime flow bullets, context source bullets, conflict policy bullets
- Workflow rule bullets (per-project policy like "do not merge without explicit instruction")
- Human-readable MCP descriptions (e.g., "project tracking through the ai-specs-cli Roadmap board")
- Section ordering and headings

---

## Section-by-Section Analysis (Question 1)

| Section | Source under Option C | Difficulty |
|---------|-----------------------|------------|
| H1 title | Computed from `[project].name` | Easy |
| Intro | `[brief].intro` free string | Medium (just a TOML string) |
| Project identity | `[project].name`, `[agents].enabled`, recipe configs | Easy |
| Runtime MCPs | `[mcp.*]` names (already rendered), descriptions from `[brief].mcp_descriptions` | Medium |
| Runtime Flow | `[brief].runtime_flow` prose block OR `[[brief.runtime_flow]]` bullets | Medium |
| Trello Tracking | `recipes.trello-mcp-workflow.config.board_id` + bound recipe name via capability lookup | Easy-Medium |
| Context Sources | `[brief].context_sources` bullets | Medium |
| Conflict Policy | `[brief].conflict_policy` bullets | Medium |
| Workflow Rules | `[brief].workflow_rules` bullets | Medium |
| Useful Commands | `recipes.tdd-flow.config.test_command` + computed validate command | Easy |

**Key insight**: The "hard" sections are not hard to implement — they are hard because they
require adding prose to the TOML manifest. Every field would be a regular TOML string or array
of strings. The real question is WHERE to put them (project-level `[brief]` vs recipe fragments).

---

## Template Interpolation (Question 2)

### Current state: no interpolation exists

Templates today are copied verbatim (`shutil.copy2`). The renderer loops over recipe fields
directly. There is ZERO existing interpolation machinery in the codebase.

### Minimum viable interpolation for Option C

The `{{ config.x }}` interpolation mentioned in the hypothesis is needed only if we go the
per-recipe brief-fragment approach. Assessment:

**Python's `str.format_map()` with a config dict** — zero dependencies, ~5 lines of code:
```python
def interpolate(template_text: str, config: dict) -> str:
    class ConfigProxy:
        def __getitem__(self, key): return config.get(key, f"{{config.{key}}}")
    return template_text.format_map({"config": ConfigProxy()})
```

This handles `{config.board_id}` → `69ec097f13e2d38ecd89a557` without any dependency.

**However**: interpolation is only necessary if recipe-fragment templates are stored as text
files. If the renderer constructs sections in Python (which it already does for MCP), no
interpolation is needed — the Python code just accesses `merged_config["board_id"]` directly.
This is the recommended approach: **compute in Python, no template files needed**.

---

## Composition Ordering and Recipe Resolution (Question 3)

### How to get active recipes + bindings

`recipe-materialize.py` already does this work completely:
- `load_recipes_from_manifest()` → enabled recipe dict
- `read_recipe(catalog_dir, rid)` → full `Recipe` dataclass with capabilities + config_schema
- `resolve_bindings()` → `capability_id → recipe_id` map
- `merge_config(recipe, manifest_config)` → final resolved config for each recipe

`agents-render.py` currently only takes `toml_path` as input. Option C needs it to also
receive `ai_specs_home` (to locate the catalog), OR the resolved bindings/config can be
passed as a pre-computed JSON blob from `sync.sh` (similar to how `--recipe-mcp` works today).

### Composition ordering

The hypothesis proposes ordering by capability tier (foundational before specific). The catalog
already has a natural ordering via the `[[capabilities]]` declarations. A practical ordering for
the generated brief:
1. Project identity (always first, from `[project]`)
2. MCP table (already emitted, keep it)
3. Recipe-contributed sections, ordered by: foundational recipes first (by declaration order in
   manifest), then specific (capability-provider) recipes
4. Project-level `[brief]` prose sections (runtime_flow, context_sources, conflict_policy,
   workflow_rules, useful_commands)

The simplest approach: fixed section order hardcoded in `_render_lines()`, reading from
manifest + resolved configs. No dynamic ordering needed.

---

## Migration and Safety (Question 4)

### Decomposition strategy

1. Add `[brief]` table to `ai-specs.toml` with all prose fields.
2. Enhance `agents-render.py` to emit the full rich format when `[brief]` is present.
3. **Golden/needle test**: add a pytest that runs `agents-render.py` on the actual
   `ai-specs/ai-specs.toml` and asserts the output contains key needle strings from the
   current manual brief (board ID, integration_branch, test_command, vault_scope).
4. Run `ai-specs sync` on this repo → compare generated output against hand-maintained file.
5. When generated ≈ manual, remove the `<!-- ai-specs:runtime-brief -->` marker.
6. Add idempotency test (already exists for the thin format in `test_sync_pipeline.py`).

### Rollback story

The `--preserve-if-runtime-brief` flag acts as the rollback: if the generated output is wrong,
just restore the marker and AGENTS.md. The marker/escape-hatch can remain as a permanent
opt-out for projects that prefer manual briefs — it costs nothing to keep.

### Test pattern precedent

`test_sync_pipeline.py` already has `test_sync_redacts_literal_mcp_secrets_in_agents_md` which
asserts needle strings in `AGENTS.md`. The new tests would follow the exact same pattern:
```python
agents = (workspace / "AGENTS.md").read_text()
self.assertIn("board_id: 69ec097f13e2d38ecd89a557", agents)  # or similar
self.assertIn("integration_branch: development", agents)
```

---

## Alternatives (Question 5)

### Option C-1: Per-recipe `[brief]` fragments (the hypothesis)

Each recipe has a `[brief]` table with `section`, `template_path`. The renderer collects
fragments from all enabled recipes and assembles them.

- **Pro**: each recipe owns its brief contribution; decentralized
- **Pro**: supports catalog recipes for third-party projects
- **Con**: adds template files to catalog (more moving parts)
- **Con**: requires interpolation machinery for `{{ config.x }}`
- **Con**: section ordering is fragile (who decides heading names?)
- **Con**: recipe `recipe.toml` schema extension is more breaking
- **Effort**: High

### Option C-2: Project-level `[brief]` table only (no recipe changes)

All prose goes into `[brief]` in `ai-specs.toml`. Structured data (board_id, test_command,
integration_branch) is pulled directly from recipe configs. No recipe schema changes.

- **Pro**: minimal schema surface — only `ai-specs.toml` gains `[brief]`
- **Pro**: no template files, no interpolation, no recipe.toml changes
- **Pro**: renderer stays simple Python — reads `[brief]` + recipe configs
- **Con**: `[brief]` prose is project-specific, not reusable across projects via recipes
- **Con**: the "Trello Tracking" section and "VCS/PR provider" bullets need recipe awareness in
  the renderer anyway (it needs to know WHICH recipe provides the tracker capability)
- **Effort**: Low-Medium

### Option C-3: Single project-level brief TEMPLATE file

A file like `ai-specs/brief-template.md` with `{config.board_id}` style tokens. The renderer
reads and fills it in.

- **Pro**: familiar Markdown editing experience for prose
- **Con**: another file to maintain, another format to specify
- **Con**: interpolation engine needed
- **Con**: not significantly simpler than C-2
- **Effort**: Medium

### Option C-4: Keep marker as permanent escape hatch (no Option C)

Keep the current approach. Accept that projects with rich briefs manage them by hand.

- **Pro**: zero implementation effort
- **Con**: AGENTS.md stays non-idempotent; `ai-specs sync` can't be safely re-run here
- **Con**: structured data (board_id, test_command) is duplicated in manifest and AGENTS.md
- **Effort**: Zero

### Recommended: Option C-2 (project-level `[brief]` table)

The structured data is the HIGH VALUE part — it eliminates the duplication of board_id,
integration_branch, test_command, vault_scope between manifest and AGENTS.md. The prose
sections are relatively small and project-specific. Putting them in `[brief]` in `ai-specs.toml`
is the smallest incremental step and needs zero recipe schema changes.

The per-recipe fragment idea (C-1) is attractive long-term (for third-party catalog recipes to
self-document), but it adds significant complexity and is not needed to solve THIS project's
immediate problem.

---

## Scope Boundaries (Question 6)

### In scope for Option C

1. New `[brief]` table in `ai-specs.toml` manifest schema (documented in `docs/ai-specs-toml.md`).
2. Enrich `agents-render.py` `_render_lines()` to read:
   - `[brief].*` for prose fields
   - `[recipes.*]` + catalog for structured fields (board_id, test_command, integration_branch, etc.)
   - Capability bindings to name the tracker/canonical-store/vcs provider
3. Update `agents-render.py` to accept `ai_specs_home` path (to read catalog for capability resolution)
   OR receive a pre-computed bindings JSON (simpler, like `--recipe-mcp` pattern).
4. Remove the `<!-- ai-specs:runtime-brief -->` marker from this repo's AGENTS.md once generated ≈ manual.
5. Tests: golden/needle test for generated AGENTS.md content, idempotency test.
6. Docs: update `docs/ai-specs-toml.md` with `[brief]` table reference.

### Explicitly deferred / out of scope

- **Per-recipe `[brief]` fragments** (C-1 approach) — deferred to a future "recipe brief contributions" change.
- **Auto-invoke table in AGENTS.md** — this was referenced in sync.sh comments and `ai-specs.toml` comments (`# auto_invoke (opt) trigger phrases for the AGENTS.md Auto-invoke table`). The `skill-registry-artifact` spec is now RETIRED and the `.skill-registry.md` file no longer generated. The `AGENTS.md` today does NOT contain an auto-invoke table. This comment in the manifest is stale/aspirational. Decision: **leave as-is for Option C** — the auto-invoke mechanism is already handled via SKILL.md frontmatter read by agents directly; no table needed in AGENTS.md.
- **`skills-as-rules.md` Auto-invoke table** — per the skill and CLAUDE.md, this is handled by skill-sync and the SKILL.md `auto_invoke` frontmatter, not by agents-render.py. Out of scope.
- **Recipe schema `[brief]` extension** — deferred.
- **Subrepo behavior** — subrepos already receive the thin generated output (no marker). Option C makes the ROOT project's output richer; subrepos benefit automatically once the renderer is enhanced (since sync-agent.sh calls agents-render.py without `--preserve-if-runtime-brief`).

---

## Affected Areas

- `lib/_internal/agents-render.py` — main renderer to enrich; needs access to recipe catalog
- `ai-specs/ai-specs.toml` — add `[brief]` table with prose fields
- `docs/ai-specs-toml.md` — document `[brief]` table
- `tests/test_sync_pipeline.py` — add golden/needle tests for rich generated output
- `CLAUDE.md` (= `AGENTS.md`) — remove marker once generated output matches

## Files Inspected

- `/Users/robert/proyectos/nnodes/ai-specs-cli/CLAUDE.md` — the rich manual brief (this repo's AGENTS.md)
- `/Users/robert/proyectos/nnodes/ai-specs-cli/lib/_internal/agents-render.py` — current thin renderer
- `/Users/robert/proyectos/nnodes/ai-specs-cli/lib/sync.sh` — pipeline; --preserve-if-runtime-brief call site
- `/Users/robert/proyectos/nnodes/ai-specs-cli/lib/sync-agent.sh` — fan-out; no --preserve flag for subrepos
- `/Users/robert/proyectos/nnodes/ai-specs-cli/ai-specs/ai-specs.toml` — full manifest with all recipe configs
- `/Users/robert/proyectos/nnodes/ai-specs-cli/docs/capabilities.md` — capability model
- `/Users/robert/proyectos/nnodes/ai-specs-cli/docs/recipe-schema.md` — recipe.toml contract
- `/Users/robert/proyectos/nnodes/ai-specs-cli/docs/ai-specs-toml.md` — manifest contract
- `/Users/robert/proyectos/nnodes/ai-specs-cli/lib/_internal/recipe_schema.py` — Recipe dataclass
- `/Users/robert/proyectos/nnodes/ai-specs-cli/lib/_internal/recipe-materialize.py` — resolve_bindings(), merge_config()
- `/Users/robert/proyectos/nnodes/ai-specs-cli/lib/_internal/mcp-render.py` — rendering pattern to mirror
- `/Users/robert/proyectos/nnodes/ai-specs-cli/catalog/recipes/worktree-flow/recipe.toml`
- `/Users/robert/proyectos/nnodes/ai-specs-cli/catalog/recipes/trello-mcp-workflow/recipe.toml`
- `/Users/robert/proyectos/nnodes/ai-specs-cli/catalog/recipes/tdd-flow/recipe.toml`
- `/Users/robert/proyectos/nnodes/ai-specs-cli/catalog/recipes/git-pr-flow/recipe.toml`
- `/Users/robert/proyectos/nnodes/ai-specs-cli/catalog/recipes/vault-canonical-store/recipe.toml`
- `/Users/robert/proyectos/nnodes/ai-specs-cli/catalog/recipes/session-context/recipe.toml`
- `/Users/robert/proyectos/nnodes/ai-specs-cli/tests/test_sync_pipeline.py` — existing test patterns
- `/Users/robert/proyectos/nnodes/ai-specs-cli/tests/run.sh` and `validate.sh` — test commands
- `/Users/robert/proyectos/nnodes/ai-specs-cli/openspec/config.yaml` — strict_tdd: true

---

## Risks

1. **agents-render.py needs catalog access** — today it only reads `ai-specs.toml`. To resolve
   capability bindings (which recipe provides `tracker`?) it needs `ai_specs_home`. The cleanest
   approach: pass a pre-computed `--resolved-config <json>` file from sync.sh (like --recipe-mcp
   already does). This avoids adding catalog dependency to the renderer. Risk: adds another temp
   file to the pipeline. Mitigation: small scope, same pattern as existing --recipe-mcp.

2. **Prose in TOML is awkward for long text** — TOML multiline strings are verbose. Long policy
   bullets in `[brief].workflow_rules` become noisy. Mitigation: use arrays of strings for bullet
   lists; prose intro as a single multi-line TOML string. Document the pattern clearly.

3. **Migration atomicity** — the marker removal and the `[brief]` population must happen
   together. If they're committed separately and sync runs between, AGENTS.md gets overwritten
   with the old thin format. Mitigation: do them in the same commit, or keep marker until the
   `[brief]` data is confirmed correct.

4. **Subrepo test fixture** — existing sync tests check that subrepos contain `fixture-sync` in
   AGENTS.md. New sections added by `[brief]` won't break this unless the renderer changes the
   heading format. Low risk.

5. **Scope creep into per-recipe fragments** — the hypothesis describes a larger vision. Staying
   strictly at C-2 (project-level `[brief]` only) keeps this manageable. Risk of over-engineering
   if the proposal tries to do C-1 too.

---

## Ready for Proposal

Yes. The design is clear enough to propose. The recommended approach (C-2) is well-scoped and
the changes are concentrated in 2-3 files.
