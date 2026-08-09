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

## Assisted non-interactive configure

Use `ai-specs recipe configure <id> [path]` for an agent-assisted flow; this is
additive to (and does not replace) the interactive `configure-recipes` wizard.
Follow the phases in order:

1. **Inspect** — run `ai-specs recipe configure <id> [path] --inspect --json`.
   Confirm the recipe id from `recipe list`, read the schema fields and current
   config, and use the grounding signals (topology, MCP, and CLI dependencies).
2. **Recommend** — show proposed schema-valid `KEY=VALUE` changes, keys left
   unchanged, assumptions/questions (especially `monorepo-apps` versus
   `standalone` when no `.gitmodules` exists), and the planned sync/doctor
   verification. Do not mutate until the user gives explicit approval.
3. **Apply** — after approval, run `--set KEY=VALUE` (repeatable), optionally
   with `--dry-run`. The helper writes only `[recipes.<id>.config]`, preserves
   comments and unmentioned keys, and leaves every `overrides/` file untouched.
   Never put literal tokens, passwords, API keys, or other secrets in a
   command or report; use `${env:VAR}` references and redaction.
4. **Sync/verify** — add `--sync` after approval. The helper runs sync and
  doctor, forwards `--ignore-cli-version` only when explicitly requested, and
  reports partial failure without claiming the project is complete. This is
  the no-secret-literal rule: use `${env:VAR}` references and redaction.
5. **Report** — retain the JSON report fields `status`, `applied.changed`,
   `applied.unchanged`, `applied.preserved`, `preflight`, `sync`, `verify`,
   `assumptions`, `drift`, and `gaps`. Exit 0 is success/no-op, 1 is
   apply/sync failure or partial, 2 is usage, 3 is rejected input, and 4 is a
   preflight block. Codes 3 and 4 guarantee no manifest write.

The deterministic helper is suitable for `worktree-flow` topology grounding
without an `init.md`, `plan-build-flow` plain config, and
`trello-mcp-workflow` MCP/secrets/init guidance. `recipe init` remains
read-only and does not invoke sync.

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
