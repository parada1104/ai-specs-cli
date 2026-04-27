## Why

Projects can currently run `ai-specs init` and `ai-specs sync`, but there is no single diagnostic command that explains whether the generated project state is healthy. This makes setup drift, missing generated files, disabled agents, incomplete bundled assets, and MCP render issues harder to detect before an agent or developer starts work.

## What Changes

- Add a new `ai-specs doctor` CLI command that diagnoses an ai-specs project without mutating it.
- Validate required project inputs and generated outputs, including `ai-specs/ai-specs.toml`, `AGENTS.md`, enabled agent outputs, bundled skills, bundled commands, generated symlinks, and MCP configs when declared.
- Report checks with clear `OK`, `WARN`, and `ERROR` severities so humans and agents can decide whether to run `init`, `sync`, or fix configuration manually.
- Return a non-zero exit code when blocking errors are found, while warnings remain advisory.
- Keep the command scoped to diagnostics only; it must not refresh bundled assets, vendor dependencies, regenerate files, or edit manifests.

## Capabilities

### New Capabilities
- `project-doctor`: Diagnoses whether an ai-specs project is initialized, synchronized, and structurally consistent enough for agent tooling to rely on generated configuration.

### Modified Capabilities
- None.

## Impact

- Affected CLI entrypoint: `bin/ai-specs` dispatch table and help output.
- Affected shell modules: new `lib/doctor.sh` wrapper following existing command style.
- Potential Python helper: new `lib/_internal/doctor.py` if implementation benefits from structured manifest parsing and file checks.
- Affected tests: new focused unittest coverage for doctor checks and CLI/help behavior, plus existing `./tests/validate.sh` final validation.
- No new runtime dependencies beyond Bash and Python 3.11+ standard library.
- Rollback plan: remove the `doctor` dispatch/help entry, delete `lib/doctor.sh`, delete any `lib/_internal/doctor.py`, and remove the associated tests/spec delta. Existing commands and generated artifacts remain unaffected because doctor is read-only.
