# Tasks: Add Pi Agent Target

## Phase 1: Foundation — Platform Registration

- [x] 1.1 Add `pi` case block to `lib/_internal/platform.sh` with `skills_dir=.pi/skills`, `mcp_config_path=.mcp.json`, `mcp_key=mcpServers`, `native=true`, empty `instructions_path`, `agents_dir`, `commands_dir`. Also update the agent list comment in the file header (line 6) to include `pi`.
- [x] 1.2 Add `.pi/` and `.pi/skills/` entries to `templates/gitignore-root.tmpl` under the managed `ai-specs` block, alongside existing agent output directories (`.claude/`, `.opencode/`, etc.)

## Phase 2: Core — CLI Wiring

- [x] 2.1 Add `--pi` flag case to argument parsing in `lib/sync-agent.sh` (~line 70, alongside `--codex|--copilot|--gemini`)
- [x] 2.2 Add `--pi` line to `usage()` help text in `lib/sync-agent.sh`
- [x] 2.3 Append `"pi"` to supported agents comment in `ai-specs/ai-specs.toml`

## Phase 3: Integration — Doctor

- [x] 3.1 Add `pi` entry to `PLATFORM` dict in `lib/_internal/doctor.py` mirroring `platform.sh`. Note: `--all` wiring is NOT needed — `sync-agent.sh` already reads `[agents].enabled` and syncs all listed agents automatically.

## Phase 4: Testing & Verification

- [x] 4.1 Unit: `platform_get pi skills_dir` returns `.pi/skills`, `platform_get pi commands_dir` returns empty string (exit 0 — defined-but-empty field), `platform_get pi nonexistent_field` exits 1 via the `*)` fallback
- [x] 4.2 Integration: `sync-agent --pi` creates `.pi/skills/` symlink and `.mcp.json` with `mcpServers`
- [x] 4.3 Regression: `sync-agent --all` produces identical existing-agent configs before and after adding Pi
- [x] 4.4 Doctor: `ai-specs doctor` accepts `pi` as valid agent (not flagged unknown), reports OK for valid symlink/MCP, ERROR for broken symlink
- [x] 4.5 Automated tests: Extend `tests/test_doctor.py` — verify `pi` is in PLATFORM dict and is NOT rejected as unknown agent by `test_unknown_enabled_agent_reports_error`. Add `pi`-specific enabled-agent test mirroring `test_enabled_agent_output_present_reports_ok`.
