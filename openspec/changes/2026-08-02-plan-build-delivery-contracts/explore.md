# Explore: plan-build-delivery-contracts

Phase: **explore** (sdd-explore subagent, read-only, 2026-08-02)
Branch: `feat/plan-build-delivery-contracts` — Worktree: `/Users/robert/proyectos/nnodes/ai-specs-cli-plan-build-delivery-contracts`
Base: `development@312f985`
Tracker: Trello #58 (`OdQPZZa3`)

## Problem framing

The `plan-build-flow` recipe currently declares the mechanics of planning and implementation,
but not the project's delivery defaults. Its bundled skill can classify change depth and gate
PR/archive transitions, while the generated project brief can carry ambient workflow rules.
There is no declarative answer today for where SDD artifacts belong or how much review workload
is acceptable. As a result, a runtime's session preflight has to fall back to its own defaults,
even when a repository has a different policy.

This change adds two repo-owned delivery contracts:

- `artifact_store_default`: `openspec`, `engram`, or `both`.
- `review_budget_lines`: an integer review-budget threshold.

The recipe remains an orchestrator-agnostic spec/tool integration. It declares defaults and
renders them into `AGENTS.md`; an external runtime may consume those rules during its own
interactive preflight. The recipe does not invoke, import, or depend on that runtime.

## Findings by target

### A. Current plan-build-flow surface

**Recipe declaration.** `catalog/recipes/plan-build-flow/recipe.toml:1-39` defines recipe
metadata, capability `plan-build-flow`, version `1.2.0`, an on-sync `validate-config` hook,
the bundled `plan-build-flow` skill, five `provides.brief.workflow_rules`, one blocking
`pre-tool-use` hook, and the generated README. There is no `[config.*]` table today.
The five rules establish: depth classification and authorization; planning before direct
implementation; minimum committed artifacts before PR; worktree-aware implementation; and
archive before merge (`recipe.toml:20-27`).

**Bundled skill.** `catalog/recipes/plan-build-flow/skills/plan-build-flow/SKILL.md:22-33`
defines the ambient plan/build contract and intentionally exposes no slash commands. Its
classifier at `SKILL.md:38-60` selects exactly Full, Standard, or Light. Full requires
`explore → proposal → spec → design → tasks`; Standard requires `spec → tasks`; Light
requires `tasks.md`. The skill also says planning stops for authorization before production
edits, and that an absent orchestrator or memory store is handled by equivalent inline or
file-artifact behavior (`SKILL.md:62-92`).

The PR gate at `SKILL.md:94-114` requires a matching change folder, tier-minimum files, and
completed implementation/verification before PR creation. The archive-tail and merge guardian
at `SKILL.md:116-157` require archiving on the review branch before merge and reject an active
change folder or incomplete archive. These are artifact/phase gates, not review-line budget
gates.

**Runtime hook.** `catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh:1-25` documents a
portable normalized JSON pre-tool-use contract. It blocks production edits only when there is
no active non-archived `openspec/changes/*/tasks.md`; planning files, tests, docs, and agent
configuration remain allowed. The implementation resolves the target relative to the Git root
and gates only configured production top-level directories (`plan-build-gate.sh:50-85`). It
allows production edits once any active tasks file exists and otherwise exits 2 with a
plain-language instruction to write the plan (`:87-100`). It is fail-open for parse/lookup
errors and has no configuration switch. Crucially, it does not inspect forecasted changed
lines and does not enforce a review budget.

**Recipe README and root README.** The recipe README repeats the classifier and PR/archive
contracts (`catalog/recipes/plan-build-flow/README.md:1-35`), enables the recipe with a
versioned manifest example (`:37-45`), and currently says “Config: None” (`:47-50`). The
repository README describes recipes as catalog bundles materialized by `ai-specs sync` and
points users to the recipe catalog and schema docs (`README.md:142-149`); it does not define
plan-build delivery defaults. The catalog row in `docs/recipes-catalog.md` is therefore a
second documentation surface to update in apply.

### B. Brief materialization and interpolation

`lib/_internal/agents-render.py:60-93` implements `substitute_config`. It recognizes
`{config.KEY}` with a regex, substitutes a present key, preserves an unknown config placeholder
verbatim, leaves bare `{KEY}` untouched, supports `{{`/`}}` escapes, and does not crash on
unbalanced prose braces. `collect_recipe_brief_fragments` (`agents-render.py:96-143`) walks
enabled recipes in manifest order, builds a namespace from each resolved recipe config as
`config.<key>`, substitutes each fragment, then performs key and exact-text deduplication.
Thus a recipe rule containing `{config.artifact_store_default}` or
`{config.review_budget_lines}` will render with that recipe's merged values, without changing
manifest prose or requiring special orchestrator code.

`lib/_internal/recipe-materialize.py:488-529` merges schema defaults first, then overlays
manifest values, warns and ignores unknown keys, checks required fields, and validates enum
constraints. The materialization path subsequently carries the merged recipe configuration into
resolved recipe data used by brief rendering. The existing implementation therefore provides
the required propagation path: schema default/override → resolved recipe config → brief
namespace → generated AGENTS.md.

The direct precedent is `catalog/recipes/git-pr-flow/recipe.toml:25-41`: `[config.base_branch]`
has type `string`, default `main`, and explanatory `help_text`; its workflow rules interpolate
`{config.base_branch}` at `:47-54`. The new fields should follow this shape rather than adding a
new renderer or recipe-specific substitution mechanism.

### C. Config schema support

`lib/_internal/recipe_schema.py:109-121` models a `ConfigField` with `required`, `type`,
`default`, `validation`, optional `enum`, and `help_text`; `ConfigSchema.fields` stores the
parsed field map. `_parse_config` begins at `recipe_schema.py:441`; standard config entries are
`[config.<key>]` tables identified by `required`, while non-standard tables are retained in
`extra`. The parser validates field shape and supported primitive types, parses defaults,
validation, enum values, and `help_text` (the `help_text` extraction is at `:499`, construction
at `:501-508`). Existing recipes demonstrate `string`, `bool`, and integer fields; the target
fields need only the existing `string` and `int` types.

The proposed schema is:

```toml
[config.artifact_store_default]
required = false
type = "string"
default = "openspec"
enum = ["openspec", "engram", "both"]
help_text = "Default artifact store ..."

[config.review_budget_lines]
required = false
type = "int"
default = 400
help_text = "Review workload budget in changed lines ..."
```

The exact wording belongs to proposal/design, but defaults must preserve current behavior. An
optional enum is preferable for the artifact-store contract because the allowed values are a
closed vocabulary; the integer field must be validated as an integer by the existing schema
parser and should reject malformed manifest values rather than silently stringifying them.
Both fields are optional with defaults, so projects that add the recipe without configuration
remain backwards compatible.

### D. Affected test surface

`tests/test_plan_build_flow_recipe.py:69-100` verifies skill-only materialization, hooks, and
forbidden vocabulary. `test_recipe_adds_no_schema_surface` currently asserts zero config fields
at lines 91-96; it is intentionally obsolete once the two fields exist and must be replaced by
schema assertions for names, types, defaults, and the artifact-store enum. The same file checks
brief vocabulary and materialized README content (`:109-125`), worktree and classifier rules
(`:127-136`, `:190-200`), and PR/archive gate vocabulary (`:168-180`). New brief rules must
remain free of forbidden recipe-layer terms used by these tests and preserve those existing
contracts.

`tests/test_agents_render_brief_fragments.py:37-86` already proves known-key interpolation,
missing-key preservation, bare-key preservation, brace escaping, mixed substitutions, and
unbalanced-brace safety. Its collection tests (`:90-183`) prove enabled-order behavior,
key/exact-string deduplication, and empty/disabled recipe handling. Add focused coverage for
both new keys (including an integer rendered as text) and an end-to-end resolved recipe
fragment, rather than changing the renderer.

`tests/test_brief_render_policy.py` exercises the policy for project and recipe brief fragments
and should verify the new delivery rules are appended with the expected substituted values,
without substituting project-authored prose. `tests/test_runtime_brief_baseline.py` validates
the generated AGENTS.md baseline and is the highest-risk regression surface: if it asserts exact
workflow-rule lists or exact snapshots, adding rules will require updating the expected baseline
while preserving ordering and unrelated recipe fragments. Search and update only the precise
expectations; do not weaken exact-list assertions merely to make the change pass.

The focused recipe, renderer, brief-policy, and baseline tests are the relevant verification
surface. No project-wide test suite is required for this exploration or apply phase.

### E. External consumption context (not a recipe dependency)

An external `gentle-ai` SDD runtime currently asks four interactive preflight questions. Its
verified defaults are: artifact store `openspec`, review budget `400`, execution mode
`interactive`, and chained PR strategy `auto-forecast` (session context, verified technical
facts; external implementation is `~/.pi/agent/npm/node_modules/gentle-pi/lib/sdd-preflight.ts`).
The runtime's preflight is session-interactive and remains outside this repository's ownership.

The intended consumption is simple: the generated project brief tells the orchestrator agent
which per-project defaults to answer when that external preflight asks. The agent supplies the
recipe's rendered `artifact_store_default` and `review_budget_lines`; an external runtime may
then use those answers. This is external runtime behavior we do not control, not a dependency
of `plan-build-flow`, and not a reason to modify gentle-pi/gentle-ai code. When no project
configuration is declared, the recipe defaults reproduce the current external defaults.

The contract deliberately excludes execution mode and chained PR strategy. The external
preflight continues to ask those questions and owns their defaults and semantics. The recipe
must not name or declare `chained_pr_default`, must not declare an execution-mode field, and
must not implement preflight logic.

### F. Gap analysis

The apply phase needs to add:

1. Two optional recipe config fields in `catalog/recipes/plan-build-flow/recipe.toml`, with
   defaults, supported types, artifact-store enum, and useful help text.
2. Brief workflow rules using `{config.artifact_store_default}` and
   `{config.review_budget_lines}`. Rules should state that the project defaults are the values
   to provide when an external session preflight asks, while preserving the recipe's
   orchestrator-agnostic language. They should also state how the review budget relates to
   plan/build review forecasting or gates, if design confirms a budget warning contract.
3. Recipe README documentation describing the fields, accepted values, defaults, and the fact
   that `ai-specs sync` materializes the rules. Update the catalog documentation row similarly.
4. Tests for schema parsing/defaults/enum rejection, manifest override and integer behavior,
   rendered interpolation, generated brief policy/baseline expectations, and preservation of
   existing plan-build vocabulary and gates.
5. A recipe version decision and, if policy requires it, a version bump from `1.2.0` with
   corresponding test fixtures/docs updated consistently.

The apply phase must not add `chained_pr_default`, an execution-mode config, preflight questions
or preflight code, gentle-ai/gentle-pi references in recipe code/docs/tests, an orchestrator
runtime dependency, or a new renderer path. It must not make `plan-build-gate.sh` invoke or
understand an external preflight. It must not change the existing requirement that planning
artifacts precede production edits, PR creation, or archive/merge transitions.

### G. Interaction with tracker-card-gate brief

The archived tracker-card-gate exploration shows that recipe workflow rules are ambient and can
coexist with hard file/shell gates. The new rules should be additive and independently keyed (if
keys are used), so enabling both recipes does not cause accidental key-based deduplication or
replace one policy with another. `agents-render.py` preserves enabled recipe order and performs
first-key-wins deduplication (`agents-render.py:104-141`); choose stable, unique keys and avoid
reusing tracker vocabulary. The plan-build contract should describe delivery defaults without
turning tracker availability into a prerequisite or changing tracker-card-gate's own soft/hard
policy.

## Risks and unknowns

- **Exact baseline expectations (high):** `test_runtime_brief_baseline.py` may compare exact
  rule lists or generated AGENTS.md. New rules change the committed baseline and could affect
  ordering with tracker-card-gate fragments. Update intentional expectations, not the renderer's
  generic merge policy.
- **Recipe version policy (medium):** adding config and materialized brief behavior may require
  bumping `recipe.version` beyond `1.2.0`; the repository currently has no decision in this
  context on whether catalog recipe versions are bumped for additive config. Proposal/design
  must settle the version and update manifest fixtures or docs that pin it.
- **Default compatibility (medium):** `openspec`/`400` must remain defaults when a project has
  no overrides. Incorrect merge ordering could make an absent key disappear or allow an unknown
  manifest key to alter the brief; existing `merge_config` behavior should be preserved.
- **Type and enum validation (medium):** TOML integer parsing and schema validation must remain
  strict. A malformed budget or unsupported artifact-store value should fail predictably during
  config handling, while omitted fields resolve to defaults.
- **Budget semantics (medium/high):** the current plan-build gate only checks the existence of
  `tasks.md`; it has no line forecast. Design must decide whether `review_budget_lines` is a
  declarative brief-only threshold or also drives a warning/check in plan/build artifacts.
  Adding an enforcement path without a clear observable contract would overreach this change.
- **External runtime drift (medium):** the external preflight may change its questions or
  defaults independently. This repository can guarantee rendered project rules, not runtime
  consumption; no gentle-ai change should be coupled to this recipe.
- **Tracker brief coexistence (low/medium):** enabled-order and keyed deduplication can alter
  generated rule order. Stable unique keys and additive rules reduce collisions; baseline tests
  must cover the combined enabled-recipe configuration if available.
- **Propagation scope (low):** `ai-specs sync` fans out generated instructions to multiple
  harness formats. The source-of-truth test should target rendered AGENTS.md/brief fragments;
  harness-specific adapters should remain unchanged unless an existing test demonstrates a
  regression.

## Ready for proposal

**Yes.** Proposal/design must settle the exact brief wording and keys, whether the review budget
is advisory or enforced by a plan/build forecast, the recipe version bump policy, and the precise
baseline/catalog documentation updates. The implementation boundary is clear: schema defaults
and overrides, generic existing interpolation, ambient brief rules, docs, and focused tests;
no orchestrator or external-runtime code.
