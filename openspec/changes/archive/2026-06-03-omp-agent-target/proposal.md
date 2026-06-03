# Proposal: Add omp (Oh My Pi) Agent Target

## Intent

`omp` (Oh My Pi, `can1357/oh-my-pi`) is a Rust fork of `pi` with its own binary and `.omp/` config conventions. ai-specs-cli registers `pi` but has no `omp` target, so `omp` users get no explicit, versioned fan-out. `omp` can inherit `.claude`/`.cursor`/`AGENTS.md` natively, but inheritance does not cover `.omp/commands` or `.omp/extensions` hooks and conflicts with the repo philosophy of self-contained per-agent artifacts. A dedicated target mirrors `pi`.

## Scope

### In Scope
- Register `omp` in `lib/_internal/platform.sh` with confirmed fields
- Add `--omp` flag + usage to `lib/sync-agent.sh`; include in `--all` when enabled
- Add `omp` to `lib/_internal/hooks-render.py` EVENT_MAP + `render_omp()` (extension shims to `.omp/extensions`)
- Add `.omp/` to `templates/gitignore-root.tmpl`
- Add `omp` to commented agents list in `templates/ai-specs.toml.tmpl`
- New spec `openspec/specs/omp-agent-target/spec.md` (mirror of `pi-agent-target`)
- Golden tests in `tests/test_sync_pipeline.py` + `tests/test_hooks_render.py`

### Out of Scope
- Reimplementing omp features (LSP, DAP, hashline edits)
- Touching existing targets (claude/cursor/opencode/codex/copilot/gemini/pi) — strict backward-compat
- omp recipe-specific concepts beyond the shared fan-out

## Capabilities

### New Capabilities
- `omp-agent-target`: fan-out of skills, MCP, commands, and runtime hooks to omp via `sync-agent --omp`

### Modified Capabilities
- None (existing targets unchanged; `--all` reuses existing enabled-agent logic)

## Approach

Approach A — dedicated explicit `omp` target, mirroring `pi`. Platform entry:
`instructions_path=""` (AGENTS.md native) · `skills_dir=".omp/skills"` · `agents_dir=""` · `mcp_config_path=".omp/mcp.json"` · `mcp_key="mcpServers"` · `native=true` · `commands_dir=".omp/commands"` · `runtime_hooks_target=".omp/extensions"`.

Differences vs `pi`: dedicated `.omp/mcp.json` (avoids `.mcp.json` collision with pi) AND a populated `commands_dir` (pi has none). `render_omp()` reuses pi's ExtensionAPI shim pattern (omp is a pi fork; the TS import path is the one open question, deferred to apply).

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `lib/_internal/platform.sh` | Modified | Add `omp)` case with confirmed fields |
| `lib/sync-agent.sh` | Modified | Add `--omp` flag, usage/help, `--all` wiring |
| `lib/_internal/hooks-render.py` | Modified | Add `omp` to EVENT_MAP + `render_omp()` |
| `templates/gitignore-root.tmpl` | Modified | Add `.omp/` entry |
| `templates/ai-specs.toml.tmpl` | Modified | Add `omp` to commented agents list |
| `openspec/specs/omp-agent-target/spec.md` | New | Spec mirroring `pi-agent-target` |
| `tests/test_sync_pipeline.py`, `tests/test_hooks_render.py` | Modified | omp tests mirroring pi |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| omp ExtensionAPI TS import path differs from pi | Med | Open question deferred to apply; verify upstream/test before render_omp() |
| `.omp/mcp.json` key wrong | Low | Confirmed `mcpServers` in exploration (0.95) |
| Existing target fan-out regresses | Low | New case + flag only; shared logic untouched; backward-compat tests |

## Rollback Plan

Revert the `omp)` case, `--omp` flag, `render_omp()`/EVENT_MAP entry, template edits, new spec, and tests. Delete any generated `.omp/` artifacts. No existing target touched, so revert is isolated.

## Dependencies

- omp's TypeScript ExtensionAPI package/import path (resolve during apply)

## Success Criteria

- [ ] `platform_get omp <field>` returns correct values; exit 1 on invalid field
- [ ] `ai-specs sync-agent --omp` puts `omp` in target agents; `--help` lists `--omp`
- [ ] Skills fan-out: symlink to `.omp/skills/` (root → resolved-skills; sub-target → ai-specs/skills)
- [ ] MCP fan-out: `.omp/mcp.json` with `mcpServers` when `[mcp.*]` present; omitted if none
- [ ] AGENTS.md native: no instructions symlink for `omp`
- [ ] `--all` includes/excludes `omp` per `[agents].enabled`
- [ ] Backward-compat: existing agent configs byte-identical before/after
- [ ] `.gitignore` root contains `.omp/`
- [ ] Green: focused tests + `./tests/validate.sh`
