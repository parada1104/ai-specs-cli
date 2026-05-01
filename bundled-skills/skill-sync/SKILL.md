---
name: skill-sync
description: >
  Syncs skill metadata to the registry artifact ai-specs/.skill-registry.md.
  Trigger: When updating skill metadata (metadata.scope/metadata.auto_invoke), regenerating the skill registry, or running ai-specs/skills/skill-sync/assets/sync.sh.
license: Apache-2.0
metadata:
  author: prowler-cloud
  version: "1.0"
  scope: [root]
  auto_invoke:
    - "After creating/modifying a skill"
    - "Regenerate skill registry artifact (sync.sh)"
    - "Troubleshoot why a skill is missing from the registry"
allowed-tools: Read, Edit, Write, Glob, Grep, Bash
---

## Purpose

Keeps `ai-specs/.skill-registry.md` in sync with the canonical skill
frontmatter contract documented in
[`../../contracts/skill-frontmatter.md`](../../contracts/skill-frontmatter.md).

`sync.sh` discovers every `ai-specs/skills/<name>/SKILL.md` under the repo,
validates metadata through `lib/_internal/skill_contract.py`, and generates
the registry artifact at `ai-specs/.skill-registry.md`. It does not vendor
external skills; root `ai-specs sync` does that first.

## Required skill metadata

Each skill that should appear in the Auto-invoke mappings needs
`metadata.scope` and `metadata.auto_invoke` as canonical YAML lists. Skills
can live in `ai-specs/skills/<name>/SKILL.md`, `.recipe/<id>/skills/<name>/`,
or `.deps/<id>/skills/<name>/`. See [skill-creator/SKILL.md](../skill-creator/SKILL.md).

### Scope values

Scopes are recorded in the registry artifact's Auto-invoke table. Skills may
use multiple scopes: `scope: [root, docs]`.

## Usage

```bash
ai-specs/skills/skill-sync/assets/sync.sh
ai-specs/skills/skill-sync/assets/sync.sh --dry-run
ai-specs/skills/skill-sync/assets/sync.sh --scope root
bin/ai-specs sync .
```

## Checklist after modifying skills

- [ ] `metadata.scope` and `metadata.auto_invoke` set on new or changed skills
- [ ] Ran `ai-specs/skills/skill-sync/assets/sync.sh` or `bin/ai-specs sync .`
- [ ] Verified `ai-specs/.skill-registry.md`
