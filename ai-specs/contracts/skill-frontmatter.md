# Skill Frontmatter Contract

This document is the human-owned source of truth for `SKILL.md` frontmatter.
Executable enforcement lives in `lib/_internal/skill_contract.py`.

## Canonical local contract

Every local skill at `ai-specs/skills/<name>/SKILL.md` must use this schema:

```yaml
---
name: my-skill
description: >
  One paragraph explaining what the skill enables.
  Trigger: When the AI should auto-load the skill.
license: Apache-2.0
metadata:
  author: your-team
  version: "1.0"
  scope: [root]
  auto_invoke:
    - "Primary action phrase"
---
```

### Required authored fields

- `name`
- `description`
- `license`
- `metadata.author`
- `metadata.version`

### Optional authored fields

- `metadata.scope`
- `metadata.auto_invoke`

`metadata.scope` and `metadata.auto_invoke` are optional for the skill itself,
but they are required together if the skill must appear in the Auto-invoke
mappings in `ai-specs/.skill-registry.md`.

## Generated vendored contract

Vendored skills under `ai-specs/skills/<dep-id>/SKILL.md` are derived output.
They are generated from `[[deps]]` in `ai-specs/ai-specs.toml` plus the upstream
skill body. Do not hand-edit vendored frontmatter.

Canonical generated fields:

- `name` ← `[[deps]].id`
- `description` ← normalized upstream description + vendoring attribution
- `license` ← `[[deps]].license` or `Unknown`
- `metadata.author` ← `[[deps]].vendor_attribution` or `upstream`
- `metadata.version` ← `[[deps]].version` if provided, otherwise `1.0`
- `metadata.source` ← `[[deps]].source`
- `metadata.vendor_attribution` ← `[[deps]].vendor_attribution` when provided
- `metadata.scope` ← `[[deps]].scope` or `[root]`
- `metadata.auto_invoke` ← `[[deps]].auto_invoke`

## Ownership boundaries

- Local skills are authored in `ai-specs/skills/**/SKILL.md`.
- Vendored skills are generated only by root `ai-specs sync`.
- `AGENTS.md` is a generated runtime brief; never hand-edit it.
- `ai-specs/.skill-registry.md` is the generated skill registry artifact; never hand-edit it.
- Subrepo `ai-specs/skills/**` copies are fan-out artifacts from the root run and
  must stay byte-consistent with root-generated output.

## Consumer rules

- `skill-creator` and `/skills-as-rules` must scaffold the canonical local schema.
- `vendor-skills.py` must render vendored skills through
  `lib/_internal/skill_contract.py`.
- `agents-md-render.py` must generate the runtime brief from `ai-specs.toml`
  and must not include skill catalogs or Auto-invoke tables.
- `skill-sync` must validate sync metadata through the shared contract parser,
  not bespoke shell parsing, and must generate `ai-specs/.skill-registry.md`.

## Compatibility window

Compatibility mode is temporary and only exists for first-party rollout.

- Scalar `metadata.auto_invoke: "..."` is normalized to a one-item list.
- Missing first-party `license`, `metadata.author`, or `metadata.version` may be
  normalized with compatibility defaults during rollout.
- Missing or malformed `name` / `description`, malformed sync metadata types,
  and semantically invalid values still fail with actionable errors.

## Hard-fail cutover

After first-party skills are migrated, new or updated local skills are expected
to be canonical at author time. Compatibility mode should then remain only as a
short-lived read path for untouched legacy skills.

## Rollout checklist

1. Restore required baseline fixtures for sync and target resolution tests.
2. Introduce the shared contract parser and document this contract.
3. Migrate Python consumers and `skill-sync` to the shared module.
4. Update first-party skill templates/docs.
5. Run `ai-specs sync` to regenerate derived outputs.
