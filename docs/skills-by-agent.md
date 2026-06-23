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
