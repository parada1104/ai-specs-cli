# Skills by Agent

How skills are surfaced to each supported agent. The canonical skill index
lives in each `SKILL.md` frontmatter (`metadata.scope`, `metadata.auto_invoke`).
`ai-specs sync` validates this metadata through `skill-sync`.

| Agent    | Reads AGENTS.md natively? | Native skill auto-invoke? | What sync-agent generates |
|----------|---------------------------|---------------------------|---------------------------|
| Claude   | No (needs `CLAUDE.md`)    | Yes (`.claude/skills/<name>/SKILL.md`) | `CLAUDE.md` symlink + `.claude/skills` symlink → `ai-specs/skills` + `.mcp.json` |
| Cursor   | Yes                       | Yes (`.cursor/skills/<name>/SKILL.md`) | `.cursor/skills` symlink → resolved-skills + `.cursor/commands` + `.cursor/mcp.json` + runtime hooks |
| OpenCode | Yes                       | No                                     | `opencode.json` |
| Codex    | Yes                       | No                                     | `.codex/config.toml` |
| Copilot  | No (`.github/copilot-instructions.md`) | No                          | `.github/copilot-instructions.md` symlink |
| Gemini   | No (needs `GEMINI.md`)    | Yes (`.gemini/skills/<name>/SKILL.md`) | `GEMINI.md` symlink + `.gemini/skills` symlink + `.gemini/settings.json` |

## Harness CLI literacy (always-on)

Bundled skills `harness-lifecycle`, `harness-recipes`, and `harness-skills-deps`
ship via `refresh-bundled` into every project. They teach agents how to operate
the public `ai-specs` CLI (init/sync, recipes, skills/deps, doctor).

On agents with native skill auto-invoke (Claude, Cursor, Gemini), intent-matched
triggers load those skills. On agents without auto-invoke (OpenCode, Codex,
Copilot, and typically pi/omp), the generated `AGENTS.md` `## Useful Commands`
section includes a fixed pointer to the same harness skill ids (resolved via
agent fan-out from the CLI cache, not from `ai-specs/skills/`).
