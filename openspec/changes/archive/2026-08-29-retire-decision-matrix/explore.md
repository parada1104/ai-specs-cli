# Exploration: retire sdd-adaptive-contract and consolidate into plan-build-flow

> Change slug: `retire-decision-matrix`
> Tracker: **TBD** (no Trello card found for this change — see `tasks.md` tracker section)
> Depth signal: **full** — cross-cutting removal of a canonical spec, a live
> config section, and a documented recipe field, replaced by the
> `plan-build-flow` contract; requires proposal, design, delta specs, and tasks.
> Branch / worktree: `change/retire-decision-matrix` /
> `.worktrees/retire-decision-matrix/`

## Problem

The adaptive SDD ceremony contract (`openspec/specs/sdd-adaptive-contract/`)
is **dead configuration that still claims to be live**. It defines four
ceremony levels (`trivial` / `local_fix` / `behavior_change` /
`domain_change`), a `decision_matrix` in `openspec/config.yaml`, a
`sdd.threshold` recipe field, and a `openspec-sdd-decision` skill. None of the
runtime machinery that was supposed to consume it exists anymore:

- `lib/_internal/sdd.py` (the CLI reader/validator of `decision_matrix`) does
  not exist.
- The `openspec-sdd-decision` skill does not exist.
- `recipe_schema.py` / `recipe-read.py` no longer parse or validate a `[sdd]`
  table or `sdd.threshold` (the field is silently ignored today).
- The `[sdd]` section of `ai-specs/ai-specs.toml` and all SDD product code
  were removed in the archived change `2026-05-18-docs-remove-sdd-refocus`.

Meanwhile `plan-build-flow` (canonical spec `openspec/specs/plan-build-flow/
spec.md`) has become the **live replacement** for ceremony classification: it
defines the `Light` / `Standard` / `Full` depth classifier, artifact minima,
adversarial depth-conflict handling, PR artifact gate, staged verify gate, and
pre-merge merge guardian. The plan-build spec already mandates that removed
`[sdd]` configuration must **stay removed** and that root discovery must not
reintroduce a `decision_matrix` (spec.md "Coexistence with classic SDD").

## Acceptance outcomes

1. The canonical spec `openspec/specs/sdd-adaptive-contract/` is removed from
   the live tree.
2. `openspec/config.yaml` no longer contains the `sdd:` section (or it is
   replaced by nothing — no dead configuration remains).
3. The `[sdd]` recipe metadata section in `docs/recipe-schema.md` is removed;
   `sdd.threshold` is no longer documented as a supported recipe field.
4. `tests/test_manifest_contract_docs.py` is updated so it no longer asserts
   the `[sdd]` recipe metadata section or the `threshold` table row.
5. The "Ceremony vocabulary note" in
   `catalog/recipes/trello-mcp-workflow/README.md` no longer points at
   `sdd.decision_matrix` / the adaptive-contract levels.
6. `CHANGELOG.md` and `docs/recipes-catalog.md` carry no live reference to the
   retired spec or the removed config surface.
7. The migration mapping is documented and enforced by delta spec: old
   `trivial`/`local_fix` → `Light`, `behavior_change` → `Standard`,
   `domain_change` → `Full`.
8. Archives are preserved: nothing under `openspec/changes/archive/` is
   modified, and the retired spec stays readable in the archived change
   `2026-04-30-definir-sdd-adaptive-contract/`.

## Current state (grounded in this worktree @ 7af4b22)

### Live references to the retired contract (inventory)

| # | Location | Kind | Evidence (worktree) |
|---|---|---|---|
| 1 | `openspec/specs/sdd-adaptive-contract/spec.md` | Canonical spec (REMOVE) | 6 requirements, 18 scenarios (Ceremony Levels, Artifacts per Level, Declarative Configuration, Recipe Threshold, Configuration Validation, Decision Skill) |
| 2 | `openspec/config.yaml` lines 74–106 | Live config section (REMOVE) | `sdd: {mode: adaptive, decision_matrix: {trivial, local_fix, behavior_change, domain_change}}`; comment cites `openspec/specs/sdd-adaptive-contract/spec.md` |
| 3 | `docs/recipe-schema.md` lines 479–500 | Doc section (REMOVE) | `## [sdd] recipe metadata` + `### threshold` + example `[sdd] threshold = "behavior_change"` |
| 4 | `tests/test_manifest_contract_docs.py` | Test assertions (UPDATE) | `test_recipe_reference_covers_current_v2_contract_and_boundaries` asserts `## [sdd] recipe metadata` and the `threshold` table row (lines 124–125) |
| 5 | `catalog/recipes/trello-mcp-workflow/README.md` lines 123–129 | Doc note (UPDATE) | `## Ceremony vocabulary note` mapping `sdd.decision_matrix` levels to `light/standard/full/tasks-only` |
| 6 | `CHANGELOG.md` line 64 | Changelog entry (UPDATE) | "aligned `sdd.decision_matrix` with `sdd-adaptive-contract`; added declarative `tracking:` section" (historical; add a new entry) |
| 7 | `docs/recipes-catalog.md` | Doc (VERIFY / UPDATE) | No `sdd`/ceremony refs found today, but the plan-build section (line 198+) should cross-reference the depth contract, not the retired matrix |

### Already absent (verified — do NOT recreate)

| Surface | Status |
|---|---|
| `lib/_internal/sdd.py` | absent (`ls` fails) — `decision_matrix` loader/validator gone |
| `ai-specs/skills/openspec-sdd-decision/` | absent — decision skill gone |
| `tests/test_sdd.py` | absent — matrix validation tests gone |
| `sdd.threshold` in `lib/_internal/recipe_schema.py` / `recipe-read.py` | absent — `grep` for `sdd|Sdd|threshold` finds nothing; `[sdd]` tables in any `recipe.toml` are not parsed (`load_recipe_toml` reads `[recipe]`, `[provides]`, `[deps.cli]`, `[capabilities]`, `[hooks]`, `[config]`, `[init]` only) |
| `[sdd]` in `ai-specs/ai-specs.toml` / `templates/ai-specs.toml.tmpl` | absent (only an unrelated prose mention of "gentle-ai stack (SDD, memory)" in `ai-specs.toml` line 27) |
| `sdd.threshold` in any catalog `recipe.toml` | absent (grep over catalog finds no `[sdd]`/`threshold` in `recipe.toml` files) |
| `openspec-sdd-workflow` / `openspec-phase-orchestrator` skills | absent (archived `2026-05-18-docs-remove-sdd-refocus` removed the product SDD skills) |

### Historical references (archives — MUST NOT be touched)

- `openspec/changes/archive/2026-04-30-definir-sdd-adaptive-contract/` —
  full archived change that created the spec; contains `specs/sdd-adaptive-contract/spec.md`, `design.md`, `tasks.md`, `verify-report.md`.
- `openspec/changes/archive/2026-05-18-docs-remove-sdd-refocus/` —
  archived docs change that removed SDD product code/skills and documented the remaining refs as follow-up candidates (its Warnings section explicitly lists `docs/recipe-schema.md` N3-protected `[sdd]` section as a candidate).
- `openspec/changes/archive/2026-07-31-worktree-flow-repo-topology/explore.md`
  and `2026-07-31-worktree-gate-bash-coverage/explore.md` — cite
  `sdd-adaptive-contract`/`decision_matrix` for classification rationale
  (historical, not live).
- `openspec/changes/archive/2026-08-02-tracker-card-gate/` — aligned
  `decision_matrix` with the spec; the alignment work itself is now obsolete.
- Other spec deltas in `openspec/changes/archive/2026-04-26-*` reference the
  old `[sdd]` manifest / `sdd-cli-integration` / `sdd-artifact-store` contracts
  (already superseded by the refocus removal; immutable).

### Replacement contract: plan-build-flow (live)

`openspec/specs/plan-build-flow/spec.md` (949 lines, requirements + 14 ACs) is
the replacement. Relevant normative anchors for this change:

- **Change depth classifier** — exactly one of `Light` (proposal → tasks),
  `Standard` (conditional explore → proposal → spec → tasks), `Full`
  (explore → proposal → spec → design → tasks); explicit depth request vs
  signal detection.
- **Adversarial depth conflict detection** / **Conflict ask** /
  **Depth resolution annotation** — `Requested depth:` / `Signal depth:` /
  `Decided depth:` / `Decision source:` lines.
- **Depth artifact minima** — Light `proposal.md` + `tasks.md`; Standard +
  `specs/**/*.md` (+ `explore.md` on criteria); Full `tasks.md` +
  (`proposal.md` or `design.md`) + `specs/**/*.md`.
- **Staged verify gate** — Light advisory; Standard/Full require
  `verify-report.md` with auditable shape.
- **Pre-merge merge guardian** — enforces tier minima + verify evidence on the
  archived folder, per slug, CLI-home path.
- **Pre-tool-use artifact gate hook** (`hooks/plan-build-gate.sh`).
- **Vocabulary hygiene** — generated brief/README must not contain `sdd`,
  `openspec`, `spec-driven`, `/plan`, `/build`.
- **Coexistence with classic SDD** — MUST NOT reintroduce `[sdd]` config, a
  planning decision matrix, `artifact_root`, or per-subrepository stores.
- **Centralized artifact convention / topology-aware root** — the gate derives
  the planning-artifact root from repo topology (submodule/superproject), with
  zero new config.

The plan-build **recipe itself** (`catalog/recipes/plan-build-flow/`) and its
tests (`tests/test_plan_build_flow_recipe.py`) already assert that `[sdd]`,
`decision matrix`, and `artifact_root` never appear in the generated surface
(`FORBIDDEN_TERMS = ("sdd", "openspec", "spec-driven")` at line 22;
`test_recipe_surface_stays_additive_standalone_planning` line 348).

## Gaps relative to acceptance

1. **Canonical spec still live** — `openspec/specs/sdd-adaptive-contract/`
   is the only live spec referencing `decision_matrix` and
   `openspec-sdd-decision`; deleting it leaves no live owner of the old levels.
2. **Config still declares dead ceremony** — `openspec/config.yaml` `sdd:`
   section documents levels nobody reads; it also contradicts plan-build's
   "no decision matrix" mandate on this very branch.
3. **Docs still teach the retired field** — `docs/recipe-schema.md` `[sdd]`
   metadata section; the trello README ceremony note; both are live,
   user-facing surfaces.
4. **Test asserts the dead doc** — `test_manifest_contract_docs.py`
   `test_recipe_reference_covers_current_v2_contract_and_boundaries` will fail
   if the docs section is removed without a matching test update.
5. **No migration mapping** — nothing in plan-build states the
   trivial/local_fix/behavior_change/domain_change → Light/Standard/Full map,
   so historical artifacts (and agents who learned the old vocabulary) have no
   documented translation.

## Approaches (options — not locked)

| # | Approach | Pros | Cons / risks |
|---|---|---|---|
| A | **Full retirement (recommended)** — delete live spec + `[sdd]` config section + doc section; update test and trello README note; add plan-build delta codifying the migration map | Removes dead surface completely; matches plan-build "stay removed" mandate; one coherent cutover | Touches canonical specs + config + docs + tests in one change (a `Full` change) |
| B | Keep `[sdd]` config as inert documentation | Zero test churn | Leaves live dead config that contradicts the plan-build contract; violates the spec's own coexistence requirement |
| C | Migrate the decision_matrix into plan-build config | Reuses level names | Directly prohibited by plan-build spec ("MUST NOT reintroduce a planning decision matrix"); reintroduces a `[sdd]`-shaped config surface |
| D | Docs-only cleanup, keep the canonical spec | Smallest diff | Leaves a live spec that defines removed runtime surfaces (`openspec-sdd-decision` skill) and a config section nothing consumes |

## Decision lean, first pass

Preferred **A (full retirement + plan-build consolidation)**:

- Delete `openspec/specs/sdd-adaptive-contract/`.
- Remove the `sdd:` section (with its two comment lines) from
  `openspec/config.yaml`.
- Remove the `[sdd]` recipe metadata section from `docs/recipe-schema.md`.
- Update `tests/test_manifest_contract_docs.py` assertions.
- Replace the trello README "Ceremony vocabulary note" with a pointer to
  plan-build depth vocabulary.
- Add a plan-build **delta spec** (`specs/plan-build-flow/spec.md`) with a new
  requirement codifying: (a) the legacy ceremony vocabulary is retired; (b)
  the mapping `trivial`/`local_fix` → `Light`, `behavior_change` →
  `Standard`, `domain_change` → `Full`; (c) no live config/docs/tests may
  reference the retired names.
- Keep the `## Tracker` section with **TBD** until a card exists.

## Open questions — resolved

1. **Does `plan-build-flow` spec.md already forbid the matrix?** Yes —
   "Coexistence with classic SDD" scenario "Classic flow and removed
   configuration remain unaffected" and "Central root is not user-configured"
   scenario both mandate no `[sdd]` / decision-matrix reintroduction. This
   change executes that mandate.
2. **Is `sdd.threshold` still parsed anywhere?** No — `recipe_schema.py`
   `validate_recipe_toml` reads only `[recipe]`, `[provides]`, `[deps.cli]`,
   `[capabilities]`, `[hooks]`, `[config]`, `[init]`; no `[sdd]` table is
   consumed, and `grep sdd|threshold` in `lib/` returns nothing. The doc
   section is pure dead documentation.
3. **Are there catalog recipes declaring `sdd.threshold`?** No —
   `grep threshold|\[sdd\]` over `catalog/**/recipe.toml` finds none.
4. **Which test asserts the retired doc?** `tests/test_manifest_contract_docs.py`
   `test_recipe_reference_covers_current_v2_contract_and_boundaries`
   (asserts `## [sdd] recipe metadata` and the `threshold` row).
5. **Where does the retired spec survive?** Only in
   `openspec/changes/archive/2026-04-30-definir-sdd-adaptive-contract/`
   (spec delta + design + tasks + verify) — the immutable audit trail. No
   archive mutation is needed or allowed.
6. **Does anything live still name `openspec-sdd-decision`?** Only the
   retired spec itself (lines 131–148) and the archived change. No live skill.
7. **Will deleting the spec break plan-build or other live specs?** No live
   spec cross-references `sdd-adaptive-contract` (grep over
   `openspec/specs/` finds it only in itself). `test_plan_build_flow_recipe.py`
   already treats `sdd` as forbidden surface vocabulary.
