# Design: Add omp (Oh My Pi) Agent Target

## Technical Approach

Approach A — a dedicated, explicit `omp` target that mirrors `pi` across the four
fan-out surfaces (platform fields, flag parsing, runtime-hook render, templates).
`omp` is a Rust pi fork: same event model and TS ExtensionAPI shape, but its own
`.omp/` config root and (unlike pi) native slash commands. The change is purely
additive — a new `omp)` case, a new flag token, a new `render_omp()`, and two
template lines. No shared logic or existing target is touched.

## Architecture Decisions

| Decision | Choice | Alternative rejected | Rationale |
|----------|--------|---------------------|-----------|
| Target model | Dedicated `omp` target (Approach A) | Rely on omp's native inheritance of `.claude`/`AGENTS.md` | Inheritance skips `.omp/commands` + `.omp/extensions` and breaks self-contained per-agent artifacts. |
| MCP path | `.omp/mcp.json` (dedicated) | Reuse `.mcp.json` like pi | pi already owns `.mcp.json`; sharing it would collide when both targets are enabled. Dedicated path isolates omp. |
| commands_dir | `.omp/commands` (populated) | `""` (mirror pi exactly) | omp supports native slash commands; pi does not. This is the key field delta vs pi. |
| Hook render | New `render_omp()` mirroring `render_pi`, writing to `.omp/extensions/<recipe>-<hook>.ts` | Generalize render_pi to take a path arg | Per-agent render functions are the established pattern; a parallel function keeps the diff isolated and backward-compat trivially provable. |
| TS import path | Single module-level constant `OMP_EXT_IMPORT`, defaulting to `@earendil-works/pi-coding-agent` (same as pi) | Hardcode inline like render_pi | omp's upstream package name is unconfirmed (open question). Isolating it as one constant makes a future correction a one-line change with zero structural churn. |
| EVENT_MAP | omp inherits pi's native event names (`tool_call`, `tool_result`, `session_start`, `agent_end`) | New event names | omp is a pi fork; event model is identical. |

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `lib/_internal/platform.sh` | Modify | Add `omp)` case after `pi)`, before final `*)`. |
| `lib/sync-agent.sh` | Modify | Add `--omp` to the flag alternation; add usage line. |
| `lib/_internal/hooks-render.py` | Modify | Add `"omp"` to each EVENT_MAP entry; add `render_omp()` + `OMP_EXT_IMPORT` constant; add `elif agent == "omp"` dispatch branch. |
| `templates/gitignore-root.tmpl` | Modify | Add `.omp/` after `.pi/`. |
| `templates/ai-specs.toml.tmpl` | Modify | Add `omp` to the agents comment/example list. |
| `openspec/specs/omp-agent-target/spec.md` | New | Spec mirroring `pi-agent-target`. |
| `tests/test_sync_pipeline.py`, `tests/test_hooks_render.py` | Modify | omp tests mirroring pi. |

## Interfaces / Contracts

**platform.sh — `omp)` case (all 8 fields):**

```sh
omp)
    # Oh My Pi (can1357/oh-my-pi) — Rust pi fork; AGENTS.md native, .omp/ root.
    case "$field" in
        instructions_path)    echo "" ;;
        skills_dir)           echo ".omp/skills" ;;
        agents_dir)           echo "" ;;
        mcp_config_path)      echo ".omp/mcp.json" ;;   # dedicated; avoids .mcp.json clash with pi
        mcp_key)              echo "mcpServers" ;;
        native)               echo "true" ;;
        commands_dir)         echo ".omp/commands" ;;   # KEY DELTA vs pi ("")
        runtime_hooks_target) echo ".omp/extensions" ;;
        *) return 1 ;;
    esac
    ;;
```

**sync-agent.sh:** extend the flag alternation
`--claude|--cursor|--opencode|--codex|--copilot|--gemini|--pi|--omp)` and add usage line
`  --omp            Oh My Pi     (.omp/skills, .omp/mcp.json, .omp/commands)`.
`--all` needs no change — it reuses `[agents].enabled` and the generic `TARGETS`
loop, so `omp` flows through once it is in the enabled list and registered in platform.sh.

**hooks-render.py:** add module constant
`OMP_EXT_IMPORT = "@earendil-works/pi-coding-agent"`, then `render_omp()` is a
copy of `render_pi` with two changes: path `.omp/extensions/...`, and the import
line built from `OMP_EXT_IMPORT` (refactor render_pi's literal into the same
constant only if trivial; otherwise leave pi untouched for strict backward-compat).

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | `platform_get omp <field>` returns each of the 8 values; invalid field → exit 1 | Mirror pi platform assertions in shell test. |
| Structural | `render_omp` writes `.omp/extensions/demo-shell-gate.ts` containing the import + `pi.on("tool_call"` | Mirror `test_hooks_render.py:102-110` with agent `"omp"` and `.omp/` path. |
| Integration | `--all` syncs omp when `enabled = ['omp']` → `.omp/skills` symlink; NOT synced when absent | Mirror `test_sync_pipeline.py:1113/1135` pi cases. |
| MCP | `.omp/mcp.json` written with `mcpServers` when `[mcp.*]` present; coexists with pi's `.mcp.json` without collision | New assertion exercising both pi+omp enabled. |
| Backward-compat | Existing target outputs byte-identical before/after | Run full `./tests/validate.sh`; no edits to claude/cursor/opencode/pi paths. |

## Migration / Rollout

No migration required. Additive only; revert removes the `omp)` case, flag, render
function, EVENT_MAP entries, template lines, spec, and tests in isolation.

## Open Questions

- [ ] **omp ExtensionAPI TS import path** — defaults to `@earendil-works/pi-coding-agent` via `OMP_EXT_IMPORT`. Verify omp's published package name during apply; if different, change the single constant only.
