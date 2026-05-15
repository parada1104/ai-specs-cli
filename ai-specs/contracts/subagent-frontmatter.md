# Subagent Frontmatter Contract

This document is the human-owned source of truth for SDD subagent files distributed by the CLI. It is intentionally separate from the `Skill Frontmatter Contract` (`ai-specs/contracts/skill-frontmatter.md`): skills live in `SKILL.md` files and participate in auto-invoke; subagent files live in harness-specific paths (e.g., `.claude/agents/*.md`) and follow each harness's native conventions.

## Scope

Applies to bundled subagent files shipped by ai-specs-cli under `bundled-agents/<harness>/*.md` and materialized into a consumer project under harness-specific directories (e.g., `.claude/agents/`) when `[sdd].sub_agents = true` in `ai-specs.toml`.

Currently in scope:

- `bundled-agents/claude/sdd-*.md` → `.claude/agents/sdd-*.md` in the consumer project.

OpenCode and Cursor do not receive subagent files in V1; SDD phases run inline through the primary orchestrator (fallback documented in the runtime brief).

## Canonical frontmatter for Claude Code subagent files

```yaml
---
name: sdd-explore
description: One-line summary of the phase role. Triggers and constraints belong in the body.
tools: Read, Grep, Glob
---
```

### Required fields

- `name` — exact slug of the subagent (`sdd-explore`, `sdd-proposal`, `sdd-artifacts`, `sdd-apply`, `sdd-verify`, `sdd-archive`). Must match the file basename without extension.
- `description` — single line summarizing the phase role. Used by the harness to surface the agent in selection UIs.
- `tools` — comma-separated list of tool names allowed for this subagent. Use exact tool names recognized by Claude Code (e.g., `Read, Grep, Bash, mcp__trello__get_card`). Use `all` only when intentional.

### Optional fields

- `model` — override the default model (e.g., `claude-sonnet-4-6`). Leave omitted in V1 so the user's selected model applies.

### Forbidden patterns

- Frontmatter keys not listed above are not supported by the bundled catalog. Adding custom keys is acceptable only when the harness recognizes them; otherwise they should live in the body.
- Do not duplicate `name` or `description` in the body.

## Canonical body structure

The body uses Markdown and SHOULD include these sections in order so that humans and tools can scan the role quickly:

1. `# <name>` — heading.
2. `**Phase**: <phase>` — one-line phase tag.
3. `## Role` — what this subagent does and what it owns.
4. `## Allowed tools` — explicit list with usage notes; mirrors and elaborates frontmatter `tools`.
5. `## Blocked tools` — explicit list of denylist patterns and why.
6. `## Turn budget` — numeric turn budget plus behavior when exhausted.
7. `## Handoff format` — what the subagent returns to the orchestrator.
8. `## Out of scope` — what the subagent must escalate instead of doing.

## Generated vs. authored

Subagent files in `bundled-agents/<harness>/*.md` are authored in this repository. The materialized copies under a consumer project's `.claude/agents/` are **derived artifacts**: the renderer (`lib/_internal/agents-render.py`) produces byte-identical copies from the bundled source, respecting `.ai-specs.lock` and `.new` sidecars for user-modified files.

Do not hand-edit the materialized copies expecting persistence. To change behavior:

- For the catalog: edit `bundled-agents/<harness>/<name>.md` in the CLI repo.
- For a single project: edit the local file and accept the `.new` sidecar on next sync, or persist the change upstream.

## Ownership boundaries

- Bundled subagent source: ai-specs-cli maintainers.
- Materialized copies: derived, treated like generated artifacts.
- Per-project overrides: live in the user's workspace and surface via `.new` sidecars when the bundled source moves.

## Compatibility expectations

- The catalog is closed: six subagents, fixed slugs. Adding a new subagent is a new SDD change in the CLI.
- The frontmatter keys (`name`, `description`, `tools`, optional `model`) are stable. Removing a key from the catalog is a breaking change for consumers and requires a deprecation cycle.

## Relation to other contracts

- `Skill Frontmatter Contract` (`ai-specs/contracts/skill-frontmatter.md`) governs `SKILL.md` files. Subagent files are not skills.
- The `sdd-subagent-deployment` capability (`openspec/specs/sdd-subagent-deployment/spec.md`) governs the deployment semantics. This contract governs the file format.
