# Spec: plan-build-flow

## ADDED Requirements

### Requirement: Legacy ceremony vocabulary retirement

The legacy four-level ceremony vocabulary — `trivial`, `local_fix`,
`behavior_change`, `domain_change` — and the `openspec-sdd-decision` skill
SHALL be retired. Live repositories MUST NOT reference the retired vocabulary
or skill in `openspec/config.yaml`, `docs/`, or tests. References in
`openspec/changes/archive/`, `CHANGELOG.md`, and git history are exempt and
MUST NOT be rewritten.

When a legacy ceremony classification must be translated into plan-build
depth, the system SHALL apply this migration mapping:

- `trivial` and `local_fix` → **Light**
- `behavior_change` → **Standard**
- `domain_change` → **Full**

The mapping applies only to translation of an existing legacy classification
(from an archived change, an external report, or prior tooling). For new work
with no legacy classification, the plan-build classifier signal rules and
explicit depth request handling decide depth, and this mapping SHALL NOT be
consulted.

The depth artifact minima and gates of the mapped tier apply unchanged and
MUST NOT be reduced by the legacy artifact lists (for example, a `trivial`
classification translated to Light still requires `proposal.md` and
`tasks.md`; a `domain_change` translated to Full still requires the Full
minimum artifacts and verify evidence).

Live configuration MUST NOT declare a ceremony decision matrix. In
particular, `openspec/config.yaml` SHALL NOT contain a `sdd:` section with a
`decision_matrix`, and no `recipe.toml` SHALL declare `sdd.threshold` as a
documented or enforced field.

#### Scenario: Legacy config section is a retired-surface violation

- **GIVEN** a repository whose `openspec/config.yaml` contains a `sdd:`
  section declaring `decision_matrix` levels
- **WHEN** the live configuration is scanned for retired ceremony surfaces
- **THEN** the section SHALL be reported as a retired-surface violation
- **AND** the repository MUST remove it to be compliant with this contract

#### Scenario: Retired vocabulary absent from live docs and tests

- **GIVEN** a compliant repository after this change
- **WHEN** `openspec/config.yaml`, `docs/recipe-schema.md`,
  `docs/recipes-catalog.md`, `catalog/recipes/trello-mcp-workflow/README.md`,
  and the manifest/recipe doc-contract tests are scanned
- **THEN** the tokens `sdd-adaptive-contract`, `openspec-sdd-decision`,
  `sdd.decision_matrix`, and `sdd.threshold` SHALL be absent
- **AND** references inside `openspec/changes/archive/`, `CHANGELOG.md`, and
  git history SHALL be excluded from the scan
- **AND** the only permissible token occurrences in the scanned tests SHALL be
  the intentional ban-list literals embedded in the enforcement tests
  themselves (`test_no_retired_ceremony_tokens_in_live_surfaces` and the
  retired-surface assertions in the manifest doc-contract module); any other
  test reference to the retired vocabulary is a violation

#### Scenario: Trivial and local_fix map to Light

- **GIVEN** an archived change classified `trivial` or `local_fix` under the
  retired contract
- **WHEN** its classification is translated to plan-build depth
- **THEN** the decided depth SHALL be `Light`
- **AND** the Light minima (`proposal.md` and `tasks.md`) SHALL apply

#### Scenario: Behavior change maps to Standard

- **GIVEN** an archived change classified `behavior_change`
- **WHEN** its classification is translated to plan-build depth
- **THEN** the decided depth SHALL be `Standard`
- **AND** the Standard minima (`proposal.md`, `tasks.md`, and at least one
  `specs/**/*.md`) SHALL apply

#### Scenario: Domain change maps to Full

- **GIVEN** an archived change classified `domain_change`
- **WHEN** its classification is translated to plan-build depth
- **THEN** the decided depth SHALL be `Full`
- **AND** the Full minima (`tasks.md`, `proposal.md` or `design.md`, and at
  least one `specs/**/*.md`) and the Full verify evidence SHALL apply

#### Scenario: Mapping is not consulted without a legacy classification

- **GIVEN** a new change with no legacy ceremony classification
- **WHEN** the plan-build classifier runs
- **THEN** depth SHALL be decided by the signal tier and explicit depth
  request handling
- **AND** the legacy migration mapping SHALL NOT influence the decision

#### Scenario: Archived records remain untouched

- **GIVEN** a repository that has retired the legacy vocabulary
- **WHEN** archived change folders and historical changelog entries are
  inspected
- **THEN** archived references to `sdd-adaptive-contract` and the retired
  levels SHALL remain intact
- **AND** no archived artifact SHALL be rewritten, moved, or deleted

## Acceptance Criteria (test map)

| AC | Test | Req |
|----|------|-----|
| AC1 | `tests/test_manifest_contract_docs.py::test_recipe_reference_covers_current_v2_contract_and_boundaries` (updated — retired `[sdd]`/`threshold` assertions removed) | Legacy ceremony vocabulary retirement |
| AC2 | `tests/test_manifest_contract_docs.py::test_recipe_doc_has_no_retired_sdd_surface` (new) | Legacy ceremony vocabulary retirement |
| AC3 | `tests/test_manifest_contract_docs.py::test_legacy_ceremony_mapping_table` (new) | Legacy ceremony vocabulary retirement |
| AC4 | `tests/test_manifest_contract_docs.py` or `tests/test_plan_build_flow_recipe.py` retired-token doc scan (new) | Legacy ceremony vocabulary retirement |
| AC5 | `tests/test_plan_build_flow_recipe.py` existing vocabulary-hygiene tests (`FORBIDDEN_TERMS`, `test_recipe_surface_stays_additive_standalone_planning`) stay green | Legacy ceremony vocabulary retirement |
| AC6 | `./tests/validate.sh` green; `openspec/config.yaml` YAML-parseable with no `sdd:` key; `git status -- openspec/changes/archive/` clean | Legacy ceremony vocabulary retirement |
