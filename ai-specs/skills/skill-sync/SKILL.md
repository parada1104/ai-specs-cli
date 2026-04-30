---
name: skill-sync
description: >
  Validates skill frontmatter and prepares separated skill/capability registries.
  In runtime-brief projects, AGENTS.md must remain project context and must not
  be rewritten as an Auto-invoke registry.
license: Apache-2.0
metadata:
  author: prowler-cloud
  version: "2.0"
  scope: [root]
  auto_invoke:
    - "After creating/modifying a skill"
    - "Validating skill metadata"
    - "Preparing skill or capability registries"
allowed-tools: Read, Edit, Write, Glob, Grep, Bash
---

# skill-sync

This skill documents the current runtime contract for skill metadata. It does not authorize running `ai-specs sync` in repositories where `AGENTS.md` is still a manual runtime brief.

## Contract

- `AGENTS.md` is project runtime context, not a full skill registry.
- Skills keep valid frontmatter: `name`, `description`, `metadata.scope`, and `metadata.auto_invoke` as YAML lists.
- Empty `metadata.auto_invoke: []` is valid for optional/reference skills.
- Skill and capability inventories belong in separate generated artifacts once the CLI renderer supports them.

## Current Dogfooding Rule

If `AGENTS.md` contains `<!-- ai-specs:runtime-brief -->`, direct `skill-sync` checks may validate metadata, but must not insert an Auto-invoke section into `AGENTS.md`.

Do not run root `ai-specs sync` in this repo until the runtime-brief renderer and registry split are implemented.

## After Modifying Skills

- Keep canonical `ai-specs/skills/<name>/SKILL.md` and runtime copies aligned manually while the CLI renderer is transitional.
- Verify no skill tells agents to regenerate `AGENTS.md` as an Auto-invoke registry.
- Prefer concise skill bodies; project-specific details belong in `AGENTS.md` or manifest/config.
- Preserve secret hygiene: generated docs or registries must not expose env-backed MCP values.

## Future CLI Behavior

Card #65 should productize this by making the CLI:

- Generate a stable runtime brief from manifest/project config.
- Generate skill/capability registries outside `AGENTS.md`.
- Validate skill metadata without bloating startup context.
- Keep direct registry generation idempotent.
