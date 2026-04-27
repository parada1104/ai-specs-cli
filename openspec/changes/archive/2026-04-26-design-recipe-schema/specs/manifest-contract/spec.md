# manifest-contract Delta Specification

## Purpose

Extend the manifest V1 contract to recognize `[recipes.<id>]` as an optional top-level section.

## MODIFIED Requirements

### Requirement: Superficie canónica mínima del manifiesto
The system MUST treat `ai-specs/ai-specs.toml` as V1 source of truth and recognize `[project]`, `[agents]`, `[[deps]]`, `[mcp.<name>]`, `[sdd]`, and **optionally** `[recipes.<id>]` as canonical sections.

#### Scenario: Manifest V1 mínimo válido con recipes
- **WHEN** a manifest includes `[recipes.<id>]` along with other canonical sections
- **THEN** the manifest MUST remain valid
- **AND** the omission of `[recipes.*]` MUST NOT invalidate the manifest

### Requirement: Campo mínimo por sección
The `[recipes.<id>]` section MUST require `enabled` (boolean) and `version` (string) as minimum fields.

#### Scenario: Campo mínimo para recipes
- **WHEN** `[recipes.my-recipe]` declares `enabled = true` and `version = "1.0.0"`
- **THEN** validation MUST pass

#### Scenario: Campo faltante en recipe
- **WHEN** `[recipes.my-recipe]` omits `version`
- **THEN** validation MUST fail with an explicit error
