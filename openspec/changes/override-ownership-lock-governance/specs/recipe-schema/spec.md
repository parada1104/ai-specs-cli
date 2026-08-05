# recipe-schema (delta)

## ADDED Requirements

### Requirement: Optional template update_policy

A `[[provides.templates]]` entry MAY declare `update_policy` with one of
`auto`, `confirm`, or `never-force`. When absent, the effective policy for
templates SHALL default to `auto` under the override-ownership capability.
Unknown values SHALL fail recipe validation with an explicit error.

#### Scenario: Valid update_policy accepted

- **GIVEN** a template entry with `update_policy = "never-force"`
- **WHEN** the recipe is parsed
- **THEN** validation SHALL pass
- **AND** the parsed template metadata SHALL include `update_policy = "never-force"`

#### Scenario: Invalid update_policy rejected

- **GIVEN** a template entry with `update_policy = "sometimes"`
- **WHEN** the recipe is parsed
- **THEN** validation SHALL fail naming `update_policy`

#### Scenario: Absent update_policy defaults at runtime

- **GIVEN** a template entry without `update_policy`
- **WHEN** the recipe is parsed
- **THEN** validation SHALL pass
- **AND** sync SHALL treat the effective policy as `auto` for that template
