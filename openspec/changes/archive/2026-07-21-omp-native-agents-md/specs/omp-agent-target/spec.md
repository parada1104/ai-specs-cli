## MODIFIED Requirements

### Requirement: Platform registration

The system MUST register `omp` in `lib/_internal/platform.sh` with:
`skills_dir=.omp/skills`, `mcp_config_path=.omp/mcp.json`, `mcp_key=mcpServers`,
`native=true`, `commands_dir=.omp/commands`, `runtime_hooks_target=.omp/extensions`,
`instructions_path=.omp/AGENTS.md`, and empty `agents_dir`.

#### Scenario: Lookup succeeds

- GIVEN `platform_get omp skills_dir` is invoked
- WHEN executed
- THEN it MUST print `.omp/skills` and exit 0

#### Scenario: All fields return correct values

- GIVEN `platform_get omp <field>` is invoked for each registered field
- WHEN executed
- THEN `instructions_path` MUST return `.omp/AGENTS.md`, `skills_dir` MUST return `.omp/skills`, `agents_dir` MUST return `""`, `mcp_config_path` MUST return `.omp/mcp.json`, `mcp_key` MUST return `mcpServers`, `native` MUST return `true`, `commands_dir` MUST return `.omp/commands`, `runtime_hooks_target` MUST return `.omp/extensions`

#### Scenario: Invalid field fails

- GIVEN `platform_get omp nonexistent_field` is invoked
- WHEN executed
- THEN it MUST exit 1 (via the `*)` fallback case)

### Requirement: AGENTS.md native slot

The system MUST route omp's runtime brief through its native, highest-priority
provider slot by symlinking `.omp/AGENTS.md` to the root `AGENTS.md`. This is
required because omp's `agents-md` provider ignores any `AGENTS.md` whose parent
directory name starts with a dot and loads the standalone root file only at the
lowest provider priority; the native `.omp/AGENTS.md` provider has the highest
priority and shadows other providers at the same depth.

#### Scenario: Native instruction symlink created

- GIVEN `ai-specs sync-agent --omp` runs
- WHEN sync completes
- THEN `.omp/AGENTS.md` MUST be a symlink resolving to the root `AGENTS.md`

#### Scenario: Symlink is relative

- GIVEN `ai-specs sync-agent --omp` runs with `TARGET_PATH == SOURCE_ROOT`
- WHEN sync completes
- THEN the `.omp/AGENTS.md` symlink target MUST be the relative path `../AGENTS.md`
