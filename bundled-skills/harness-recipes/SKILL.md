---
name: harness-recipes
description: >
  Install and configure ai-specs catalog recipes: list, add, init, and
  configure-recipes, then sync. Trigger: When enabling a recipe, inspecting
  recipe setup, configuring recipe fields or env, or choosing catalog
  capabilities for a project.
license: MIT
metadata:
  author: ai-specs
  version: "1.0"
  scope: [root]
  auto_invoke:
    - "Add or install an ai-specs recipe"
    - "List available or installed recipes"
    - "Configure recipe settings or recipe env"
    - "Run recipe init or read a recipe setup brief"
---

# Harness recipes

Operate **catalog recipes** through the public CLI. Lifecycle sync/doctor live
in `harness-lifecycle`. Skills/deps live in `harness-skills-deps`.

## Canonical install sequence

```bash
ai-specs recipe list [path]           # see catalog + installed
ai-specs recipe add <id> [path]       # declare in ai-specs.toml (+ placeholders)
ai-specs recipe init <id> [path]      # read-only setup brief (optional but useful)
ai-specs configure-recipes [path]     # fill config / CLI deps / .envrc.example
ai-specs sync [path]                  # materialize skills, docs, brief fragments
```

`recipe add` alone does **not** fully materialize assets — **sync** does.
`recipe init` is read-only guidance; it does not replace configure + sync.

## Commands

| Command | Role |
|---|---|
| `ai-specs recipe list` | Available catalog recipes and what the project has enabled. |
| `ai-specs recipe add <id>` | Add recipe id to the manifest (declaration). |
| `ai-specs recipe init <id>` | Print init brief for humans/agents (read-only). |
| `ai-specs configure-recipes` | Interactive/config pass for recipe fields and deps. |

After any add/config change: `ai-specs sync` (see `harness-lifecycle`).

## Mental model

- Recipes declare **capabilities**, skills, docs, optional brief fragments, and
  config schema.
- Projects **bind** providers for capabilities in the manifest (see project
  `docs/capabilities.md` when present).
- Conflicts / tags / CLI-bound catalog versions are enforced by sync — read
  doctor output if materialize fails.

## Pitfalls

- **add without sync** — toml mentions the recipe but skills/docs are missing.
- **Skipping configure-recipes** — MCP/env/config placeholders stay empty;
  runtime fails later.
- **Wrong recipe id** — use `recipe list`; do not invent ids.
- **Expecting recipe add to vendor git deps** — external skills use
  `ai-specs skills add` (`harness-skills-deps`), not `recipe add`.

## Checklist

- [ ] Confirmed id via `ai-specs recipe list`
- [ ] `ai-specs recipe add <id>`
- [ ] Read `ai-specs recipe init <id>` when unsure about setup
- [ ] `ai-specs configure-recipes` for required fields/env
- [ ] `ai-specs sync` and verify materialized paths / doctor
