# Design: Add Pi Agent Target

## Technical Approach

Treat Pi as a **native agent** that reads `AGENTS.md` at the project root, receives skills via a symlinked `.pi/skills/` directory, and consumes MCP servers through `.mcp.json` under the `mcpServers` key. Pi requires no new rendering logic — it reuses the Claude/Cursor JSON path and the generic MCP translator. The change is a **platform registration + flag wiring** with no new file generators.

## Architecture Decisions

| Decision | Options | Tradeoffs | Choice |
|----------|---------|-----------|--------|
| Pi `native` flag | `true` vs `false` | `true`: AGENTS.md at root is read natively, no symlink needed. `false`: would require a dedicated instruction file symlink (none exists for Pi). | `true` — Pi docs confirm root-level `AGENTS.md` support. |
| MCP format | Reuse `.mcp.json` + `mcpServers` vs new format | Pi adapter v2.7.0 already validates against the Claude/Cursor JSON schema. New format = unnecessary divergence. | Reuse existing JSON path — no translator needed. |
| Skills delivery | Symlink vs copy | OpenCode uses copy (`skills_copy=true`) because its skill loader has symlink restrictions. Pi docs confirm symlink support for `.pi/skills/`. | Symlink (default pattern, same as claude/gemini). |
| Commands | Skip vs stub empty dir | Pi has no slash-command UX; it uses browser extensions. An empty stub dir would mislead users. | Skip entirely (`commands_dir=""`). |
| Recipes | In scope vs out of scope | Pi has no recipe concept today. Adding it now would require a recipe runtime on Pi that does not exist. | **Out of scope** — deferred until Pi supports bundle loading. |

## Data Flow

```
 ai-specs.toml          RESOLVED_SKILLS_DIR/         AGENTS.md
      │                        │                        │
      │    [agents].enabled    │                        │
      └─────────┬───────────────┘                        │
                │                                        │
         sync-agent.sh ──→ per-agent loop               │
                │                                        │
      ┌─────────┴──────────┬─────────────┬───────────────┘
      │                    │             │
  platform_get          skills_dir  instructions_path
  (pi case)             (.pi/skills)   (skip — native)
      │                    │
      │               symlink created
      │                    │
      └────────────→ .mcp.json
                     (mcpServers key)
                     via mcp-render.py
                     (generic JSON, no
                      translator needed)
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `lib/_internal/platform.sh` | Modify | Add `pi` case block with `skills_dir=.pi/skills`, `mcp_config_path=.mcp.json`, `mcp_key=mcpServers`, `native=true`, empty strings for `instructions_path`, `agents_dir`, `commands_dir`. |
| `lib/sync-agent.sh` | Modify | Add `--pi` to flag-parsing case; add `--pi` line to `usage()` help text. |
| `ai-specs/ai-specs.toml` | Modify | Append `"pi"` to supported agents comment. |
| `lib/_internal/doctor.py` | Modify | Add `pi` entry to `PLATFORM` dict (mirrors `platform.sh`). |
| `.gitignore` | Modify | Add `.pi/` and `.pi/skills/` under the managed-ai-specs block. |

## Interfaces / Contracts

### `platform.sh` — Pi case

```bash
pi)
    case "$field" in
        instructions_path) echo "" ;;
        skills_dir)        echo ".pi/skills" ;;
        agents_dir)        echo "" ;;
        mcp_config_path)   echo ".mcp.json" ;;
        mcp_key)           echo "mcpServers" ;;
        native)            echo "true" ;;
        commands_dir)      echo "" ;;
        *) return 1 ;;
    esac
    ;;
```

### `doctor.py` — Pi platform entry

```python
"pi": {
    "instructions_path": "",
    "skills_dir": ".pi/skills",
    "mcp_config_path": ".mcp.json",
    "mcp_key": "mcpServers",
    "commands_dir": "",
},
```

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | `platform_get pi <field>` returns correct values | Bash ` bats` or inline `[[ $(platform_get pi skills_dir) == ".pi/skills" ]]` in `tests/run.sh`. |
| Integration | `sync-agent --pi` creates `.pi/skills/` symlink and updates `.mcp.json` | Run `ai-specs sync-agent --pi` in a temp project; assert symlink target and JSON key presence. |
| Integration | `sync-agent --all` includes Pi when enabled | Temp project with `[agents].enabled = ["pi"]`, run `--all`, assert `.pi/skills/` exists. |
| Regression | Existing agents (claude, opencode) unaffected | Run `ai-specs sync-agent --all` in `ai-specs-cli` repo itself; assert `.claude/` and `.opencode/` unchanged. |
| Doctor | `ai-specs doctor` accepts `pi` in enabled list | Add `pi` to test manifest enabled list; assert doctor reports OK for agent. |

## Migration / Rollout

No migration required. This is purely additive:
1. Merge platform + sync-agent changes.
2. Projects opt-in by adding `"pi"` to `[agents].enabled`.
3. Run `ai-specs sync-agent --all` (or `--pi`) to generate `.pi/skills/` and `.mcp.json`.

Rollback: remove `pi` from `[agents].enabled`, delete `.pi/skills/` symlink and `.mcp.json` (if Pi was the only agent using it).

## Open Questions

- [ ] **None** — Pi adapter v2.7.0 validates the `.mcp.json` format; docs confirm `.pi/skills/` path. All decisions are bounded by existing patterns.
