# skill-registry-artifact Specification

> **RETIRED** — The skill registry artifact (`ai-specs/.skill-registry.md`) was removed.
> Skill metadata validation is now performed by `skill-sync` without generating a registry file.
> The `<available_skills>` block in `AGENTS.md` is the
> canonical agent-facing skill index. Individual `SKILL.md` frontmatter remains the source of
> truth for auto-invoke triggers.

## Historical Note

This spec previously defined a generated registry artifact containing a skill index and
auto-invoke mappings table. It was removed because:

1. The `<available_skills>` block in `AGENTS.md` already provides the agent with a skill
   index including name, description, and path.
2. Each `SKILL.md` frontmatter contains `metadata.auto_invoke`, which agents read directly.
3. The registry file was never consumed by any agent at runtime — it was purely human
   documentation, redundant with the `AGENTS.md` block and individual skill frontmatter.
4. Agent skill directories (`.opencode/skills/`, `.claude/skills/`, etc.) are now symlinks
   to the resolved skill tree, eliminating stale skill accumulation.