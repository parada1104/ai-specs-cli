---
name: skill-creator
description: >
 Creates new AI agent skills following the Agent Skills spec.
 Trigger: When user asks to create a new skill, add agent instructions, or document patterns for AI.
license: Apache-2.0
metadata:
 author: prowler-cloud
 version: "1.0"
 scope: [root]
 auto_invoke:
  - "Creating new skills"
allowed-tools: Read, Edit, Write, Glob, Grep, Bash, WebFetch, WebSearch, Task
---

## When to Create a Skill

Create a skill when:
- A pattern is used repeatedly and AI needs guidance
- Project-specific conventions differ from generic best practices
- Complex workflows need step-by-step instructions
- Decision trees help AI choose the right approach

**Don't create a skill when:**
- Documentation already exists (create a reference instead)
- Pattern is trivial or self-explanatory
- It's a one-off task

---

## Skill Structure

```
skills/{skill-name}/
├── SKILL.md              # Required - main skill file
├── assets/               # Optional - templates, schemas, examples
│   ├── template.py
│   └── schema.json
└── references/           # Optional - links to local docs
    └── docs.md
```

---

## Naming (melon-alquimia)

| Type | Pattern | Examples |
|------|---------|----------|
| Monorepo / infra (root `skills/`) | `melon-{area}` | `melon-monorepo` |
| App / MS (under `<subrepo>/skills/<name>/`) | `melon-{area}` or `melon-ms-{service}` | `melon-front-web`, `melon-ms-auth` |
| Meta | `skill-*` | `skill-creator`, `skill-sync` |

Use `metadata.scope` values defined in [skill-sync/SKILL.md](../skill-sync/SKILL.md).

---

## Frontmatter Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Skill identifier (lowercase, hyphens) |
| `description` | Yes | What + Trigger |
| `license` | Yes | SPDX id (e.g. `Apache-2.0`, `MIT`) |
| `metadata.author` | Yes | Team or `prowler-cloud` for upstream-derived |
| `metadata.version` | Yes | Semantic version string |
| `metadata.scope` | For sync | Scopes whose `AGENTS.md` get Auto-invoke rows |
| `metadata.auto_invoke` | For sync | Action phrases as a YAML list |

Canonical reference: [`../../contracts/skill-frontmatter.md`](../../contracts/skill-frontmatter.md).

---

## Registering the Skill

Do not hand-edit `AGENTS.md`. Run `ai-specs sync` after creating or changing
skill metadata so the root-generated files are refreshed through the supported
workflow.

---

## Checklist Before Creating

- [ ] Skill name not already in `skills/`
- [ ] Pattern is reusable
- [ ] Frontmatter matches `ai-specs/contracts/skill-frontmatter.md`
- [ ] Commands section with copy-paste commands
- [ ] Run `ai-specs sync` after adding `scope` + `auto_invoke`

## Resources

- **Templates**: [assets/SKILL-TEMPLATE.md](assets/SKILL-TEMPLATE.md)
