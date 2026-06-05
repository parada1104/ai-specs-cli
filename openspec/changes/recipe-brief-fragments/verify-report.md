# Verification Report — recipe-brief-fragments

**Change**: recipe-brief-fragments
**Mode**: Strict TDD
**Verdict**: **PASS WITH WARNINGS**

---

## Completeness

| Metric | Value |
|--------|-------|
| Tasks total | 40 |
| Tasks complete | 40 (all `[x]`) |
| Tasks incomplete | 0 |

Every task realized in the tree and proven by tests. No checked task whose work is absent.

---

## Build & Tests Execution (independent re-run)

- **`./tests/run.sh`** (`python3 -m unittest discover`): PASS — `Ran 512 tests in 76.7s` / `OK` / exit 0
- **`./tests/validate.sh`**: PASS — py_compile + bash -n + full suite clean / exit 0
- Targeted modules:
  - `test_agents_render_brief_fragments` + `test_recipe_schema` + `test_recipe_materialize`: 138/138 OK
  - catalog recipe tests (worktree/git-pr/tdd/session/vault/trello/manifest-docs): 24/24 OK

---

## Spec Scenario Coverage

Covered: 17 / 17 core behaviors confirmed with a test or direct re-derivation.

| Behavior | Test / evidence |
|----------|-----------------|
| Two fragment forms | `test_simple_array_normalizes_key_none`, `test_inline_table_form_normalizes_with_key` |
| intro/purpose rejection | `test_intro_in_brief_raises_project_only_error`, `test_purpose_...` (+ live loader check) |
| unknown-section rejection | `test_unknown_section_raises_error_with_valid_list` (+ live) |
| missing-text rejection | `test_inline_table_missing_text_raises_error` |
| missing-key rejection | `test_inline_table_missing_key_raises_error` |
| mixed-form rejection | `test_mixed_forms_raises_error` |
| merge order by enabled list | `test_enabled_order_preserved`, `test_reversed_enabled_order` |
| key dedupe | `test_key_dedup_first_wins` |
| exact-string dedupe | `test_exact_string_dedup_across_recipes`, `test_exact_string_dedup_recipe_vs_manifest` |
| append default | `test_append_default_recipe_before_manifest`, `*_append` per section |
| `<section>_mode=replace` | `test_replace_mode_suppresses_recipe_fragments`, `test_replace_mode_isolates_other_sections`, `test_replace_mode_in_full_render` |
| `{config.KEY}` substitution | `test_known_key_resolves`, `test_substitution_applied`, e2e dogfood render |
| `{{`/`}}` escape | `test_double_brace_escape`, `test_mixed_escape_and_substitution` (+ live `{{config.x}}`→`{config.x}`) |
| missing-key verbatim | `test_missing_key_verbatim`, `test_missing_key_no_crash` |
| lone-brace no-crash | `test_lone_unbalanced_brace_no_crash` (+ live `a { b`, `a } b`) |
| manifest prose never substituted | `test_manifest_prose_never_substituted` |
| mcp_descriptions override-fills-gap | `test_project_override_wins`, `test_recipe_fills_gap`, `test_multi_recipe_non_overlapping_keys`, `test_no_mcp_descriptions_no_crash` |
| marker suppression | `test_marker_suppresses_regeneration_with_recipe_fragments` |
| recipe-without-fragments backward compat | `test_recipe_without_provides_brief_does_not_break_render`, `test_no_fragments_backward_compat` |
| idempotency | `test_idempotency_with_config_substitution`, `test_exact_string_dedup_idempotency` |

No core spec scenario is left without a test.

---

## End-to-End Render Check (real render of dogfood manifest)

Ran `recipe-materialize.py --resolved-config-only` then `agents-render.py` against
`ai-specs/ai-specs.toml`. Result: PASS.
- All `{config.KEY}` resolved to real values: `development` (integration_branch / base_branch),
  `github` (provider), `./tests/validate.sh` (test_command).
- No literal `{config...}` left, no unrendered `{{`.
- Empty-from-manifest sections populated by recipe fragments (Workflow Rules, Context Sources,
  Conflict Policy all carry recipe-contributed bullets).
- 6 recipes enabled; bindings auto-resolved correctly.

---

## Dogfood Regression — intent diff (before vs after migration)

No-loss is MOSTLY confirmed. Most removed manifest bullets are re-provided by recipes:
- "Trello is the source of truth..." → trello-mcp-workflow (key `trello-source-of-truth`) ✓
- "Current explicit human instruction controls..." → session-context (`conflict-policy-source-authority`) ✓
- "Create a dedicated worktree..." / "Preserve unrelated worktree changes..." → worktree-flow (exact) ✓
- "Do not merge or push to `development` without explicit human instruction." → worktree-flow
  superset "...without a PR and explicit human instruction." ✓
- "Tracker controls work state; vault controls..." → session-context, genericized (Trello→Tracker,
  Vault→vault) ✓ acceptable relocation
- branch/board/vault values still correct (`development`, board id, vault scope).

**Two genuine intent changes (WARNING):**
- DROPPED, not re-provided: "Follow the project's designated workflow for structured changes." —
  removed from `[brief].workflow_rules` and no recipe re-provides it. Net loss of one (generic/SDD)
  instruction from the rendered brief.
- SOFTENED + relocated: "Before final verification, run the relevant focused tests plus
  `./tests/validate.sh` when feasible." replaced by tdd-flow's "Run the full test suite before
  committing..." The explicit validate.sh reference now lives only in `## Useful Commands`. Intent
  (run tests before commit) preserved; the specific pre-verification emphasis is weaker.

No duplication in output (exact-string dedup working). `useful_commands_mode = "replace"` correctly
prevents the auto-generated test line from duplicating tdd-flow's command fragment.

---

## Genericity Audit (catalog `[provides.brief]` bullets)

No leaks. Every project-specific value is a `{config.KEY}` placeholder whose key exists in that
recipe's `[config.*]`:
- worktree-flow → `{config.integration_branch}` (config.integration_branch present) ✓
- git-pr-flow → `{config.base_branch}`, `{config.provider}` (both present) ✓
- tdd-flow → `{config.test_command}` (present) ✓
- trello-mcp-workflow, session-context, vault-canonical-store → keyed prose, no hardcoded
  `development`/board ids/vault scope ✓

Note: the apply-progress engram note says vault-canonical-store has "no [provides.brief]"; in fact
it DOES contribute `context_sources` + `mcp_descriptions`. Harmless note inaccuracy, code is correct.

---

## Doc Accuracy

Docs describe shipped behavior and match the code.
- `docs/recipe-schema.md`: both forms, contributable sections table, intro/purpose exclusion,
  `{config.KEY}` substitution rules, `{{`/`}}` escape worked example, key/dedup semantics.
- `docs/ai-specs-toml.md`: `[brief]` table, per-section `_mode` append/replace, mcp_descriptions
  override-fills-gap with worked example + migration note.
- `templates/ai-specs.toml.tmpl`: `[brief]` reduced to intro/purpose with comment directing authors
  to recipes and mentioning `<section>_mode = "replace"`.

---

## Edge / Robustness

- intro/purpose, unknown section, missing key, missing text, mixed-form → all rejected with
  explicit errors (confirmed live + tests).
- `<section>_mode` typo (`merge`) → rejected at render with valid-values message.
- substitution never crashes on weird input: lone `{`, lone `}`, `{config.}`, nested `{{...}}`.

**Minor robustness gap (SUGGESTION):** a misspelled section in a `_mode` key (e.g.
`workflo_rules_mode = "replace"`) is silently accepted (ends in `_mode`, value valid) but matches no
section, so it is a no-op with no warning. Out of spec scope; low risk.

---

## Critical: none

## Warnings
1. Dropped instruction "Follow the project's designated workflow for structured changes." not
   re-provided by any recipe — silent net loss of one rule from the dogfood brief.
2. "Before final verification, run ... `./tests/validate.sh` when feasible." softened to a generic
   tdd-flow rule; explicit pre-verify validate.sh emphasis weakened (relocated to Useful Commands).

## Suggestions
1. Add a generic SDD/structured-change rule to a recipe (or keep it in the manifest) to recover the
   dropped workflow rule, if it is still considered load-bearing.
2. Consider warning on `_mode` keys whose section prefix is not a known contributable section.

---

**Next recommended**: archive (warnings are intentional-migration judgment calls, not defects).
Optionally restore the one dropped workflow rule first if it is still wanted.
