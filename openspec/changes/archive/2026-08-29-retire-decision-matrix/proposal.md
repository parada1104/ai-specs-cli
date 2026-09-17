# Proposal: retire sdd-adaptive-contract and consolidate into plan-build-flow

## Why

The `sdd-adaptive-contract` ceremony contract is dead configuration that
still claims to be live. Its runtime consumers were removed in the archived
change `2026-05-18-docs-remove-sdd-refocus` and later product cleanups:

- `lib/_internal/sdd.py` (the `decision_matrix` loader/validator) does not
  exist.
- The `openspec-sdd-decision` skill does not exist.
- `recipe_schema.py` / `recipe-read.py` no longer parse or validate a `[sdd]`
  table or `sdd.threshold`; any `[sdd]` in a `recipe.toml` is silently
  ignored.
- `tests/test_sdd.py` and the `[sdd]` manifest section are gone.

Yet the canonical spec `openspec/specs/sdd-adaptive-contract/spec.md`, the
`sdd.decision_matrix` section in `openspec/config.yaml`, the `[sdd]` recipe
metadata section in `docs/recipe-schema.md`, and a ceremony-vocabulary note in
`catalog/recipes/trello-mcp-workflow/README.md` still document and "enforce"
those removed surfaces.

The replacement already exists and is live: `plan-build-flow`
(`openspec/specs/plan-build-flow/spec.md`, recipe
`catalog/recipes/plan-build-flow/`) classifies planning depth as **Light /
Standard / Full**, enforces artifact minima, a PR artifact gate, a staged
verify gate, and a pre-merge merge guardian. The plan-build spec explicitly
mandates that removed `[sdd]` configuration stays removed and that root
discovery never reintroduces a decision matrix. This change executes that
mandate end to end and codifies the vocabulary migration.

## What Changes

1. **Remove the canonical spec** `openspec/specs/sdd-adaptive-contract/`
   (spec.md with 6 requirements / 18 scenarios). Its content is preserved in
   the immutable archive
   `openspec/changes/archive/2026-04-30-definir-sdd-adaptive-contract/`.
2. **Remove the dead config section** `sdd:` (mode + decision_matrix + both
   comment lines) from `openspec/config.yaml`. No replacement section is
   added; plan-build resolves everything from topology and skill, per its own
   spec.
3. **Remove the `[sdd]` recipe metadata section** (heading, `threshold`
   field table, TOML example, "Invalid threshold values are rejected" note)
   from `docs/recipe-schema.md`.
4. **Update tests** `tests/test_manifest_contract_docs.py`: the test
   `test_recipe_reference_covers_current_v2_contract_and_boundaries` must stop
   asserting `## [sdd] recipe metadata` and the `threshold` table row.
5. **Replace the ceremony-vocabulary note** in
   `catalog/recipes/trello-mcp-workflow/README.md`: drop the
   `sdd.decision_matrix` / adaptive-contract mapping, point at
   `plan-build-flow` depth vocabulary instead (or remove the note entirely).
6. **Add a plan-build delta spec** under
   `openspec/changes/retire-decision-matrix/specs/plan-build-flow/spec.md`
   with a new requirement "Legacy ceremony vocabulary retirement" that:
   - declares the four legacy levels and the `openspec-sdd-decision` skill
     retired,
   - codifies the migration mapping `trivial`/`local_fix` → `Light`,
     `behavior_change` → `Standard`, `domain_change` → `Full`,
   - forbids live config, docs, and tests from referencing the retired names
     (excluding archives and changelog history).
7. **Record the change in `CHANGELOG.md`** under Unreleased/Changed with a
   note that the matrix was removed and plan-build is the ceremony source.
8. **Verify** `docs/recipes-catalog.md` carries no reference to the retired
   contract (add a `plan-build-flow` cross-reference only if the depth
   contract is not already summarized there).

## Capabilities

### New Capabilities

- `retire-decision-matrix`: this change itself — a documentation/config/spec
  retirement delta with a codified vocabulary migration. No new runtime
  capability; the plan-build capability absorbs the classifier semantics.

### Modified Capabilities

- `plan-build-flow` (spec delta): gains the explicit legacy-vocabulary
  retirement requirement and migration mapping.
- `recipe-schema` (docs contract): loses the `[sdd]` recipe metadata section;
  the manifest/recipe schema surface is unchanged (the field was never parsed).
- `sdd-adaptive-contract`: **removed** as a live capability (archived only).

## Impact

### Affected Modules

| File | Action |
|---|---|
| `openspec/specs/sdd-adaptive-contract/spec.md` | DELETE (dir) |
| `openspec/config.yaml` | Modify — remove `sdd:` section (lines 74–106) |
| `docs/recipe-schema.md` | Modify — remove `## [sdd] recipe metadata` (lines 479–500) |
| `tests/test_manifest_contract_docs.py` | Modify — drop `[sdd]`/`threshold` assertions (lines 124–125) |
| `catalog/recipes/trello-mcp-workflow/README.md` | Modify — replace `## Ceremony vocabulary note` (lines 123–129) |
| `CHANGELOG.md` | Modify — add retirement entry |
| `docs/recipes-catalog.md` | Verify — no retired-contract reference; optional plan-build depth cross-ref |
| `openspec/changes/retire-decision-matrix/specs/plan-build-flow/spec.md` | ADD — delta spec |

### APIs / Interfaces

- No CLI entrypoint changes; `recipe.toml` `[sdd]` was already unparsed and
  remains unparsed (now also undocumented).
- `openspec/config.yaml` consumers: nothing reads `sdd.decision_matrix` today;
  removing it changes no runtime behavior. The `tracking:` block is untouched.

### Dependencies

- None new. Relies on `plan-build-flow` being the live ceremony contract
  (canonical spec already present at `openspec/specs/plan-build-flow/spec.md`).

### Rollback Plan

- Restore `openspec/specs/sdd-adaptive-contract/` and the `sdd:` config
  section from the archived change folder or the merge commit (both are fully
  preserved). Docs/test updates revert with the same commit. No data loss:
  nothing is deleted that is not recoverable from
  `openspec/changes/archive/2026-04-30-definir-sdd-adaptive-contract/` or git
  history.

## Success Criteria

- `openspec/specs/sdd-adaptive-contract/` no longer exists in the live tree.
- `openspec/config.yaml` has no `sdd:` key; `grep -r sdd` over live
  config/docs (excluding `openspec/changes/`, `CHANGELOG.md`, and archives)
  matches only the replacement plan-build vocabulary.
- `docs/recipe-schema.md` has no `[sdd]`/`threshold` section.
- `tests/test_manifest_contract_docs.py` passes and does not assert retired
  sections.
- The trello README ceremony note references `plan-build-flow` depth
  vocabulary, not `sdd.decision_matrix`.
- The plan-build delta spec documents the migration mapping; every scenario
  is covered by an explicit test or a documented verification step.
- `openspec/changes/archive/` is byte-identical before/after (verified with
  `git status` on that subtree).
- `./tests/run.sh` and `./tests/validate.sh` are green (run during apply;
  not run as part of this artifact-only change).

## Non-Goals

- Reintroducing any `[sdd]` section, decision matrix, `artifact_root`, or
  per-subrepository artifact store (plan-build "Coexistence" mandate).
- Rewriting `plan-build-flow`'s classifier, conflict handling, gates, or
  guardian — only adding the legacy-retirement requirement and mapping.
- Renaming `Light/Standard/Full` or introducing new depth names.
- Touching `openspec/changes/archive/` (immutable audit trail).
- Updating archived change folders, historical specs, or old evals that
  reference the retired vocabulary.
- Implementing a new `decision_matrix`-style config for plan-build.

## Tracker

- **card_id**: `6a78c1e212f9b77a94405085`
- **shortLink**: `oUfVVwde`
- **url**: https://trello.com/c/oUfVVwde/66-retire-decision-matrix-and-consolidate-into-plan-build-flow
***
