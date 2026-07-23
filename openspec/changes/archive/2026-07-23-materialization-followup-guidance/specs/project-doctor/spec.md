## ADDED Requirements

### Requirement: Tracked bundled-skill leftover guidance

When a project is a git repository and the index still tracks paths under
`ai-specs/skills/<bundled-id>/` for a CLI-bundled skill id (typically after sync
deleted the working-tree copy), `doctor` SHALL emit a WARN that names the
tracked paths and recommends `git rm -r --cached` for those paths. The CLI
MUST NOT run `git rm`, stage, or commit.

#### Scenario: Tracked leftover after disk removal

- **GIVEN** `ai-specs/skills/skill-creator/` is tracked in git
- **AND** the working tree no longer contains that directory (sync removed it)
- **WHEN** `doctor` runs
- **THEN** it reports a WARN about tracked bundled-skill leftovers
- **AND** the guidance includes `git rm -r --cached`
- **AND** the git index is unchanged by doctor
