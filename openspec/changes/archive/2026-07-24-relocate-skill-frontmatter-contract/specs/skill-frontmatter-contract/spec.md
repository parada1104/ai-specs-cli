# Delta for skill-frontmatter-contract

## MODIFIED Requirements

### Requirement: Contract documentation and ownership boundaries

The system MUST publish a human-owned contract document for skill frontmatter and define ownership boundaries for local, vendored, and fan-out skill files.

#### Scenario: Contract document describes required and generated fields

- **GIVEN** `bundled-skills/skill-creator/assets/skill-frontmatter-contract.md`
- **WHEN** a maintainer reads the contract
- **THEN** it MUST document required local fields, optional sync metadata, generated vendored fields, compatibility behavior, and hard-fail cutover expectations

#### Scenario: Generated skill files are treated as derived output

- **GIVEN** vendored skills or subrepo skill copies exist after sync
- **WHEN** a maintainer needs to change their metadata
- **THEN** the contract MUST direct changes to the manifest or root local skill source instead of hand-editing derived output
