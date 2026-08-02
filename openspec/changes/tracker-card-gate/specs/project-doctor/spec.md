# project-doctor (delta)

## ADDED Requirements

### Requirement: Active-change missing trello.md WARN

When all of the following hold, `ai-specs doctor` SHALL scan active OpenSpec
change folders and WARN for missing/invalid card-link artifacts:

1. The `trello-mcp-workflow` recipe is enabled in the project manifest.
2. The recipe bootstrap-ready marker is present at the runtime cache path
   `cache/projects/<hash>-<name>/.recipe/trello-mcp-workflow/bootstrap-ready`
   (same location materialize writes; not a project-local `.recipe/` path).

For each directory matching `openspec/changes/<slug>/` that is **not** under
`openspec/changes/archive/`:

- If `tracker.none` (`tracker:none` exemption) is present → no missing-card WARN
  for that slug.
- Else if `trello.md` is absent, or present but invalid (missing non-empty
  `card_id` or `url` per `trello-card-linking` validity rules) → emit
  `Severity.WARN` naming the slug and remediation guidance to create/link a
  card and write `openspec/changes/<slug>/trello.md`.

Default severity is WARN only. Doctor MUST NOT fail the command exit solely
because of these WARN findings (no FAIL-by-default in v1). Doctor MUST remain
read-only.

Archived changes MUST NOT be migrated or warned by this check.

#### Scenario: WARN when recipe and marker present and trello.md missing

- **GIVEN** `trello-mcp-workflow` is enabled
- **AND** the bootstrap-ready marker exists under the project recipe cache
- **AND** `openspec/changes/demo-change/` exists without `trello.md` and without
  `tracker.none`
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include a `WARN` naming `demo-change`
- **AND** guidance MUST mention creating/linking a card and writing `trello.md`
- **AND** the command MUST still exit `0` if no unrelated `ERROR` checks exist

#### Scenario: Valid trello.md is OK

- **GIVEN** recipe enabled and bootstrap marker present
- **AND** `openspec/changes/demo-change/trello.md` contains non-empty `card_id`
  and `url`
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST NOT WARN for `demo-change` as missing a card link

#### Scenario: tracker:none suppresses missing-card WARN

- **GIVEN** recipe enabled and bootstrap marker present
- **AND** `openspec/changes/demo-change/tracker.none` exists
- **AND** `trello.md` is absent
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST NOT WARN for `demo-change` as missing a card link

#### Scenario: Silent when recipe disabled

- **GIVEN** `trello-mcp-workflow` is not enabled
- **AND** an active change lacks `trello.md`
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST NOT emit the tracker missing-card WARN

#### Scenario: Silent when bootstrap marker absent

- **GIVEN** `trello-mcp-workflow` is enabled
- **AND** the bootstrap-ready marker is absent from the recipe cache path
- **AND** an active change lacks `trello.md`
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST NOT emit the tracker missing-card WARN

#### Scenario: Archives are not warned

- **GIVEN** recipe enabled and bootstrap marker present
- **AND** only `openspec/changes/archive/...` folders lack `trello.md`
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST NOT WARN those archive slugs for missing card links

#### Scenario: Invalid trello.md warns

- **GIVEN** recipe enabled and bootstrap marker present
- **AND** `openspec/changes/demo-change/trello.md` exists but has an empty or
  missing `card_id`
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include a `WARN` naming `demo-change` as lacking a
  valid card-link artifact
