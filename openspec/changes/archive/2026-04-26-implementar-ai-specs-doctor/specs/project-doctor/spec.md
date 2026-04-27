## ADDED Requirements

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
The system MUST validate the bundled local assets that are expected after initialization and sync.

#### Scenario: Bundled skills present
- **GIVEN** `ai-specs/skills/skill-creator/` and `ai-specs/skills/skill-sync/` exist
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include `OK` checks for bundled skills

#### Scenario: Bundled skill missing
- **GIVEN** one of the bundled skill directories is missing
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include an `ERROR` check naming the missing bundled skill
- **AND** the report MUST recommend running `ai-specs init --force` or `ai-specs refresh-bundled`

#### Scenario: Bundled commands present
- **GIVEN** `ai-specs/commands/` contains bundled command Markdown files
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include an `OK` check for bundled commands

#### Scenario: Bundled commands missing
- **GIVEN** `ai-specs/commands/` is missing or contains no command Markdown files
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include a `WARN` or `ERROR` check explaining the missing command assets

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
