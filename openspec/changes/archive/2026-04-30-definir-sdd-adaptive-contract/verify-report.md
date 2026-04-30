# Verification Report

Change: definir-sdd-adaptive-contract
Date: 2026-04-30

## Test Results

| Test Suite | Status | Count |
|---|---|---|
| tests.test_sdd | PASS | 14 tests |
| tests.test_recipe_schema | PASS | 5 tests |
| Full suite (./tests/run.sh) | PASS | 196 tests |
| Validation (./tests/validate.sh) | PASS | - |

## Spec Compliance

| Requirement | Scenario | Status | Evidence |
|---|---|---|---|
| Ceremony Levels | Typo correction classified as trivial | COMPLIANT | `ai-specs/skills/openspec-sdd-decision/SKILL.md` defines `trivial` criteria |
| Ceremony Levels | Localized null-pointer fix classified as local_fix | COMPLIANT | `ai-specs/skills/openspec-sdd-decision/SKILL.md` defines `local_fix` criteria |
| Ceremony Levels | Validation rule change classified as behavior_change | COMPLIANT | `ai-specs/skills/openspec-sdd-decision/SKILL.md` defines `behavior_change` criteria |
| Ceremony Levels | New capability classified as domain_change | COMPLIANT | `ai-specs/skills/openspec-sdd-decision/SKILL.md` defines `domain_change` criteria |
| Artifacts per Level | Trivial change needs no artifacts | COMPLIANT | `openspec/config.yaml` `trivial.artifacts: []`; skill confirms no artifacts required |
| Artifacts per Level | Local fix requires tests but not specs | COMPLIANT | `openspec/config.yaml` `local_fix.artifacts: []`; skill confirms tests only |
| Artifacts per Level | Behavior change requires updated specs and tests | COMPLIANT | `openspec/config.yaml` `behavior_change.artifacts: ["tasks.md"]`; skill mandates specs+tests |
| Artifacts per Level | Domain change requires full SDD cycle | COMPLIANT | `openspec/config.yaml` `domain_change.artifacts: ["proposal.md", "design.md", "tasks.md"]` |
| Declarative Configuration | decision_matrix defines all four levels | COMPLIANT | `openspec/config.yaml` contains all 4 levels under `sdd.decision_matrix` |
| Declarative Configuration | decision_matrix level maps to artifact list and flags | COMPLIANT | Each level declares `artifacts`, `worktree_required`, `proposal_required`, `design_required` |
| Recipe Threshold | recipe.toml declares threshold | COMPLIANT | `lib/_internal/recipe_schema.py` `_parse_sdd` parses `threshold`; `lib/_internal/recipe-read.py` reads it |
| Recipe Threshold | invalid threshold value rejected | COMPLIANT | `tests/test_recipe_schema.py:test_recipe_with_invalid_threshold_fails` and `test_recipe_read_defensively_rejects_invalid_threshold` |
| Configuration Validation | Missing level in decision_matrix fails validation | COMPLIANT | `tests/test_sdd.py:test_validate_decision_matrix_missing_level` |
| Configuration Validation | Unknown threshold in recipe fails validation | COMPLIANT | `tests/test_recipe_schema.py:test_recipe_with_invalid_threshold_fails` |
| Configuration Validation | Low-impact change forced to high formality emits warning | COMPLIANT | `ai-specs/skills/openspec-sdd-decision/SKILL.md` step 6 mandates warning; `ai-specs/skills/openspec-sdd-workflow/SKILL.md` Guardrails reinforce it |
| Decision Skill | Skill inspects specs before classification | COMPLIANT | `ai-specs/skills/openspec-sdd-decision/SKILL.md` step 2 requires inspecting `openspec/specs/` |
| Decision Skill | Skill reports classification and rationale | COMPLIANT | `ai-specs/skills/openspec-sdd-decision/SKILL.md` step 5 requires `classification` and `reasoning` |
| Decision Skill | Skill reports touched artifacts | COMPLIANT | `ai-specs/skills/openspec-sdd-decision/SKILL.md` step 5 requires `specs_touched`, `code_touched`, `tests_touched` |

## Coverage

- Unit tests: 15 tests covering `load_decision_matrix`, `validate_decision_matrix`, `_parse_sdd`, `recipe_to_dict`, and schema validation
- Integration tests: 4 tests covering CLI help, refresh-bundled preset, and recipe-read defensive validation
- Full suite: 196 tests pass with no regressions
- Unavailable signals: coverage, linter, type-checker, formatter (not configured per `openspec/config.yaml`)

## Issues Found

None.

## Verdict

PASS
