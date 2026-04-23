---
name: skill-sync
description: >
  Syncs skill metadata to AGENTS.md Auto-invoke sections for melon-alquimia.
  Trigger: When updating skill metadata (metadata.scope/metadata.auto_invoke), regenerating Auto-invoke tables, or running ./skills/skill-sync/assets/sync.sh (including --dry-run/--scope/--vendor).
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

Keeps `AGENTS.md` **Auto-invoke** sections in sync with skill frontmatter. Derived from [prowler-cloud/prowler `skill-sync`](https://github.com/prowler-cloud/prowler/tree/master/skills/skill-sync).

**Monorepo vs standalone:** If **`$REPO_ROOT/.melon-monorepo`** exists, `get_agents_path` maps melon scopes to each subrepo’s `AGENTS.md` and root `AGENTS.md`. If the marker is **absent**, all scopes are merged into **one** `$REPO_ROOT/AGENTS.md` (Lego / single-repo checkout). Standalone trees should set **`skills/.scope`** (one line, e.g. `apis_designv2`) for `vendor-skills.sh` when not passing `--scope`.

**Vendored externals:** [`../vendor.manifest.toml`](../vendor.manifest.toml) + [`assets/vendor-skills.sh`](assets/vendor-skills.sh) + [`assets/vendor-manifest-normalize.py`](assets/vendor-manifest-normalize.py) download upstream `SKILL.md` and inject melon frontmatter. `sync.sh --vendor` runs the vendor script for **`$REPO_ROOT/skills` only** before syncing; use `./skills/vendor-skills.sh --all-subrepos` to refresh every subrepo listed in [`../subrepos.txt`](../subrepos.txt).

**Discovery:** `sync.sh` finds every file matching `*/skills/*/SKILL.md` under the monorepo root (including `$REPO_ROOT/skills/...` and `$REPO_ROOT/<subrepo>/skills/...`). Paths under `node_modules`, `.git`, or `.worktrees` are ignored.

## Required skill metadata

Each skill that should appear in Auto-invoke needs `metadata.scope` and `metadata.auto_invoke` (string or YAML list). Skills can live in the root `skills/` tree or under a subrepo’s `skills/` folder. See [skill-creator/SKILL.md](../skill-creator/SKILL.md).

### Scope values (melon-alquimia)

| Scope | Updates |
|-------|---------|
| `root` | `AGENTS.md` (repo root) |
| `front_web` | `alquimia-front-web/AGENTS.md` |
| `back_web` | `alquimia-back-web/AGENTS.md` |
| `apis_design` | `alquimia-apis-design/AGENTS.md` |
| `apis_designv2` | `apis-designv2/AGENTS.md` |
| `ml` | `alquimia-ml/AGENTS.md` |
| `ms_auth` | `alquimia-ms-auth/AGENTS.md` |
| `ms_memories` | `alquimia-ms-memories/AGENTS.md` |
| `ms_products` | `alquimia-ms-products/AGENTS.md` |
| `ms_recomendations` | `alquimia-ms-recomendations/AGENTS.md` |

Skills may use multiple scopes: `scope: [root, front_web]`.

## Usage

```bash
./skills/skill-sync/assets/sync.sh
./skills/skill-sync/assets/sync.sh --dry-run
./skills/skill-sync/assets/sync.sh --scope root
./skills/skill-sync/assets/sync.sh --vendor
```

## Checklist after modifying skills

- [ ] `metadata.scope` and `metadata.auto_invoke` set on new or changed skills
- [ ] Ran `./skills/skill-sync/assets/sync.sh`
- [ ] Verified affected `AGENTS.md` files
