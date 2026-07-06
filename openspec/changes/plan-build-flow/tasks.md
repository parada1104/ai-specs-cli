# Tasks: plan-build-flow

Source spec: `openspec/changes/plan-build-flow/specs/plan-build-flow/spec.md`
Source design: `openspec/changes/plan-build-flow/design.md`

Execution mode: **strict TDD**. Phase 1 tasks MUST land as failing tests before
any Phase 2 recipe content exists (RED). Phase 2 tasks make them pass with the
minimum recipe content (GREEN). No task may skip the RED step.

Legend: `[P]` = can run in parallel with sibling `[P]` tasks in the same phase.
Unmarked tasks are sequential (depend on the prior task in the phase).

---

## Phase 1 — Tests scaffolding (RED)

All tasks in this phase write to a single new file. They are sequential within
that file (same file, ordered edits) but the phase as a whole must complete and
show RED (failing/erroring collection) before Phase 2 starts.

- [x] **T1.1** — Create `tests/test_plan_build_flow_recipe.py` scaffold:
  imports (`importlib.util`, `re`, `sys`, `tempfile`, `unittest`, `Path`),
  `ROOT`, `RECIPE_MATERIALIZE_PATH`, `RECIPE_SCHEMA_PATH`, `CATALOG`,
  `RECIPE_ID = "plan-build-flow"`, `load_module()`, `_recipe_version()` (regex
  read of `version` from `recipe.toml`, never hardcoded), following
  `tests/test_tdd_flow_recipe.py` conventions.
  **Requirement:** manifest/naming (spec §Recipe manifest and command naming).
  **Done when:** file exists, imports resolve, `python3 -m pytest
  tests/test_plan_build_flow_recipe.py --collect-only` fails only because the
  recipe directory does not exist yet (RED for the right reason, not a syntax
  error).

- [x] **T1.2** — Add `test_recipe_materializes_two_commands` (AC1): load
  `catalog/recipes/plan-build-flow/recipe.toml` via `recipe_schema.load_recipe_toml`;
  assert `id == "plan-build-flow"`, exactly two commands declared with ids
  `plan` and `build`, one bundled skill `plan-build-flow`. Then build a tmp
  project fixture (mirroring `_make_project` in `test_tdd_flow_recipe.py`),
  run `materialize_recipes(root, ROOT)`, assert it returns `0`, and assert:
  `ai-specs/commands/plan.md` exists, `ai-specs/commands/build.md` exists, AND
  **`ai-specs/commands/archive.md` does NOT exist** (negative assertion per
  design-gate adjustment: no third command file is generated).
  **Requirement:** Recipe manifest and command naming — Scenario "Materialization
  produces exactly two commands".
  **Done when:** test is written and fails only on missing recipe content (RED).

- [x] **T1.3** — Add `test_recipe_adds_no_schema_surface` (AC2): parse the
  loaded recipe and assert `config_schema.fields` is empty (no `[config.*]`
  fields declared), assert the only hook is `on-sync = ["validate-config"]`
  (no other hook events/actions), and assert command/skill ids use none of the
  literal strings `sdd`, `openspec`, `spec-driven` (case-insensitive) anywhere
  in `recipe.toml` identifiers (id, name fields for commands/skills/capability).
  **Requirement:** Recipe manifest and command naming — Scenario "No new schema
  or materializer surface".
  **Done when:** test is written and fails only on missing recipe content (RED).

- [x] **T1.4** — Add `test_brief_and_readme_vocabulary_clean` (AC8): read
  `[provides.brief]` fragments from the parsed recipe (or raw TOML) and the
  materialized `ai-specs/recipes/plan-build-flow/README.md` from a tmp-project
  materialization; assert none of `SDD`, `OpenSpec`, `spec-driven` (case-
  insensitive) appear in either.
  **Requirement:** Vocabulary hygiene in generated output.
  **Done when:** test is written and fails only on missing recipe content (RED).

- [x] **T1.5** — Add `test_build_brief_references_worktree_flow` (AC9): parse
  `[provides.brief].workflow_rules` from `recipe.toml` (or the materialized
  brief fragment) and assert at least one entry mentions `worktree` in the
  context of `/build` (substring match, e.g. `"worktree"` present alongside
  `"build"` in the same rule string), confirming the cross-reference is a
  `workflow_rules` note and not a hard dependency (assert no `requires`/
  `conflicts_with` entry names `worktree-flow`).
  **Requirement:** Worktree-flow cross-reference.
  **Done when:** test is written and fails only on missing recipe content (RED).

- [x] **T1.6** — Add `test_classic_sdd_commands_unchanged` (AC10): in the tmp
  project fixture, first materialize `tdd-flow` and `worktree-flow` alone
  (both already enabled in the manifest) and snapshot the byte content of
  every file under `ai-specs/commands/`, `ai-specs/.recipe/tdd-flow/`, and
  `ai-specs/.recipe/worktree-flow/`. Then enable `plan-build-flow` in the same
  manifest, re-run `materialize_recipes`, and assert every previously
  snapshotted file still exists with **identical byte content**, and that no
  pre-existing command file was renamed or removed.
  **Requirement:** Coexistence with classic SDD.
  **Done when:** test is written and fails only because `plan-build-flow` does
  not exist yet to enable (RED).

- [x] **T1.7** — Run `./tests/run.sh` (or targeted
  `python3 -m pytest tests/test_plan_build_flow_recipe.py -v`) and capture the
  RED output (all six new tests failing for the expected reason: missing
  recipe directory/files, not import or syntax errors).
  **Done when:** RED evidence is captured verbatim for the apply/verify record.

---

## Phase 2 — Recipe content (GREEN)

Tasks T2.1–T2.5 write independent new files under
`catalog/recipes/plan-build-flow/` and can run **in parallel** `[P]`; T2.6
(catalog entry doc) and T2.7 (re-run tests) are sequential and depend on all
of T2.1–T2.5.

- [x] **T2.1 [P]** — Create `catalog/recipes/plan-build-flow/recipe.toml`
  exactly per design §4: `[recipe]` block (`id = "plan-build-flow"`,
  `version = "1.0.0"`, `tags = ["workflow"]`, no `conflicts_with`),
  `[[capabilities]] id = "plan-build-flow"`, `[[hooks]] event = "on-sync"
  action = "validate-config"`, `[provides]` with bundled skill
  `plan-build-flow` and commands `plan` → `commands/plan.md`, `build` →
  `commands/build.md`, `[provides.brief]` with `workflow_rules` (three rules
  per design, including the worktree cross-reference rule) and
  `useful_commands` (`/plan`, `/build`), `[[provides.docs]]` mapping
  `README.md` → `ai-specs/recipes/plan-build-flow/README.md`. No
  `[config.*]` fields.
  **Requirement:** Recipe manifest and command naming; Worktree-flow
  cross-reference.
  **Done when:** `T1.2`, `T1.3`, and `T1.5` pass against this file alone
  (schema-level assertions).

- [x] **T2.2 [P]** — Create
  `catalog/recipes/plan-build-flow/skills/plan-build-flow/SKILL.md` per
  design §5: front-matter (`name: plan-build-flow`, description, `license:
  MIT`, `metadata.scope: runtime`, `auto_invoke` list), then body sections in
  order: (1) what plan/build mean, (2) phase mapping (`/plan` →
  explore→proposal→spec→design→tasks; `/build` → apply→verify→archive-tail,
  explicitly noting phase-name vocabulary stays out of brief/README), (3)
  orchestrator degradation policy (gentle-ai present vs. absent — inline
  single-conversation run), (4) memory degradation policy (Engram present vs.
  absent → OpenSpec files, or explicit `none`), (5) artifact-store default
  policy (preflight wins; otherwise default OpenSpec files), (6) change-slug
  derivation and persistence rules, (7) worktree deference (`/plan` no
  worktree required; `/build` defers to `worktree-flow` when enabled), (8)
  archive-tail graceful no-op rules (vault/tracker channels no-op with a note
  when their recipes are absent; change-folder close always completes).
  **Requirement:** `/plan` phase mapping; `/build` phase mapping and automatic
  close; Archive channel degradation; Orchestrator-absence degradation;
  Artifact store degradation and default.
  **Done when:** file exists and materializes to
  `ai-specs/.recipe/plan-build-flow/skills/plan-build-flow/SKILL.md`.

- [x] **T2.3 [P]** — Create `catalog/recipes/plan-build-flow/commands/plan.md`
  per design §6: title `# /plan — Turn an intent into a reviewable plan`,
  defers to the `plan-build-flow` skill, steps covering slug derivation,
  artifact-store resolution, running explore→proposal→spec→design→tasks
  (orchestrated or inline), and stopping for human review/authorization
  without implementing.
  **Requirement:** `/plan` phase mapping.
  **Done when:** file exists and materializes to `ai-specs/commands/plan.md`.

- [x] **T2.4 [P]** — Create `catalog/recipes/plan-build-flow/commands/build.md`
  per design §6: title `# /build — Implement, validate, and close an
  authorized plan`, defers to the `plan-build-flow` skill, steps covering
  slug/store resolution, authorization check (stop and point back to `/plan`
  if unauthorized), worktree-flow deference when enabled, running
  apply→verify, running the archive tail with graceful no-op notes, and
  reporting completion.
  **Requirement:** `/build` phase mapping and automatic close; Archive channel
  degradation; Worktree-flow cross-reference.
  **Done when:** file exists and materializes to `ai-specs/commands/build.md`.

- [x] **T2.5 [P]** — Create `catalog/recipes/plan-build-flow/README.md`
  (materialized doc), following the `worktree-flow`/`tdd-flow` README shape:
  what it provides (skill + two commands), how `/plan` and `/build` map to
  ceremony without naming SDD/OpenSpec/spec-driven, enable snippet
  (`[recipes.plan-build-flow] enabled = true / version = "1.0.0"`), and a note
  on worktree-flow coexistence. Must contain none of `SDD`, `OpenSpec`,
  `spec-driven` (case-insensitive).
  **Requirement:** Vocabulary hygiene in generated output.
  **Done when:** `T1.4` passes against this file's materialized content.

- [x] **T2.6** — Update `docs/recipes-catalog.md`: add a new row to the "At a
  glance" table (`plan-build-flow`, tier `Foundational`, focus summary,
  capability `plan-build-flow`, no MCP, no key config) and a new `##
  plan-build-flow` section (mirroring the `tdd-flow`/`worktree-flow` section
  shape: description paragraph, Provides bullet, Config = none, Full README
  link, enable snippet). Must not modify any existing recipe's row or section.
  **Requirement:** Coexistence with classic SDD (documentation must not touch
  existing entries).
  **Done when:** the new section renders correctly and a diff shows only
  additions, no edits to pre-existing rows/sections.

- [x] **T2.7** — Run `./tests/run.sh` (or targeted
  `python3 -m pytest tests/test_plan_build_flow_recipe.py -v`) and confirm
  all six tests from Phase 1 (T1.2–T1.6) now pass (GREEN), with no regression
  in `test_tdd_flow_recipe.py` or `test_worktree_flow_recipe.py`.
  **Done when:** GREEN evidence is captured verbatim; zero failures, zero
  errors across the three recipe test files.

---

## Phase 3 — Docs and catalog polish

- [ ] **T3.1** — Cross-check `tags = ["workflow"]` is still unused by any
  other catalog recipe (re-verify the design's assumption at apply time):
  `grep -r 'tags = \[' catalog/recipes/*/recipe.toml` and confirm no sibling
  recipe already declares `workflow`.
  **Requirement:** design assumption validation (§10, D10).
  **Done when:** confirmed no collision, or a deviation is recorded if one is
  found.

- [ ] **T3.2** — Spot-check the generated `AGENTS.md` brief fragment in the
  Phase 1/2 tmp-project fixture (or a scratch project) for vocabulary leakage
  beyond the automated string check: read the merged `workflow_rules` /
  `useful_commands` prose by eye and confirm it reads naturally as plan/build
  only.
  **Requirement:** Vocabulary hygiene in generated output (human-review
  complement to T1.4/T2.7's automated check).
  **Done when:** a reviewer note is recorded confirming no ceremony vocabulary
  leaked through composition with other recipes' brief fragments.

---

## Phase 4 — Verification evidence

- [ ] **T4.1** — Run full validation: `./tests/validate.sh`. Capture pass/fail
  output for the apply/verify record.
  **Done when:** validate.sh exits 0 with the new recipe present.

- [ ] **T4.2** — Record final RED→GREEN evidence in the apply-progress
  artifact: the T1.7 RED capture, the T2.7 GREEN capture, and the T4.1
  full-suite result, per the TDD Evidence Policy.
  **Done when:** evidence is present in `sdd/plan-build-flow/apply-progress`
  (or the equivalent apply record) before verify/archive runs.

- [ ] **T4.3** — **Verification-scope note (read before running sdd-verify):**
  Acceptance criteria **AC3–AC7** (`/plan` stopping before implementation,
  `/build` running apply→verify→close in one invocation, archive no-op
  degradation, inline execution without an orchestrator, and default-store
  resolution without a preflight) are **runtime agent-behavior scenarios**,
  not materialization outputs. `ai-specs-cli`'s test suite can only assert
  that files land correctly (design §1, "Boundary of correctness" / §8, "Test
  Strategy — Scope: materialization only"). These five criteria are
  out of automated-test scope **by design**, not by omission, and are instead
  verified by **human/content review** of the generated `SKILL.md` body
  (T2.2) and the two command prompts (T2.3, T2.4) during apply/verify: a
  reviewer reads the phase-mapping, degradation-policy, and archive-tail
  sections and confirms the prose unambiguously instructs an agent to perform
  each scenario correctly. `sdd-verify` MUST NOT flag the absence of automated
  tests for AC3–AC7 as a regression; it should instead confirm this content
  review was performed and record its outcome.
  **Done when:** this note is acknowledged and the content review for
  AC3–AC7 is recorded (reviewer name/date + confirmation) alongside the
  apply-progress evidence.

---

## Requirement-to-task traceability

| Requirement | AC | Covered by |
|---|---|---|
| Recipe manifest and command naming | AC1, AC2 | T1.2, T1.3, T2.1, T2.6 |
| `/plan` phase mapping | AC3 | T2.2, T2.3 — verified via T4.3 human review (out of automated scope) |
| `/build` phase mapping and automatic close | AC4 | T2.2, T2.4 — verified via T4.3 human review (out of automated scope) |
| Archive channel degradation | AC5 | T2.2, T2.4 — verified via T4.3 human review (out of automated scope) |
| Orchestrator-absence degradation | AC6 | T2.2 — verified via T4.3 human review (out of automated scope) |
| Artifact store degradation and default | AC7 | T2.2 — verified via T4.3 human review (out of automated scope) |
| Vocabulary hygiene in generated output | AC8 | T1.4, T2.1, T2.5, T2.7, T3.2 |
| Worktree-flow cross-reference | AC9 | T1.5, T2.1, T2.4 |
| Coexistence with classic SDD | AC10 | T1.6, T2.6, T2.7 |

**Naming reconciliation note:** test function names in this task list adopt
the spec's AC table names verbatim (`test_recipe_materializes_two_commands`,
`test_recipe_adds_no_schema_surface`, `test_brief_and_readme_vocabulary_clean`,
`test_build_brief_references_worktree_flow`,
`test_classic_sdd_commands_unchanged`) rather than the design document's
draft names (`test_recipe_validates_and_declares_capability`,
`test_materialize_produces_skill_commands_and_doc`,
`test_brief_has_no_ceremony_vocabulary`, `test_no_config_and_no_runtime_hooks`).
The design's four planned assertions are folded into the spec-named tests
(T1.2 absorbs schema validation + materialization + the new negative
assertion; T1.3 absorbs the no-config/no-runtime-hooks check). No coverage is
lost; only naming is aligned to the spec as the source of truth.

---

## Review Workload Forecast

- **Estimated changed lines:** ~600–650 (one new test file ~180–200 lines;
  five new recipe files ~320–350 lines; one docs edit ~30 lines — all
  additive/new files except the `docs/recipes-catalog.md` insertion).
- **Chained PRs recommended:** No — the change is a single cohesive,
  additive-only catalog recipe with no edits to existing production code
  paths, no hot-path files (`auth`/`update`/`security`/`payments`), and low
  interdependency risk between its files.
- **400-line budget risk:** Medium — raw estimated line count exceeds the
  400-line budget, but nearly all of it is net-new file content (test file +
  recipe bundle) rather than modification of existing logic.
- **Decision needed before apply:** Yes — per `delivery_strategy: ask-on-risk`,
  flag this to the orchestrator/user before `sdd-apply`: confirm whether to
  proceed as a single PR with `size:exception` (recommended, given the
  additive-only nature) or split further (e.g., tests-only PR followed by a
  recipe-content PR).
