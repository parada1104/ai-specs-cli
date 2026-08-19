---
name: skill-sync
description: >
  Validates skill metadata (scope, auto_invoke) across all skill sources.
  Trigger: When adding or modifying skills, or to troubleshoot missing metadata.
license: Apache-2.0
metadata:
  author: prowler-cloud
  version: "2.0"
  scope: [root]
  auto_invoke:
    - "After creating/modifying a skill"
    - "Troubleshoot why a skill has missing or invalid metadata"
allowed-tools: Read, Glob, Bash
---

## Purpose

Validates that every skill in the project has complete sync metadata
(`metadata.scope` and `metadata.auto_invoke`) required by the CLI sync pipeline.

`sync.sh` discovers every `SKILL.md` under the repo (across local, recipe, and
dep sources), validates metadata through `lib/_internal/skill_contract.py`, and
reports skills with missing or invalid fields. It does **not** generate any
registry artifact — that was removed in v2.0.

## Required skill metadata

Each skill that should participate in auto-invoke needs `metadata.scope` and
`metadata.auto_invoke` as canonical YAML lists. Skills can live in
`ai-specs/skills/<name>/SKILL.md`, `.recipe/<id>/skills/<name>/`, or
`.deps/<id>/skills/<name>/`. See [skill-creator/SKILL.md](../skill-creator/SKILL.md).

### Scope values

Scopes control which agent targets receive a skill. Skills may use multiple
scopes: `scope: [root, docs]`.

## Usage

```bash
ai-specs/skills/skill-sync/assets/sync.sh
ai-specs/skills/skill-sync/assets/sync.sh --dry-run
bin/ai-specs sync .
```

## Checklist after modifying skills

- [ ] `metadata.scope` and `metadata.auto_invoke` set on new or changed skills
- [ ] Ran `ai-specs/skills/skill-sync/assets/sync.sh` or `bin/ai-specs sync .`
- [ ] No skills reported with missing metadata