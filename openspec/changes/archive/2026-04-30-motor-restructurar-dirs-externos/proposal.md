## Why

Today `ai-specs/skills/` mixes local project skills with vendored dependencies and recipe-bundled skills. This forces projects to commit external code, pollutes the local source tree, and makes it impossible to distinguish what is project-owned versus what comes from an external component. We need a clean separation symmetric to how package managers isolate `node_modules/` from source code.

## What Changes

- **`ai-specs init`** creates `.recipe/` and `.deps/` at project root and adds both to `.gitignore`.
- **`vendor-skills.py`** clones `[[deps]]` skills into `.deps/{dep-id}/skills/` instead of `ai-specs/skills/`.
- **`recipe-materialize.py`** materializes recipe-bundled skills into `.recipe/{recipe-id}/skills/` instead of `ai-specs/skills/`.
- **`ai-specs/skills/`** becomes exclusively for local, committable project skills.
- **`sync-agent`** scans three skill sources with defined precedence:
  1. `ai-specs/skills/{id}/` ← local (highest precedence)
  2. `.recipe/{recipe-id}/skills/{id}/` ← bundled by recipe
  3. `.deps/{dep-id}/skills/{id}/` ← vendored from third parties
- **Overrides contract**: `.recipe/{recipe-id}/overrides/config.toml` and `overrides/templates/` are read at runtime by the bundled skill.
- **Regression tests** covering init, vendor, materialize, and sync-agent multi-source resolution.

## Capabilities

### New Capabilities
- `external-dirs-layout`: Directory structure, gitignore rules, and `init` behavior for `.recipe/` and `.deps/`.
- `skill-source-precedence`: Multi-source skill scanning and precedence rules for `sync-agent`.
- `recipe-overrides-runtime`: Runtime override loading for recipe-bundled skills from `.recipe/{recipe-id}/overrides/`.

### Modified Capabilities
- `recipe-sync-materialization`: Materialization destination paths change; recipe skills now target `.recipe/` and dep skills target `.deps/`.
- `recipe-conflict-resolution`: Conflict scope updates to account for the multi-source layout and the rule that local skills in `ai-specs/skills/` always take precedence over recipe or dep versions.

## Impact

- `lib/init.sh` — creates `.recipe/` and `.deps/`, updates `.gitignore` generation.
- `lib/_internal/vendor-skills.py` — changes clone destination to `.deps/{dep-id}/skills/`.
- `lib/_internal/recipe-materialize.py` — changes materialization targets to `.recipe/` and `.deps/`.
- `lib/sync-agent.sh` — expands skill scanning to three sources with precedence.
- `lib/_internal/gitignore-render.py` — adds `.recipe/` and `.deps/` to generated gitignore.
- `tests/` — new regression tests for the external-dirs flow.
- `sdd-cli-integration` (indirect) — the `refresh-bundled` preset behavior may need alignment in a follow-up change if bundled catalog skills are considered external.
