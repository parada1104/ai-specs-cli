# `agents.d/` — user content for AGENTS.md

This directory holds Markdown fragments that get **concatenated into the
project's `AGENTS.md`** every time `specs-ai sync` runs. AGENTS.md itself is
a generated artifact — edit fragments here, never the rendered file.

## Conventions

- Files are sorted **alphabetically** before concatenation. Use a numeric prefix
  to control order:
  - `00-intro.md`           ← shows first
  - `10-monorepo-map.md`
  - `20-conventions.md`
  - `99-legal.md`           ← shows last
- Files starting with `_` (drafts) and this `README.md` are skipped.
- Each fragment is included **verbatim** — write Markdown directly. Use `##`
  headings for section titles.
- Don't duplicate content the renderer already emits: project title, the
  Skills index, the `Auto-invoke Skills` table, or the "How AI tooling is
  wired" footer.

## What gets generated automatically

The CLI renders, in order:

1. `# {project name} — Agent Instructions`
2. **Your `agents.d/*.md` fragments** (this directory)
3. `## Skills` — table derived from `ai-specs/skills/*/SKILL.md` frontmatter
4. `### Auto-invoke Skills` — table populated by `skill-sync` from each
   skill's `auto_invoke` metadata
5. `## How AI tooling is wired` — footer explaining the standard
