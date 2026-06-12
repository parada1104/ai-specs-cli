# Design: recipe-brief-fragments

## Technical Approach

Recipes declare prose fragments under `[provides.brief]`. `recipe_schema.py` parses+normalizes
them to `{key, text}`; `recipe-materialize.py` emits them **raw** into `resolved-config.json` per
recipe; `agents-render.py` collects (in `enabled` order), dedupes, applies `{config.KEY}`
substitution, and merges with manifest `[brief]` (append default / `<section>_mode="replace"`).
Substitution happens at **render time** (config lives beside fragments in `resolved-config`), so
the materialized JSON stays raw and portable. Components are pure functions for unit isolation.

## Architecture Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Where substitution runs | Render time (`agents-render.py`) | resolved-config stays raw; same fragment text rendered against the recipe's own merged config; no double-escaping in JSON |
| Substitution mechanism | Custom `Mapping` + `str.format_map` over a `config.`-prefixed view | Only `{config.KEY}` is substitutable; missing key → verbatim; bare `{KEY}` and prose braces survive via `{{`/`}}` |
| Normalization location | `recipe_schema.py` (`_parse_brief_fragments`) | Single parse path; renderer/materialize consume typed `{key,text}` only |
| Fragment container | `BriefFragments` dataclass, `Optional[List[BriefFragment]]` per section | Mirrors existing `Recipe` dataclass style; `None` = recipe omitted slot |
| `mcp_descriptions` precedence | project `[brief]` wins; recipe fills gap | Confirmed decision #1; renderer merges into existing `mcp_descriptions` dict before `_section_mcp` |

## Data Model (recipe_schema.py)

```python
CONTRIBUTABLE_SECTIONS = ("runtime_flow","context_sources","conflict_policy",
                          "workflow_rules","useful_commands","mcp_descriptions")
PROJECT_ONLY_SECTIONS = ("intro","purpose")

@dataclass
class BriefFragment:
    text: str
    key: str | None = None

@dataclass
class BriefFragments:           # one Optional[list[BriefFragment]] field per section
    runtime_flow: list[BriefFragment] | None = None
    context_sources: list[BriefFragment] | None = None
    conflict_policy: list[BriefFragment] | None = None
    workflow_rules: list[BriefFragment] | None = None
    useful_commands: list[BriefFragment] | None = None
    mcp_descriptions: list[BriefFragment] | None = None
```
`Recipe` gains `brief_fragments: BriefFragments | None = None`.

`mcp_descriptions` form: inline-tables `{key=<server>, text=<desc>}` (key = server name).

### `_parse_brief_fragments(raw, context) -> BriefFragments | None`
Returns `None` if `raw` is absent. For each section key: reject `intro`/`purpose`
(`"{ctx}.{name}: section is project-only; recipes MUST NOT contribute it"`); reject unknown
(`"...: unknown section '{name}'; valid: {CONTRIBUTABLE_SECTIONS}"`). Per value:
- list of `str` → `BriefFragment(text=s, key=None)`; empty list → `[]`.
- list of `dict` (inline-tables, from `[[provides.brief.<section>]]`) → require `text`
  (`"...[{i}]: missing required field 'text'"`) and `key`
  (`"...[{i}]: missing required field 'key'"`); → `BriefFragment(text=..., key=...)`.
- Mixed/other → `"...: section '{name}' mixes string-array and inline-table forms"` /
  type error. Wired into `validate_recipe_toml` via `provides.get("brief")`.

## recipe-materialize.py

In `build_resolved_config()` the per-recipe `config` dict is assembled from raw manifest keys —
**no catalog read**, so fragments aren't available there. Add a catalog-aware enrichment: after
`recipes_out[rid]` is built, when a catalog is reachable, load the recipe and attach
`recipes_out[rid]["brief_fragments"] = _fragments_to_json(recipe.brief_fragments)`. Helper:
```python
def _fragments_to_json(bf) -> dict[str, list[dict]]:
    # {section: [{"key": f.key, "text": f.text}, ...]} ; omit None sections ; {} if bf is None
```
Attach in BOTH `materialize_recipes` (has `recipe` already in the enabled loop) and
`build_resolved_config_only` (its existing catalog block). Recipes without `[provides.brief]` →
key absent or `{}`. Raw text (no substitution) is stored.

## resolved-config.json contract delta

```jsonc
"recipes": { "worktree-flow": {
    "integration_branch": "main",
    "brief_fragments": { "workflow_rules": [ {"key": null, "text": "Do not push to `{config.integration_branch}`..."} ] }
}}
```
Backward-compat: `brief_fragments` absent ⇒ no fragments; renderer treats missing as `{}`.

## agents-render.py

New pure helpers (module level, fully unit-testable):

```python
def collect_recipe_brief_fragments(resolved, section) -> list[dict]:
    out, seen_keys, seen_text = [], set(), set()
    for rid in resolved.get("enabled", []):
        rcfg = resolved["recipes"].get(rid, {}) or {}
        cfg_ns = {f"config.{k}": v for k, v in rcfg.items() if k != "brief_fragments"}
        for frag in (rcfg.get("brief_fragments", {}) or {}).get(section, []):
            key, raw = frag.get("key"), frag.get("text", "")
            if key is not None and key in seen_keys: continue        # key dedup, first wins
            text = substitute_config(raw, cfg_ns)
            if text in seen_text: continue                            # exact-string dedup
            if key is not None: seen_keys.add(key)
            seen_text.add(text); out.append({"key": key, "text": text})
    return out                                                        # [{key,text}] substituted
```

```python
def substitute_config(text, cfg_ns) -> str:
    class _M(dict):
        def __missing__(self, k):
            return "{" + k + "}" if k.startswith("config.") else "{" + k + "}"
    # format_map handles {{ }} escape natively; only config.* keys resolve, all else verbatim
    try:
        return text.format_map(_M(cfg_ns))
    except (ValueError, IndexError):
        return text   # malformed single brace in prose → leave untouched, never crash
```
`format_map` already turns `{{`/`}}` into literal `{`/`}` and raises `KeyError` only for known
field names — `__missing__` re-emits any unknown placeholder verbatim (covers both `{config.X}`
unknown and bare `{integration_branch}`). The `try` guards lone unbalanced braces in prose.

**Section integration** — each `_section_*(brief, resolved)` gains:
```python
mode = brief.get(f"{section}_mode", "append")        # validated elsewhere
recipe_items = [] if mode == "replace" else collect_recipe_brief_fragments(resolved, section)
manifest_items = brief.get(section, []) or []         # NEVER substituted
bullets = [f["text"] for f in recipe_items]
for m in manifest_items:                              # exact-string dedup vs recipe text
    if m and m not in bullets: bullets.append(m)
```
Functions touched: `_section_runtime_flow`, `_section_context_sources`, `_section_conflict_policy`,
`_section_workflow_rules`, `_section_useful_commands` (append after test_command bullet). Signatures
that currently take only `brief` (context_sources/conflict_policy/workflow_rules) gain `resolved`.
`_section_intro`/`_section_project` (purpose) **untouched** — never substituted.

**mcp_descriptions** — in `_render_lines`, before calling `_section_mcp`, build an effective map:
recipe-collected descriptions fill gaps, manifest `brief.mcp_descriptions` overrides:
```python
eff = {f["key"]: f["text"] for f in collect_recipe_brief_fragments(resolved, "mcp_descriptions")}
eff.update(brief.get("mcp_descriptions", {}) or {})   # project wins
brief = {**brief, "mcp_descriptions": eff}             # local copy, no mutation
```
(`_mode` not applicable to mcp_descriptions — override-fills-gap only.)

**`_mode` validation**: in `render()` (or a small `_validate_brief_modes(brief)`), any
`<section>_mode` not in `{"append","replace"}` raises with section name + valid values.

## Doc target

- **`docs/recipe-schema.md`** — new `### [provides.brief]` subsection under `[provides]`: both
  forms (TOML examples), contributable-sections table, `intro`/`purpose` exclusion callout,
  `{config.KEY}` substitution + `{{`/`}}` escape worked example, missing-key→verbatim note.
- **`docs/ai-specs-toml.md`** — update the `[brief]` table (rows now "augment recipe fragments");
  add `<section>_mode` rows, append/replace semantics, and `mcp_descriptions` override-fills-gap.

These are the existing canonical homes (recipe author side vs. manifest side); no new files.

## Testing Strategy (strict TDD)

| File | Component | Cases |
|------|-----------|-------|
| `tests/test_recipe_schema.py` (existing) | `_parse_brief_fragments` | absent→None; simple-array→key=None; inline-table→key set; both sections; empty `[]`; reject intro; reject purpose; reject unknown; missing `text`; missing `key`; mixed-form |
| `tests/test_agents_render_brief_fragments.py` (NEW) | collect/dedupe/substitute/merge/mcp | enabled-order; reversed-order; key-dedup first-wins; exact-string dedup (recipe×recipe, recipe×manifest); append default; replace suppresses one section; replace isolates others; `{config.KEY}` resolved; missing key verbatim; bare `{KEY}` verbatim; `{{`/`}}` escape; mixed escape+sub; manifest prose never substituted; empty `[brief]` end-to-end render; recipe w/o fragments == identical; mcp override; mcp gap-fill; mcp none→no desc; unknown `_mode`→error; idempotent twice |

Tests load modules via the existing `load_module(path,name)` helper (filename has a hyphen).
Each spec scenario maps 1:1 to a case above.

## Edge cases & failure modes

- Brace-in-code bullet (`` `{config.x}` ``) → resolves; literal braces need `{{`/`}}`.
- Lone `{` in prose → `try/except` returns text untouched (no crash).
- `_mode` typo → fail-fast at render with valid values listed.
- Recipe declaring `intro`/`purpose` → schema rejects at parse.
- Duplicate `key` across recipes → first (earlier in `enabled`) wins.
- Disabled recipe → not in `enabled`, contributes nothing.
- Missing config key → placeholder verbatim, render continues.

## Sequencing

1. `recipe_schema.py` (dataclasses + `_parse_brief_fragments` + wiring) — no upstream deps.
2. `recipe-materialize.py` (`_fragments_to_json` + attach in both paths) — depends on (1).
3. `agents-render.py` (collect/substitute/merge/mcp/mode-validate) — depends on JSON shape from (2).
4. Catalog `[provides.brief]` blocks + scaffold `[brief]` reduction + docs — depend on (1)-(3).

## Migration / Rollout

No data migration. `<!-- ai-specs:runtime-brief -->` marker still suppresses regeneration;
absent `brief_fragments` ⇒ prior output. Reversible per-file via git.

## Open Questions

- None — all three decisions resolved (#750).
