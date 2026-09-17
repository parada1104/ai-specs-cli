# Design: retire sdd-adaptive-contract and consolidate into plan-build-flow

> Change slug: `retire-decision-matrix`
> Status: planning (no production edits)
> Branch / worktree: `change/retire-decision-matrix` /
> `.worktrees/retire-decision-matrix/`

## Goals

1. Eliminate the dead `sdd-adaptive-contract` surface (canonical spec, config
   section, doc section, test assertions, README note) so the live tree
   contains exactly one ceremony contract: `plan-build-flow`.
2. Codify the migration mapping from the retired four-level vocabulary to the
   plan-build depth tiers so historical artifacts and agents translating old
   classifications have a documented, testable rule.
3. Preserve the audit trail: nothing under `openspec/changes/archive/`
   changes; the retired spec remains readable in the archived change
   `2026-04-30-definir-sdd-adaptive-contract/`.
4. Keep the change small, mechanical, and fully verifiable: no runtime code
   changes are required because no runtime consumes the retired surfaces.

## Non-Goals

- No changes to plan-build classifier logic, conflict handling, artifact
  minima, PR/verify/archive gates, or the guardian.
- No new config surface for plan-build (no `[sdd]`, no `decision_matrix`, no
  `artifact_root`).
- No archive mutation, no historical reclassification, no rewriting old evals.
- No CLI / manifest-schema behavior changes (the `[sdd]` recipe field was
  never parsed; we only stop documenting it).

## Architecture Decisions

### D1: Full retirement of the live spec, not a stub or redirect

**Decision**: delete `openspec/specs/sdd-adaptive-contract/spec.md` (the
whole directory) from the live tree. Do not replace it with a stub,
deprecation notice, or redirect file.

**Rationale**:
- The spec's own requirements reference runtime surfaces that no longer exist
  (`openspec-sdd-decision` skill, `lib/_internal/sdd.py` validation,
  `recipe-read.py` threshold checks). A live spec that mandates absent
  machinery is worse than no spec: it instructs agents to do the impossible.
- The content is not lost: `openspec/changes/archive/2026-04-30-definir-sdd-adaptive-contract/specs/sdd-adaptive-contract/spec.md` is the immutable record, and git history retains every byte.
- `plan-build-flow` spec.md ("Coexistence with classic SDD") already mandates
  that removed `[sdd]` configuration and artifact-store concepts stay removed.
  Keeping a live spec that documents `decision_matrix` would contradict the
  very replacement spec on this branch.

**Alternatives considered**: stub/redirect (rejected — OpenSpec has no
redirect concept, and a stub would keep the retired vocabulary searchable and
live); keep-spec-but-mark-deprecated (rejected — nothing can consume it, and
plan-build's vocabulary-hygiene rules forbid `sdd` in generated surfaces).

### D2: Remove the `sdd:` config section entirely; add no replacement

**Decision**: `openspec/config.yaml` loses the `sdd:` section (keys `mode`
and `decision_matrix`, plus the two comment lines citing
`sdd-adaptive-contract`). The `tracking:` block and everything else stay
byte-identical.

**Rationale**:
- `sdd.decision_matrix` has zero consumers: `lib/_internal/sdd.py` is absent,
  no validator reads `openspec/config.yaml` for ceremony levels, and the
  plan-build gate derives roots from topology, not config.
- plan-build spec "Central root is not user-configured" explicitly requires
  that gate resolution "MUST NOT require, create, or read a new `[sdd]`
  configuration, decision matrix, or `artifact_root` setting". Removing the
  existing dead matrix aligns the config with that contract.
- Keeping an inert section would fail the acceptance criterion "no live
  reference to the retired contract" (the section's own comment names the
  retired spec).

**Verification**: `grep -n "sdd" openspec/config.yaml` returns nothing after
the edit; `python3 -c "import yaml,sys; yaml.safe_load(open('openspec/config.yaml'))"` still parses.

### D3: `sdd.threshold` is dead documentation; remove the doc section, keep the schema surface untouched

**Decision**: delete `## [sdd] recipe metadata` (with `### threshold` table
and TOML example) from `docs/recipe-schema.md`. Do not add a "removed"
section, and do not touch `lib/_internal/recipe_schema.py`.

**Rationale**:
- Grounded: `grep -n "sdd|Sdd|threshold" lib/_internal/recipe_schema.py lib/_internal/recipe-read.py` returns nothing; `validate_recipe_toml` (line 649+) parses only `[recipe]`, `[provides]`, `[deps.cli]`, `[capabilities]`, `[hooks]`, `[config]`, `[init]`. The field has been silently ignored for months.
- No catalog recipe declares `[sdd]`/`threshold` (`grep` over
  `catalog/**/recipe.toml` finds none), so removing the documentation breaks
  no shipped example.
- Keeping the section would mislead recipe authors into declaring a field
  with no effect — the exact failure mode this change exists to remove.

**Verification**: `grep -n "sdd" docs/recipe-schema.md` returns nothing;
`./tests/validate.sh`-style doc tests updated in tandem (see T4).

### D4: Migration mapping is codified in a plan-build delta spec, not in config

**Decision**: the mapping
`trivial`/`local_fix` → `Light`, `behavior_change` → `Standard`,
`domain_change` → `Full` is normative text + scenarios in the new delta
requirement "Legacy ceremony vocabulary retirement" under
`specs/plan-build-flow/spec.md`. It is not stored in `openspec/config.yaml`
or any new config file.

**Rationale**:
- The mapping is a translation table between the retired vocabulary and the
  live classifier. It belongs next to the classifier (plan-build spec), where
  agents translating historical artifacts will look.
- plan-build forbids a config decision matrix; encoding the mapping as spec
  requirements + scenarios keeps it enforcement-ready (verifiable by the
  vocabulary-hygiene test) without a config surface.
- The mapping is intentionally coarse and lossy: it translates **levels to
  tiers**, not artifact sets. The four legacy levels and the three plan-build
  tiers overlap in spirit (`trivial`≈no artifacts / `Light`≈proposal+tasks;
  `local_fix`≈code+tests / `Light`; `behavior_change`≈specs+tasks /
  `Standard`; `domain_change`≈full chain / `Full`) but plan-build tier minima
  **replace** the legacy artifact lists, not extend them.

**Verification**: the delta spec scenario matrix in *Specification* below is
mapped 1:1 to test assertions (see T6).

### D5: Test update is a paired cutover, not a separate cleanup

**Decision**: `tests/test_manifest_contract_docs.py`
`test_recipe_reference_covers_current_v2_contract_and_boundaries` drops the
two retired assertions (`## [sdd] recipe metadata`, `| threshold | string |
no | Optional ceremony level: ...`) in the **same change** as the doc
removal. No assertion is left asserting the dead section, and no new test
asserts its absence (absence is covered by the vocabulary-hygiene test in
`tests/test_plan_build_flow_recipe.py` and by the doc scan below).

**Rationale**: the test currently locks the dead contract in place; leaving it
would make the change permanently red. The repo convention (config.yaml rules)
requires tests for user-facing doc changes.

### D6: Trello README note points at plan-build, not at the retired matrix

**Decision**: replace the "Ceremony vocabulary note" body in
`catalog/recipes/trello-mcp-workflow/README.md` with a one-paragraph note that
ceremony/depth classification lives in `plan-build-flow` (`Light` / `Standard`
/ `Full`), and that the legacy `trivial/local_fix/behavior_change/
domain_change` vocabulary is retired (see the plan-build spec). No other
change to the trello recipe.

**Rationale**: the note's only purpose was mapping config ceremony levels to
informal aliases; with the config gone, the mapping target is gone. A short
forward-pointer preserves the reader's intent (where does depth vocabulary
live?) without resurrecting dead names as live documentation.

### D7: Scope boundary for changelog and recipes-catalog

**Decision**: `CHANGELOG.md` gains a new Changed entry describing the
retirement; the existing historical entry (line 64) that mentions
`sdd-adaptive-contract` stays (changelogs are append-only records). 
`docs/recipes-catalog.md` is checked for live references (none exist today)
and MAY gain a one-line note under `plan-build-flow` that it is the ceremony
source replacing the retired matrix — added only if the section does not
already describe depth.

**Rationale**: history is immutable; the new entry documents the cutover.
`recipes-catalog.md` must not carry stale vocabulary, so a verification step
(not a speculative rewrite) is the right scope.

## Specification

Delta spec structure (added to `openspec/specs/plan-build-flow/spec.md` as a
new requirement — see `specs/plan-build-flow/spec.md` in this change):

**Requirement: Legacy ceremony vocabulary retirement**

- The legacy four-level ceremony vocabulary (`trivial`, `local_fix`,
  `behavior_change`, `domain_change`) and the `openspec-sdd-decision` skill
  SHALL be retired. Live repositories MUST NOT reference them in
  `openspec/config.yaml`, `docs/`, or tests. References in
  `openspec/changes/archive/`, `CHANGELOG.md`, and git history are exempt.
- The migration mapping SHALL be: `trivial` and `local_fix` → `Light`;
  `behavior_change` → `Standard`; `domain_change` → `Full`. When translating
  an archived or external classification into plan-build depth, the mapping
  SHALL be applied as stated; when no legacy classification exists, the
  plan-build classifier signal rules decide depth and this mapping is not
  consulted.
- No live configuration file SHALL declare a decision matrix or ceremony
  level list. Depth is decided by the plan-build classifier from signal +
  explicit request; `openspec/config.yaml` SHALL NOT contain a `sdd:` section.
- A legacy classification maps to a tier only for translation; plan-build
  artifact minima, gates, and verify evidence for the mapped tier apply
  unchanged and are not reduced by the legacy artifact lists.

Scenarios (Given/When/Then), mapped to acceptance tests in *Testing*:

| # | Scenario | Test / verification |
|---|---|---|
| S1 | GIVEN a repo whose `openspec/config.yaml` contains a `sdd:` section; WHEN validation scans live config; THEN the section is reported as a retired-surface violation | T2, T6 |
| S2 | GIVEN a `recipe.toml` declaring `[sdd] threshold`; WHEN the schema docs are checked; THEN the field is not documented (and is not parsed) | T3, T6 |
| S3 | GIVEN an archived change classified `domain_change`; WHEN an agent translates it to plan-build depth; THEN the decided depth is `Full` and Full minima apply | T6 scenario |
| S4 | GIVEN an archived change classified `behavior_change`; WHEN translated; THEN the depth is `Standard` and Standard minima apply | T6 scenario |
| S5 | GIVEN an archived change classified `trivial` or `local_fix`; WHEN translated; THEN the depth is `Light` and Light minima apply | T6 scenario |
| S6 | GIVEN a change with no legacy classification; WHEN the classifier runs; THEN the mapping is not consulted and signal/explicit-request rules decide | T6 scenario |
| S7 | GIVEN a live tree after this change; WHEN a scan looks for `sdd-adaptive-contract`, `openspec-sdd-decision`, `sdd.decision_matrix`, or `sdd.threshold` in config/docs/tests; THEN only exempt paths (archives, CHANGELOG, git history) match | T6 |

## Data Flow

```mermaid
flowchart LR
    A[openspec/config.yaml<br/>sdd: section] -->|removed| Z[config.yaml<br/>tracking: only]
    B[openspec/specs/<br/>sdd-adaptive-contract] -->|deleted| Z2[live specs: plan-build-flow<br/>is sole ceremony contract]
    C[docs/recipe-schema.md<br/>[sdd] metadata] -->|removed| Z3[recipe docs: no threshold field]
    D[tests/test_manifest_contract_docs.py<br/>[sdd] assertions] -->|updated| Z4[green doc-contract tests]
    E[trello README<br/>ceremony note] -->|replaced| Z5[points at plan-build depth]
    F[archive 2026-04-30-<br/>definir-sdd-adaptive-contract] -.->|immutable| G[audit trail]
    H[plan-build delta spec<br/>retirement + mapping] -->|codifies| I[Light|Standard|Full<br/>classifier + gates]
```

Translation flow for historical artifacts:

```text
legacy classification (archived) ──mapping──▶ plan-build depth tier
  trivial        ──────────────────────────▶ Light
  local_fix      ──────────────────────────▶ Light
  behavior_change ─────────────────────────▶ Standard
  domain_change   ─────────────────────────▶ Full
new work (no legacy classification) ────────▶ classifier signal + explicit request
```

## Testing

Strict TDD applies (`strict_tdd: true` in `openspec/config.yaml`); this
change is docs/config/spec only, so the tests are **doc-contract tests**, not
runtime tests. No new runtime code is introduced.

| ID | Test / verification | Shape | Coverage |
|---|---|---|---|
| T1 | `tests/test_manifest_contract_docs.py::test_recipe_reference_covers_current_v2_contract_and_boundaries` updated: remove `## [sdd] recipe metadata` and the `threshold` row from the `assertContainsAll` list | unit (existing, modified) | S2 / D3 |
| T2 | New `tests/test_manifest_contract_docs.py::test_recipe_doc_has_no_retired_sdd_surface`: `assertNotIn("## `[sdd]` recipe metadata", recipe_doc)` and `assertNotIn("threshold", recipe_doc)` | unit (new) | S1, S2 |
| T3 | New doc-scan test (same module or `tests/test_plan_build_flow_recipe.py`): scan `openspec/config.yaml`, `docs/recipe-schema.md`, `docs/recipes-catalog.md`, `catalog/recipes/trello-mcp-workflow/README.md` and assert the retired tokens (`sdd-adaptive-contract`, `openspec-sdd-decision`, `sdd.decision_matrix`, `sdd.threshold`) are absent | unit (new) | S1–S7 |
| T4 | `tests/test_plan_build_flow_recipe.py` existing vocabulary-hygiene assertions (`FORBIDDEN_TERMS`, `test_recipe_surface_stays_additive_standalone_planning` line 348) remain green — they already forbid `sdd`/`decision matrix` in the generated surface | unit (existing) | S1 |
| T5 | `openspec/config.yaml` parses after removal: `python3 -c "import yaml; yaml.safe_load(open('openspec/config.yaml'))"` and `git diff` shows only the `sdd:` block removed | smoke (apply-time) | D2 |
| T6 | Mapping scenarios S3–S7 verified by a new unit test `tests/test_manifest_contract_docs.py::test_legacy_ceremony_mapping_table` asserting the mapping table text lives in `specs/plan-build-flow/spec.md` delta (presence of the four map rows) | unit (new) | S3–S7 |
| T7 | `./tests/validate.sh` (py_compile + bash -n + full `./tests/run.sh`) green | validation (apply-time) | all |
| T8 | Archive immutability: `git status -- openspec/changes/archive/` clean before and after apply | smoke (apply-time) | acceptance 8 |

## Risks / Trade-offs

| Risk | Likelihood | Mitigation |
|---|---|---|
| A reader searches `sdd-adaptive-contract` and finds nothing live | Medium | The mapping + retirement requirement in the plan-build spec names the retired vocabulary once (normatively), and the trello README note points at plan-build; archives remain searchable |
| A stale agent references `sdd.threshold` in a new `recipe.toml` | Low | Field was never parsed; now it is also undocumented; the recipe-schema docs test locks the absence |
| Test overreach — T3 scans too broadly and flags historical mentions | Medium | Exempt paths are explicit (`openspec/changes/archive/`, `CHANGELOG.md`, git history); T3 scans only the four named live files |
| Reintroduction of a decision matrix by a future change | Low | plan-build spec already forbids it; the new retirement requirement repeats the prohibition and T3 enforces absence in config |
| Verify evidence gate on this very change (Full depth) | — | This change is classified `Full`; `verify-report.md` with Full evidence shape (strict PASS, `ready_for_archive: true`, `## Success-criteria mapping`) is produced during apply per the plan-build staged verify gate |

## Migration / Rollout

1. No runtime migration: nothing consumed the retired surfaces.
2. Existing projects with a `sdd:` section in `openspec/config.yaml` MAY
   remove it; this change removes it for this repo. Other projects are
   unaffected (the section was inert).
3. Existing `recipe.toml` files with `[sdd]` are unaffected (never parsed);
   after this change they are simply undocumented.
4. The delta spec goes live when this change is verified and archived;
   `plan-build-flow` canonical spec is updated via the normal spec sync /
   archive flow.
5. Rollback: revert the merge commit; the deleted spec is recoverable from the
   archived change folder and git history.

## Open Questions

None blocking. (Tracker card TBD — the `## Tracker` section in `tasks.md`
carries the placeholder per the `tracking:` contract, `gate_mode: warn`.)
