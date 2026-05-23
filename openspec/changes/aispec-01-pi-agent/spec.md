# Delta Specs: Add Pi Agent Target

## Domain: pi-agent-target

### Requirement: Platform registration

The system MUST register `pi` in `lib/_internal/platform.sh` with:
`skills_dir=.pi/skills`, `mcp_config_path=.mcp.json`, `mcp_key=mcpServers`,
`native=true`, and empty `instructions_path`, `agents_dir`, `commands_dir`.

#### Scenario: Lookup succeeds

- GIVEN `platform_get pi skills_dir` is invoked
- WHEN executed
- THEN it MUST print `.pi/skills` and exit 0

#### Scenario: Invalid field fails

- GIVEN `platform_get pi nonexistent_field` is invoked
- WHEN executed
- THEN it MUST exit 1 (via the `*)` fallback case)

### Requirement: CLI flag

The system MUST accept `--pi` in `lib/sync-agent.sh` argument parsing and usage.

#### Scenario: Explicit flag

- GIVEN `ai-specs sync-agent --pi` is run
- WHEN arguments parse
- THEN `pi` MUST be in target agents

#### Scenario: Help lists Pi

- GIVEN `ai-specs sync-agent --help` is run
- WHEN usage prints
- THEN it MUST include `--pi`

### Requirement: Skills fan-out

The system MUST symlink resolved skills to `.pi/skills/` when syncing to Pi.

#### Scenario: Symlink created

- GIVEN `ai-specs sync-agent --pi` runs
- WHEN sync completes
- THEN `.pi/skills/` MUST be a symlink to `ai-specs/.internal/resolved-skills/`

### Requirement: MCP fan-out

The system MUST render `.mcp.json` with `mcpServers` when MCPs are declared.

#### Scenario: MCP rendered

- GIVEN `[mcp.*]` entries exist
- AND `ai-specs sync-agent --pi` runs
- WHEN sync completes
- THEN `.mcp.json` MUST exist with the `mcpServers` key

#### Scenario: MCP skipped when empty

- GIVEN no `[mcp.*]` entries exist
- AND `ai-specs sync-agent --pi` runs
- WHEN sync completes
- THEN `.mcp.json` MUST NOT be created for Pi

### Requirement: AGENTS.md native

The system MUST NOT create an instruction symlink for Pi.

#### Scenario: No instruction symlink

- GIVEN `ai-specs sync-agent --pi` runs
- WHEN sync completes
- THEN no instruction symlink MUST be created for Pi

### Requirement: No commands fan-out

The system MUST NOT copy slash-command files to Pi.

#### Scenario: Commands skipped

- GIVEN `ai-specs/commands/` has files
- AND `ai-specs sync-agent --pi` runs
- WHEN sync completes
- THEN no commands MUST be copied to a Pi directory

### Requirement: --all integration

The system MUST include Pi when `--all` is used and `pi` is in `[agents].enabled`.

#### Scenario: Enabled Pi included

- GIVEN `[agents].enabled` contains `pi`
- AND `ai-specs sync-agent --all` runs
- WHEN targets resolve
- THEN `pi` MUST be synced

#### Scenario: Disabled Pi excluded

- GIVEN `[agents].enabled` does not contain `pi`
- AND `ai-specs sync-agent --all` runs
- WHEN targets resolve
- THEN `pi` MUST NOT be synced

### Requirement: Backward compatibility

The system MUST NOT affect existing agents when Pi is added.

#### Scenario: Existing agents unchanged

- GIVEN `ai-specs sync-agent --all` runs before and after adding `pi` to enabled
- WHEN outputs are compared
- THEN existing agent configs MUST be identical

### Requirement: Gitignore entry

The system MUST include `.pi/` and `.pi/skills/` in the project root `.gitignore` (rendered from `templates/gitignore-root.tmpl`), alongside other agent output directories.

#### Scenario: Pi skills gitignored

- GIVEN `ai-specs sync-agent --pi` runs
- WHEN workspace is ensured
- THEN the root `.gitignore` MUST contain `.pi/` and `.pi/skills/`

## Domain: project-doctor

### ADDED Requirement: Pi agent diagnostics

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
