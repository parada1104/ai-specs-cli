## Domain: omp-agent-target

### Requirement: Platform registration

The system MUST register `omp` in `lib/_internal/platform.sh` with:
`skills_dir=.omp/skills`, `mcp_config_path=.omp/mcp.json`, `mcp_key=mcpServers`,
`native=true`, `commands_dir=.omp/commands`, `runtime_hooks_target=.omp/extensions`,
and empty `instructions_path`, `agents_dir`.

#### Scenario: Lookup succeeds

- GIVEN `platform_get omp skills_dir` is invoked
- WHEN executed
- THEN it MUST print `.omp/skills` and exit 0

#### Scenario: All fields return correct values

- GIVEN `platform_get omp <field>` is invoked for each registered field
- WHEN executed
- THEN `instructions_path` MUST return `""`, `skills_dir` MUST return `.omp/skills`, `agents_dir` MUST return `""`, `mcp_config_path` MUST return `.omp/mcp.json`, `mcp_key` MUST return `mcpServers`, `native` MUST return `true`, `commands_dir` MUST return `.omp/commands`, `runtime_hooks_target` MUST return `.omp/extensions`

#### Scenario: Invalid field fails

- GIVEN `platform_get omp nonexistent_field` is invoked
- WHEN executed
- THEN it MUST exit 1 (via the `*)` fallback case)

### Requirement: CLI flag

The system MUST accept `--omp` in `lib/sync-agent.sh` argument parsing and usage.

#### Scenario: Explicit flag

- GIVEN `ai-specs sync-agent --omp` is run
- WHEN arguments parse
- THEN `omp` MUST be in target agents

#### Scenario: Help lists omp

- GIVEN `ai-specs sync-agent --help` is run
- WHEN usage prints
- THEN it MUST include `--omp`

### Requirement: Skills fan-out

The system MUST symlink resolved skills to `.omp/skills/` when syncing to omp.

#### Scenario: Symlink created in root target

- GIVEN `ai-specs sync-agent --omp` runs with `TARGET_PATH == SOURCE_ROOT`
- WHEN sync completes
- THEN `.omp/skills/` MUST be a symlink to `ai-specs/.internal/resolved-skills/`

#### Scenario: Symlink created in sub-target fan-out

- GIVEN `ai-specs sync-agent --omp` runs with a sub-target (`TARGET_PATH != SOURCE_ROOT`)
- WHEN sync completes
- THEN `.omp/skills/` under the sub-target MUST be a symlink to that sub-target's `ai-specs/skills/`

### Requirement: MCP fan-out

The system MUST render `.omp/mcp.json` with `mcpServers` when MCPs are declared.

#### Scenario: MCP rendered

- GIVEN `[mcp.*]` entries exist
- AND `ai-specs sync-agent --omp` runs
- WHEN sync completes
- THEN `.omp/mcp.json` MUST exist with the `mcpServers` key

#### Scenario: MCP skipped when empty

- GIVEN no `[mcp.*]` entries exist
- AND `ai-specs sync-agent --omp` runs
- WHEN sync completes
- THEN `.omp/mcp.json` MUST NOT be created for omp

### Requirement: Commands fan-out

The system MUST copy slash-command files to `.omp/commands/` when syncing to omp.

#### Scenario: Commands populated

- GIVEN `ai-specs/commands/` has files
- AND `ai-specs sync-agent --omp` runs
- WHEN sync completes
- THEN those files MUST be present under `.omp/commands/`

#### Scenario: Commands dir absent when source empty

- GIVEN `ai-specs/commands/` has no files
- AND `ai-specs sync-agent --omp` runs
- WHEN sync completes
- THEN `.omp/commands/` MUST NOT be created or MUST be empty

### Requirement: Runtime hooks fan-out

The system MUST render runtime hook shims to `.omp/extensions/` when syncing to omp.

#### Scenario: Extensions shims created

- GIVEN runtime hooks are declared
- AND `ai-specs sync-agent --omp` runs
- WHEN sync completes
- THEN shim files MUST exist under `.omp/extensions/`

### Requirement: AGENTS.md native

The system MUST NOT create an instruction symlink for omp.

#### Scenario: No instruction symlink

- GIVEN `ai-specs sync-agent --omp` runs
- WHEN sync completes
- THEN no instruction symlink MUST be created for omp

### Requirement: --all integration

The system MUST include omp when `--all` is used and `omp` is in `[agents].enabled`.

#### Scenario: Enabled omp included

- GIVEN `[agents].enabled` contains `omp`
- AND `ai-specs sync-agent --all` runs
- WHEN targets resolve
- THEN `omp` MUST be synced

#### Scenario: Disabled omp excluded

- GIVEN `[agents].enabled` does not contain `omp`
- AND `ai-specs sync-agent --all` runs
- WHEN targets resolve
- THEN `omp` MUST NOT be synced

### Requirement: Backward compatibility

The system MUST NOT affect existing agents when omp is added.

#### Scenario: Existing agents unchanged

- GIVEN `ai-specs sync-agent --all` runs before and after adding `omp` to enabled
- WHEN outputs are compared
- THEN existing agent configs MUST be byte-identical

### Requirement: Gitignore entry

The system MUST include `.omp/` in the project root `.gitignore` (rendered from `templates/gitignore-root.tmpl`), alongside other agent output directories.

#### Scenario: omp directory gitignored

- GIVEN `ai-specs sync-agent --omp` runs
- WHEN workspace is ensured
- THEN the root `.gitignore` MUST contain `.omp/`
