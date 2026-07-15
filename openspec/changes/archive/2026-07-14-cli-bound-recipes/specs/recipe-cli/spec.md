# Spec delta: recipe-cli

## MODIFIED Requirements

### Requirement: recipe list

`recipe list` SHALL show recipe id, name, catalog version as **informational only**, and status (`installed`, `available`, or `disabled`). It MUST NOT expose `outdated` status or a pin-bump workflow.

(Previously: version column implied pin status; outdated/pin-bump UX was planned.)

#### Scenario: Info not pin

- GIVEN an enabled recipe with no toml `version`
- WHEN `recipe list` runs
- THEN catalog version is shown as informational
- AND the recipe is not marked outdated

#### Scenario: Empty and uninitialized

- GIVEN no recipes configured
- WHEN `recipe list` runs
- THEN exit code is 0
- GIVEN no `ai-specs.toml`
- WHEN `recipe list` runs
- THEN exit code is 1

### Requirement: recipe add

`recipe add` SHALL set `enabled=true`, MUST NOT write `version`, and MUST NOT trigger sync/materialize.

(Previously: wrote `version`.)

#### Scenario: Add

- WHEN `recipe add` succeeds
- THEN the manifest contains `enabled=true`
- AND no `version` key is written
- AND no materialize/sync is triggered

### Requirement: CLI catalog resolution

`recipe list`, `recipe add`, and `recipe init` SHALL resolve recipes from the installed CLI catalog. A local catalog copy is not required.

#### Scenario: Authoritative CLI catalog

- GIVEN the CLI ships a recipe in its catalog
- WHEN list/add/init runs
- THEN the CLI catalog is the source of truth

## REMOVED Requirements

### Requirement: recipe update (pin-bump)

Pin-bump via `recipe update` is removed. Users SHALL use CLI upgrade plus sync instead.

### Requirement: Doctor outdated-pin WARN

Outdated-pin doctor UX is removed. Legacy-version WARN and resync notes replace it.

## ADDED Requirements

### Requirement: #104 documentation

Docs SHALL state that `not_exists` managed templates do not refresh on sync.

#### Scenario: #104 docs

- WHEN users read recipe/template docs
- THEN non-refresh behavior for `not_exists` templates is documented

### Requirement: No pin-bump UX

The CLI and hub MUST NOT expose pin-bump or outdated-pin workflows.

#### Scenario: No update path

- WHEN a user seeks to bump a recipe pin
- THEN the supported path is upgrade plus sync, not `recipe update`
