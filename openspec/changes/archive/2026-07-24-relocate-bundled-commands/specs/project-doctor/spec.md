## MODIFIED Requirements

### Requirement: Bundled asset diagnostics

The system MUST validate the bundled local assets that are expected after
initialization and sync.

#### Scenario: Bundled skills present

- **GIVEN** `ai-specs/skills/skill-creator/` and `ai-specs/skills/skill-sync/` exist
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include `OK` checks for bundled skills

#### Scenario: Bundled skill missing

- **GIVEN** one of the bundled skill directories is missing
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include an `ERROR` check naming the missing bundled skill
- **AND** the report MUST recommend running `ai-specs init --force` or `ai-specs refresh-bundled`

#### Scenario: Bundled command present

- **GIVEN** a CLI-bundled command id (e.g. `rules-audit`) resolves at
  `{cache}/.bundled/commands/rules-audit.md`
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include an `OK` check naming that bundled command

#### Scenario: Bundled command missing

- **GIVEN** a CLI-bundled command id does not resolve at
  `{cache}/.bundled/commands/{name}.md`
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include an `ERROR` check naming the missing bundled
  command
- **AND** the report MUST recommend running `ai-specs sync`

## ADDED Requirements

### Requirement: Tracked bundled-command leftover guidance

When a project is a git repository and the index still tracks
`ai-specs/commands/{name}.md` for a CLI-bundled command name (typically after
sync deleted the working-tree copy), `doctor` SHALL emit a WARN that names the
tracked paths and recommends `git rm --cached` for those paths. The CLI MUST
NOT run `git rm`, stage, or commit.

#### Scenario: Tracked leftover after disk removal

- **GIVEN** `ai-specs/commands/rules-audit.md` is tracked in git
- **AND** the working tree no longer contains that file (sync removed it)
- **WHEN** `doctor` runs
- **THEN** it reports a WARN about tracked bundled-command leftovers
- **AND** the guidance includes `git rm --cached`
- **AND** the git index is unchanged by doctor
