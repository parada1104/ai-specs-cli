# skill-frontmatter-contract Specification

## Purpose

Definir y hacer cumplir el contrato canonico de frontmatter para skills locales y vendored, manteniendo metadatos reproducibles para sync, fan-out y Auto-invoke.

## Requirements

### Requirement: Canonical local skill frontmatter
The system MUST define canonical frontmatter for local `SKILL.md` files, including required identity and provenance fields and optional sync metadata.

#### Scenario: Canonical local skill parses successfully
- **GIVEN** a local `SKILL.md` with `name`, `description`, `license`, `metadata.author`, `metadata.version`, and optional `metadata.scope` plus `metadata.auto_invoke`
- **WHEN** the skill contract reader parses it
- **THEN** the skill MUST be accepted as canonical
- **AND** the description summary MUST remove trailing trigger text used only for auto-invoke discovery

#### Scenario: Manual-only local skill remains valid
- **GIVEN** a local `SKILL.md` without `metadata.scope` or `metadata.auto_invoke`
- **WHEN** the skill is parsed for generated skill indexes
- **THEN** the skill MUST remain valid
- **AND** it MUST NOT produce an Auto-invoke row

#### Scenario: Compatibility mode normalizes legacy local metadata
- **GIVEN** a first-party local skill missing rollout fields or using scalar `metadata.auto_invoke`
- **WHEN** compatibility mode is enabled
- **THEN** the system MUST normalize the skill with compatibility defaults and warnings
- **AND** semantically invalid names, descriptions, or sync metadata MUST still fail

### Requirement: Canonical vendored skill generation
The system MUST generate vendored skill frontmatter from manifest `[[deps]]` inputs plus upstream skill content, without relying on hand-edited vendored frontmatter.

#### Scenario: Vendored metadata is derived from manifest inputs
- **GIVEN** a dependency with `id`, `source`, `scope`, `auto_invoke`, `license`, and `vendor_attribution`
- **WHEN** root sync vendors the skill
- **THEN** the generated `SKILL.md` MUST include canonical name, license, source, vendor attribution, scope, and auto-invoke metadata
- **AND** the generated description MUST preserve upstream description while removing upstream trigger text from the summary

#### Scenario: Hand-edited vendored frontmatter is rewritten
- **GIVEN** a vendored `SKILL.md` whose frontmatter was manually modified after sync
- **WHEN** sync runs again from the same manifest dependency
- **THEN** the vendored frontmatter MUST be rewritten from manifest and upstream inputs
- **AND** manual frontmatter edits MUST NOT persist

#### Scenario: Vendored fan-out remains byte-consistent
- **GIVEN** root sync fans out derived artifacts to declared subrepos
- **WHEN** a vendored skill is present
- **THEN** the root-generated vendored skill and each subrepo copy MUST be byte-identical

### Requirement: Auto-invoke metadata validation
The system MUST validate that skills intended for Auto-invoke have complete sync metadata.

#### Scenario: Incomplete sync metadata fails actionably
- **GIVEN** a skill declares `metadata.scope` without `metadata.auto_invoke`, or `metadata.auto_invoke` without `metadata.scope`
- **WHEN** sync attempts to build Auto-invoke rows
- **THEN** sync MUST fail with an actionable error naming the missing field and skill path

#### Scenario: Complete sync metadata generates registry artifact rows
- **GIVEN** a local or vendored skill with complete `metadata.scope` and `metadata.auto_invoke`
- **WHEN** `ai-specs sync` runs
- **THEN** the generated registry artifact MUST include one Auto-invoke row per trigger for matching scopes
- **AND** `AGENTS.md` MUST NOT contain Auto-invoke rows

### Requirement: Contract documentation and ownership boundaries
The system MUST publish a human-owned contract document for skill frontmatter and define ownership boundaries for local, vendored, and fan-out skill files.

#### Scenario: Contract document describes required and generated fields
- **GIVEN** `ai-specs/contracts/skill-frontmatter.md`
- **WHEN** a maintainer reads the contract
- **THEN** it MUST document required local fields, optional sync metadata, generated vendored fields, compatibility behavior, and hard-fail cutover expectations

#### Scenario: Generated skill files are treated as derived output
- **GIVEN** vendored skills or subrepo skill copies exist after sync
- **WHEN** a maintainer needs to change their metadata
- **THEN** the contract MUST direct changes to the manifest or root local skill source instead of hand-editing derived output
