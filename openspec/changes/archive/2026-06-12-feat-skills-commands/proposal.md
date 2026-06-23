# Proposal: Add `ai-specs skills` subcommand group

## Problem

`ai-specs add-dep` is a flat command for registering vendored skills. It's
inconsistent with the `recipe` subcommand group pattern (`recipe list`, `recipe
add`, `recipe init`). As the skill ecosystem grows, users need more operations:
list registered skills, remove a dep, discover available catalog skills.

## Proposal

Add a `skills` subcommand group mirroring the `recipe` pattern:

```
ai-specs skills add <url> [path] [flags]   Register a vendored skill ([[deps]])
ai-specs skills list [path]                List registered + local + catalog skills
ai-specs skills remove <id> [path]         Remove a [[deps]] entry
```

Keep `add-dep` as a backward-compatible alias pointing to `skills add`.

## Scope

- New files: `lib/skills.sh` (dispatcher), `lib/skills-add.sh`, `lib/skills-list.sh`, `lib/skills-remove.sh`
- Modified files: `bin/ai-specs` (add `skills` case, redirect `add-dep`)
- Tests: new `test_skills_add.py`, `test_skills_remove.py`
- Docs: update README with new commands

## Non-goals

- Not modifying the `[[deps]]` schema in `ai-specs.toml`
- Not changing `lib/add-dep.sh` (preserved as standalone script)
- Not implementing `skills update` (future)
