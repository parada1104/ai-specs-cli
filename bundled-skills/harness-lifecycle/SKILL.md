---
name: harness-lifecycle
description: >
  Operate the ai-specs project lifecycle: init, configure-recipes, sync,
  sync-agent, refresh-bundled, doctor, upgrade, and hub. Trigger: When
  bootstrapping a project, syncing the harness after manifest changes,
  diagnosing health, upgrading the CLI, or using the interactive hub.
license: MIT
metadata:
  author: ai-specs
  version: "1.0"
  scope: [root]
  auto_invoke:
    - "Initialize or bootstrap ai-specs in a project"
    - "Sync the harness after manifest or recipe changes"
    - "Run doctor or diagnose ai-specs project health"
    - "Upgrade the ai-specs CLI installation"
    - "Open the ai-specs hub or interactive menu"
---

# Harness lifecycle

Use this skill for **project lifecycle** ops on the public `ai-specs` CLI.
For catalog recipes see `harness-recipes`. For local skills and external deps
see `harness-skills-deps`.

## Resolve the CLI (footnote)

Prefer PATH, then the install home:

```bash
command -v ai-specs >/dev/null 2>&1 && ai-specs "$@" \
  || "${AI_SPECS_HOME:-$HOME/.ai-specs}/bin/ai-specs" "$@"
```

Do **not** invent a per-project `ai-specs/bin/ai-specs` shim.

## Order of ops (happy path)

1. `ai-specs init [path]` — scaffold `ai-specs/`, AGENTS.md, gitignore (idempotent).
2. Edit `ai-specs/ai-specs.toml` (agents, mcp, recipes, deps) as needed.
3. `ai-specs configure-recipes [path]` — fill recipe config, check CLI deps,
   optional `.envrc.example` (also covered in `harness-recipes`).
4. `ai-specs sync [path]` — vendor/refresh + regen AGENTS.md + fan-out agents.
5. `ai-specs doctor [path]` — read-only health check after sync or when something
   looks wrong.

Re-run **sync** after any meaningful manifest / recipe / skill change. Do not
hand-edit generated agent instruction files when sync owns them.

## Commands

| Command | When |
|---|---|
| `ai-specs init` | First time (or offer-init from hub). Safe to re-run. |
| `ai-specs configure-recipes` | After enabling recipes that need config/env. |
| `ai-specs sync` | After toml/recipe/skill/dep changes; primary reconcile. |
| `ai-specs sync-agent` | Fan-out only (skip full vendor) when you only need agent targets refreshed. |
| `ai-specs refresh-bundled` | Force bundled skill/command refresh from the installed CLI. |
| `ai-specs doctor` | Diagnose without writing. |
| `ai-specs upgrade` | Upgrade the **global** CLI install (not a project op). |
| `ai-specs hub` / bare `ai-specs` | Interactive status + menu (TTY). |

`ai-specs version` / `ai-specs help` are footnotes only.

`ai-specs rules-audit` exists for legacy rules inventory; not part of this
literacy skill's primary flows — use when migrating old Cursor rules.

## refresh-bundled and `.new` sidecars

Bundled skills (including this one) ship via `refresh-bundled` on init/sync:

- First install copies into `ai-specs/skills/<name>/`.
- If you edit a bundled skill and upstream changes, CLI may write
  `SKILL.md.new` — review and resolve; do not ignore forever.
- Deleting a bundled skill can opt it out permanently (lock records opt-out).
  Prefer editing with intent over silent delete unless you mean to opt out.

## Pitfalls

- **Edit toml, forget sync** — agents keep stale skills/MCPs/brief.
- **Hand-edit AGENTS.md** that sync regenerates — changes get overwritten
  unless the file is marked user-managed (`<!-- ai-specs:runtime-brief -->`).
- **upgrade vs sync** — `upgrade` updates `~/.ai-specs`; `sync` updates the
  project from that install.
- **Wrong cwd** — pass `[path]` when not at the project root.
- **Hub needs TTY + deps** — missing `rich`/`questionary` → exit 3; non-TTY
  gets status text only.

## Checklist

- [ ] Manifest reflects the intended agents/recipes/deps
- [ ] Ran `ai-specs sync` after edits
- [ ] Ran `ai-specs doctor` if something failed or looks inconsistent
- [ ] Did not invent project-local CLI shims
