# project-doctor Specification

## Purpose

Define `ai-specs doctor`, a read-only diagnostic command that inspects an
ai-specs project's structure, manifest, agents, bundled assets, generated
symlinks, MCP wiring, environment substrate, CLI version, and leftover tracked
files, and reports actionable findings without mutating the project.

## Requirements

### Requirement: Doctor command availability
The system MUST expose `ai-specs doctor [path]` as a read-only diagnostic command for ai-specs projects.

#### Scenario: Help lists doctor
- **GIVEN** the CLI is installed from this repository
- **WHEN** a user runs `ai-specs help`
- **THEN** the help output MUST list `doctor` as an available command
- **AND** the description MUST identify it as a diagnostic command

#### Scenario: Doctor accepts target path
- **GIVEN** an ai-specs project path is provided as an argument
- **WHEN** a user runs `ai-specs doctor <path>`
- **THEN** the command MUST inspect that path instead of the current working directory

#### Scenario: Doctor is read-only
- **GIVEN** a project is inspected by `ai-specs doctor`
- **WHEN** the command completes
- **THEN** it MUST NOT create, modify, delete, vendor, refresh, or regenerate project files

### Requirement: Core project structure diagnostics
The system MUST validate the core files and directories required for an initialized ai-specs project.

#### Scenario: Manifest exists
- **GIVEN** a target project contains `ai-specs/ai-specs.toml`
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include an `OK` check for the manifest

#### Scenario: Manifest missing
- **GIVEN** a target project does not contain `ai-specs/ai-specs.toml`
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include an `ERROR` check for the missing manifest
- **AND** the command MUST exit non-zero

#### Scenario: Generated AGENTS exists
- **GIVEN** a target project contains `AGENTS.md`
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include an `OK` check for generated agent instructions

#### Scenario: Generated AGENTS missing
- **GIVEN** a target project does not contain `AGENTS.md`
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include an `ERROR` check for missing generated agent instructions
- **AND** the report MUST recommend running `ai-specs init` or `ai-specs sync`

### Requirement: Manifest-driven agent diagnostics
The system MUST inspect enabled agents from `[agents].enabled` and validate their expected generated outputs.

#### Scenario: No enabled agents
- **GIVEN** the manifest has no enabled agents
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include a `WARN` check explaining that no agents are enabled
- **AND** the command MUST NOT fail solely because no agents are enabled

#### Scenario: Unknown enabled agent
- **GIVEN** the manifest includes an enabled agent name outside the supported agent set
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include an `ERROR` check naming the unsupported agent
- **AND** the command MUST exit non-zero

#### Scenario: Enabled agent outputs present
- **GIVEN** the manifest enables an agent
- **AND** that agent's expected generated outputs exist for the target project
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include `OK` checks for that agent's generated outputs

#### Scenario: Enabled agent output missing
- **GIVEN** the manifest enables an agent
- **AND** a generated output expected for that agent is missing
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include an `ERROR` check naming the missing output
- **AND** the report MUST recommend running `ai-specs sync`

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

### Requirement: Generated symlink diagnostics
The system MUST validate generated symlinks for agents whose platform configuration uses instruction or skill symlinks.

#### Scenario: Instruction symlink valid
- **GIVEN** an enabled agent expects an instruction symlink to `AGENTS.md`
- **AND** the symlink exists and resolves to the target project's `AGENTS.md`
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include an `OK` check for that symlink

#### Scenario: Instruction symlink invalid
- **GIVEN** an enabled agent expects an instruction symlink to `AGENTS.md`
- **AND** the path is missing, is not a symlink, or resolves somewhere else
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include an `ERROR` check naming the invalid symlink
- **AND** the report MUST recommend running `ai-specs sync`

#### Scenario: Skill symlink valid
- **GIVEN** an enabled agent expects a skill directory symlink to `ai-specs/skills`
- **AND** the symlink exists and resolves to the target project's `ai-specs/skills`
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include an `OK` check for that symlink

#### Scenario: Copied skill directory valid
- **GIVEN** an enabled agent expects copied project-local skills instead of a symlink
- **AND** the target skill directory exists and contains skill directories
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include an `OK` check for the copied skill directory

### Requirement: MCP diagnostics
The system MUST validate generated MCP configuration files when the manifest declares MCP servers and the enabled agent supports MCP output.

#### Scenario: No MCP servers declared
- **GIVEN** the manifest has no `[mcp.*]` entries
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include a `WARN` check explaining that no MCP servers are declared
- **AND** the command MUST NOT fail solely because no MCP servers are declared

#### Scenario: MCP config present for enabled supporting agent
- **GIVEN** the manifest declares one or more `[mcp.*]` entries
- **AND** an enabled agent supports generated MCP configuration
- **AND** that agent's MCP config file exists
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include an `OK` check for that MCP config file

#### Scenario: MCP config missing for enabled supporting agent
- **GIVEN** the manifest declares one or more `[mcp.*]` entries
- **AND** an enabled agent supports generated MCP configuration
- **AND** that agent's MCP config file is missing
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include an `ERROR` check naming the missing MCP config file
- **AND** the report MUST recommend running `ai-specs sync`

### Requirement: Doctor report and exit code
The system MUST produce a clear report with deterministic severities and an exit code suitable for automation.

#### Scenario: Healthy project exits zero
- **GIVEN** all required doctor checks pass and only `OK` or `WARN` checks are present
- **WHEN** `ai-specs doctor` runs
- **THEN** the command MUST print a summary of check counts
- **AND** the command MUST exit `0`

#### Scenario: Project with errors exits non-zero
- **GIVEN** one or more doctor checks produce `ERROR`
- **WHEN** `ai-specs doctor` runs
- **THEN** the command MUST print a summary of check counts
- **AND** the command MUST exit non-zero

#### Scenario: Clear severity labels
- **GIVEN** `ai-specs doctor` reports checks
- **WHEN** output is printed
- **THEN** each check line MUST include exactly one of `OK`, `WARN`, or `ERROR`
- **AND** each non-OK check MUST include actionable guidance or identify the missing/invalid artifact

### Requirement: Pi agent diagnostics

The system MUST validate Pi-specific outputs when `pi` is enabled.

#### Scenario: Pi recognized as valid

- GIVEN `[agents].enabled` contains `pi`
- WHEN `ai-specs doctor` runs
- THEN `pi` MUST NOT be flagged as an unknown agent

#### Scenario: Pi skills symlink valid

- GIVEN `pi` is enabled
- AND `.pi/skills/` is a valid symlink
- WHEN `ai-specs doctor` runs
- THEN the report MUST include `OK` for Pi skills

#### Scenario: Pi skills symlink invalid

- GIVEN `pi` is enabled
- AND `.pi/skills/` is missing or broken
- WHEN `ai-specs doctor` runs
- THEN the report MUST include `ERROR` for Pi skills

#### Scenario: Pi MCP config present

- GIVEN `pi` is enabled
- AND `[mcp.*]` entries exist
- AND `.mcp.json` exists
- WHEN `ai-specs doctor` runs
- THEN the report MUST include `OK` for Pi MCP

#### Scenario: Pi MCP config missing

- GIVEN `pi` is enabled
- AND `[mcp.*]` entries exist
- AND `.mcp.json` is missing
- WHEN `ai-specs doctor` runs
- THEN the report MUST include `ERROR` for Pi MCP

#### Scenario: Pi instruction not expected

- GIVEN `pi` is enabled
- WHEN `ai-specs doctor` runs
- THEN the report MUST NOT flag a missing Pi instruction symlink

### Requirement: Tracked bundled-skill leftover guidance

`doctor` SHALL emit a WARN, when a project is a git repository and the index still
tracks paths under `ai-specs/skills/<bundled-id>/` for a CLI-bundled skill id
(typically after sync deleted the working-tree copy), that names the tracked paths
and recommends `git rm -r --cached` for those paths. The CLI MUST NOT run
`git rm`, stage, or commit.

#### Scenario: Tracked leftover after disk removal

- **GIVEN** `ai-specs/skills/skill-creator/` is tracked in git
- **AND** the working tree no longer contains that directory (sync removed it)
- **WHEN** `doctor` runs
- **THEN** it reports a WARN about tracked bundled-skill leftovers
- **AND** the guidance includes `git rm -r --cached`
- **AND** the git index is unchanged by doctor

### Requirement: Tracked bundled-command leftover guidance

`doctor` SHALL emit a WARN, when a project is a git repository and the index still
tracks `ai-specs/commands/{name}.md` for a CLI-bundled command name (typically
after sync deleted the working-tree copy), that names the tracked paths and
recommends `git rm --cached` for those paths. The CLI MUST NOT run `git rm`,
stage, or commit.

#### Scenario: Tracked leftover after disk removal

- **GIVEN** `ai-specs/commands/rules-audit.md` is tracked in git
- **AND** the working tree no longer contains that file (sync removed it)
- **WHEN** `doctor` runs
- **THEN** it reports a WARN about tracked bundled-command leftovers
- **AND** the guidance includes `git rm --cached`
- **AND** the git index is unchanged by doctor

### Requirement: direnv substrate diagnostics

When enabled recipes declare MCP env variable references, `ai-specs doctor` SHALL
report whether `direnv` is available on PATH. Missing `direnv` MUST be WARN (not
ERROR) and MUST include install guidance. Doctor MUST NOT install `direnv`.

#### Scenario: direnv missing with MCP env required

- **GIVEN** an enabled recipe MCP env references `$TRELLO_API_KEY`
- **AND** `direnv` is not on PATH
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include a WARN check for `direnv`
- **AND** guidance MUST mention how to install or enable direnv
- **AND** doctor MUST NOT run an installer

#### Scenario: No MCP env skips direnv warn

- **GIVEN** no enabled recipe MCP env references
- **AND** `direnv` is not on PATH
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST NOT require a `direnv` WARN solely for that absence

### Requirement: Managed root .envrc diagnostics

When enabled recipes declare MCP env variable references, doctor SHALL WARN if
project-root `.envrc` is missing or lacks the ai-specs managed-by markers.
Doctor MUST remain read-only.

#### Scenario: Missing managed block

- **GIVEN** MCP env vars are required by enabled recipes
- **AND** project-root `.envrc` exists without managed-by markers
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include a WARN recommending
  `ai-specs configure-recipes` (or equivalent) to ensure the managed block

### Requirement: Harness env key diagnostics

When enabled recipes declare MCP env variable references, doctor SHALL WARN for
each required variable that is missing or empty in project-root `ai-specs.env`.
Doctor MUST NOT print secret values.

#### Scenario: Empty harness key

- **GIVEN** enabled recipes require `TRELLO_TOKEN`
- **AND** `ai-specs.env` exists but `TRELLO_TOKEN` is missing or empty
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include a WARN naming `TRELLO_TOKEN`
- **AND** the message MUST NOT include any secret value

#### Scenario: Present harness key is OK

- **GIVEN** enabled recipes require `TRELLO_TOKEN`
- **AND** `ai-specs.env` contains a non-empty `TRELLO_TOKEN`
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST NOT WARN for that key as missing

### Requirement: CLI version diagnostics

The system MUST report CLI version state as part of `ai-specs doctor` output.

The report MUST include, when available:

- **installed** — version from `AI_SPECS_HOME/VERSION`
- **pinned** — from manifest `[tool]` when configured
- **last_synced** — from `ai-specs/.ai-specs.lock` `[meta].cli_version` when present

#### Scenario: All version sources present and aligned

- **GIVEN** installed CLI `0.12.2`
- **AND** manifest `[tool].version = "0.12.2"`
- **AND** lock `[meta].cli_version = "0.12.2"`
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include an `OK` check named `cli-version`
- **AND** the message MUST mention installed, pinned, and last-synced values

#### Scenario: No pin configured with last sync recorded

- **GIVEN** installed CLI `0.12.2`
- **AND** no `[tool]` section in the manifest
- **AND** lock `[meta].cli_version = "0.10.1"`
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include a `WARN` check named `cli-version`
- **AND** the message MUST note installed differs from last-synced
- **AND** the message SHOULD suggest running `ai-specs sync` or adding a `[tool]` pin

#### Scenario: Exact pin mismatch is ERROR

- **GIVEN** installed CLI `0.11.0`
- **AND** manifest `[tool].version = "0.12.2"` with policy `exact`
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include an `ERROR` check named `cli-version`
- **AND** the command MUST exit non-zero

#### Scenario: Min version violation is ERROR

- **GIVEN** installed CLI `0.10.0`
- **AND** manifest `[tool].min_version = "0.11.0"`
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include an `ERROR` check named `cli-version`
- **AND** the command MUST exit non-zero

#### Scenario: Lock meta absent is INFO

- **GIVEN** installed CLI `0.12.2`
- **AND** no `[tool]` section
- **AND** lock file exists without `[meta]`
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include an `INFO` or `WARN` check noting last-synced is unknown
- **AND** the message SHOULD recommend running `ai-specs sync` to record meta

#### Scenario: Doctor remains read-only

- **GIVEN** any project state
- **WHEN** `ai-specs doctor` inspects CLI version
- **THEN** it MUST NOT modify the manifest, lock file, or any derived artifacts

### Requirement: Active-change missing Tracker link section WARN

When all of the following hold, `ai-specs doctor` SHALL scan active OpenSpec
change folders and WARN for missing/invalid card-link artifacts:

1. The `trello-mcp-workflow` recipe is enabled in the project manifest.
2. The recipe bootstrap-ready marker is present at the canonical runtime cache path
   `cache/projects/<hash>-<name>/.recipe/trello-mcp-workflow/bootstrap-ready`
   (same location materialize writes), or at the project-local fallback
   `.recipe/trello-mcp-workflow/bootstrap-ready` used by hermetic tests.

For each directory matching `openspec/changes/<slug>/` that is **not** under
`openspec/changes/archive/`:

- If `tracker.none` (`tracker:none` exemption) is present → no missing-card WARN
  for that slug.
- Else if the `## Tracker` link section is absent, or present but invalid
  (missing a non-empty `card_id` per `trello-card-linking` validity rules) →
  emit `Severity.WARN` naming the slug and remediation guidance to
  create/link a card and write the `## Tracker` link section. A missing `url`
  is only an informational nudge and MUST NOT make the link invalid.

Default severity is WARN only. Doctor MUST NOT fail the command exit solely
because of these WARN findings (no FAIL-by-default in v1). Doctor MUST remain
read-only.

Archived changes MUST NOT be migrated or warned by this check.

#### Scenario: WARN when recipe and marker present and Tracker link section missing

- **GIVEN** `trello-mcp-workflow` is enabled
- **AND** the bootstrap-ready marker exists under the project recipe cache
- **AND** `openspec/changes/demo-change/` exists without a `## Tracker` link section and without
  `tracker.none`
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include a `WARN` naming `demo-change`
- **AND** guidance MUST mention creating/linking a card and writing the `## Tracker` section
- **AND** the command MUST still exit `0` if no unrelated `ERROR` checks exist

#### Scenario: Valid Tracker link section is OK

- **AND** the change's `proposal.md` `## Tracker` section contains non-empty
  `card_id` (and SHOULD include `url` when available)
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST NOT WARN for `demo-change` as missing a card link

#### Scenario: tracker:none suppresses missing-card WARN

- **GIVEN** recipe enabled and bootstrap marker present
- **AND** `openspec/changes/demo-change/tracker.none` exists
- **AND** the `## Tracker` link section is absent
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST NOT WARN for `demo-change` as missing a card link

#### Scenario: Silent when recipe disabled

- **GIVEN** `trello-mcp-workflow` is not enabled
- **AND** an active change lacks the `## Tracker` link section
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST NOT emit the tracker missing-card WARN

#### Scenario: Silent when bootstrap marker absent

- **GIVEN** `trello-mcp-workflow` is enabled
- **AND** the bootstrap-ready marker is absent from the recipe cache path
- **AND** an active change lacks the `## Tracker` link section
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST NOT emit the tracker missing-card WARN

#### Scenario: Archives are not warned

- **GIVEN** recipe enabled and bootstrap marker present
- **AND** only `openspec/changes/archive/...` folders lack the `## Tracker` link section
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST NOT WARN those archive slugs for missing card links

#### Scenario: Invalid Tracker link section warns

- **GIVEN** recipe enabled and bootstrap marker present
- **AND** the change's `proposal.md` `## Tracker` section exists but has an empty or
  missing `card_id`
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include a `WARN` naming `demo-change` as lacking a
  valid card-link artifact
