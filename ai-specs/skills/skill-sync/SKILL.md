---
name: skill-sync
description: >
  Syncs skill metadata to AGENTS.md Auto-invoke sections.
  Trigger: When updating skill metadata (metadata.scope/metadata.auto_invoke), regenerating Auto-invoke tables, or running ai-specs/skills/skill-sync/assets/sync.sh.
license: Apache-2.0
metadata:
  author: prowler-cloud
  version: "1.0"
  scope: [root]
  auto_invoke:
    - "After creating/modifying a skill"
    - "Regenerate AGENTS.md Auto-invoke tables (sync.sh)"
    - "Troubleshoot why a skill is missing from AGENTS.md auto-invoke"
allowed-tools: Read, Edit, Write, Glob, Grep, Bash
---

## Purpose

Keeps `AGENTS.md` **Auto-invoke** sections in sync with the canonical skill
frontmatter contract documented in
[`../../contracts/skill-frontmatter.md`](../../contracts/skill-frontmatter.md).

`sync.sh` discovers every `ai-specs/skills/<name>/SKILL.md` under the repo,
validates metadata through `lib/_internal/skill_contract.py`, and rewrites the
generated auto-invoke section in `AGENTS.md`. It does not vendor external
skills; root `ai-specs sync` does that first.

## Required skill metadata

Each skill that should appear in Auto-invoke needs `metadata.scope` and
`metadata.auto_invoke` as canonical YAML lists. Skills can live only in
`ai-specs/skills/<name>/SKILL.md` and are fanned out from the root in
multi-target mode. See [skill-creator/SKILL.md](../skill-creator/SKILL.md).

### Scope values

This repo currently renders all discovered scopes into the root `AGENTS.md`
unless a monorepo marker adds per-scope routing. Skills may use multiple
scopes: `scope: [root, docs]`.

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
- [ ] Verified affected `AGENTS.md` files
