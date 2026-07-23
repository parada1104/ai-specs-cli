# harness-cli-literacy Specification

## Purpose

Document always-on harness CLI literacy skills shipped by the CLI.

## Requirements

### Requirement: refresh-bundled literacy matches cache flatten

The `harness-lifecycle` bundled skill SHALL document that CLI-bundled skills are
flattened into `{cache}/.bundled/skills/` on sync/`refresh-bundled` and are not
copied into `ai-specs/skills/`. It SHALL NOT describe `.new` sidecars for
bundled-skill refresh.

#### Scenario: Skill text describes cache flatten

- **GIVEN** `bundled-skills/harness-lifecycle/SKILL.md`
- **WHEN** an agent reads the refresh-bundled section
- **THEN** it explains cache flatten / no in-project materialization
- **AND** it does not instruct reviewers to resolve `SKILL.md.new` for bundled skills
