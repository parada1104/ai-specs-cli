## MODIFIED Requirements

### Requirement: Command merge

Commands SHALL resolve from three sources with source-level precedence,
highest to lowest:

1. `ai-specs/commands/{name}.md` — local hand-authored (highest)
2. `{cache}/commands/{name}.md` — recipe-managed
3. `{cache}/.bundled/commands/{name}.md` — CLI-bundled, flattened from
   `$AI_SPECS_HOME/bundled-commands/` (lowest)

CLI-bundled commands are recipe-independent and shipped by the CLI. They SHALL
NOT be materialized into `ai-specs/commands/` and SHALL NOT be committed to the
project. On conflict, hand-authored commands win over both lower tiers;
recipe-managed commands win over CLI-bundled commands. Fan-out targets remain
unchanged.

#### Scenario: Merge and fan-out

- **GIVEN** a command id present in both cache-managed and hand-authored locations
- **WHEN** fan-out runs
- **THEN** the hand-authored command wins
- **AND** agent command targets match pre-change behavior aside from source relocation

#### Scenario: Bundled command resolves from cache, not project

- **GIVEN** a CLI-bundled command `rules-audit` and no local or recipe-managed
  command of that name
- **WHEN** `sync` (or `refresh-bundled`) runs
- **THEN** it SHALL flatten `rules-audit.md` into `{cache}/.bundled/commands/`
- **AND** `ai-specs/commands/rules-audit.md` MUST NOT exist afterward
- **AND** the merged command set used for fan-out MUST still include
  `rules-audit`

#### Scenario: Local command shadows a CLI-bundled command of the same name

- **GIVEN** a local `ai-specs/commands/skills-as-rules.md` authored in the project
- **AND** a CLI-bundled command `skills-as-rules` in `{cache}/.bundled/commands/`
- **WHEN** commands are merged for fan-out
- **THEN** the local copy SHALL be selected
- **AND** it SHALL NOT be deleted during leftover cleanup only if its content
  differs from the bundled source (see Requirement: Tracked bundled-command
  leftover guidance, `project-doctor` capability, for the identical-content
  case)

#### Scenario: Recipe-managed command shadows a CLI-bundled command of the same name

- **GIVEN** a recipe-managed command in `{cache}/commands/{name}.md`
- **AND** a CLI-bundled command of the same name in `{cache}/.bundled/commands/`
- **AND** no local command of that name
- **WHEN** commands are merged for fan-out
- **THEN** the recipe-managed copy SHALL be selected
- **AND** no warning SHALL be emitted (both are CLI-driven tiers)
