## Phase A — Infrastructure & Init

- [x] 1.1 Update `lib/init.sh` to create `.recipe/` and `.deps/` at project root during `ai-specs init`
- [x] 1.2 Update `lib/_internal/gitignore-render.py` to emit `.recipe/`, `.deps/`, and `.recipe/*/overrides/` into the root `.gitignore`
- [x] 1.3 Verify `init` is idempotent: re-running `ai-specs init` does not duplicate gitignore entries or fail when directories already exist

## Phase B — Vendor & Recipe Materialization

- [x] 2.1 Update `lib/_internal/vendor-skills.py` to clone `[[deps]]` skills into `.deps/{dep-id}/skills/{skill-id}/` instead of `ai-specs/skills/`
- [x] 2.2 Update `lib/_internal/recipe-materialize.py` to materialize bundled recipe skills into `.recipe/{recipe-id}/skills/{skill-id}/`
- [x] 2.3 Update `lib/_internal/recipe-materialize.py` to materialize dependency skills (from recipe `[[deps]]`) into `.deps/{dep-id}/skills/{skill-id}/`
- [x] 2.4 Ensure `ai-specs/skills/` is never written to by vendor or recipe materialization logic
- [x] 2.5 Verify re-sync idempotency: unchanged recipes/deps produce no modifications in `.recipe/`, `.deps/`, or `ai-specs/`
- [x] 2.6 **(Cleanup)** Add orphan directory removal: before materializing, compare manifest `[[deps]]` and enabled recipes against existing `.deps/` and `.recipe/` directories; delete directories that are no longer referenced

## Phase C — Sync-Agent Multi-Source Resolution

- [x] 3.1 **(Refactor)** Create `lib/_internal/skill-resolution.py` with `collect_skills(project_root)` that scans `ai-specs/skills/`, `.recipe/*/skills/`, and `.deps/*/skills/` and returns a resolved dict `{skill_id: (source_type, abs_path)}`
- [x] 3.2 Enforce precedence in `skill-resolution.py`: local (`ai-specs/skills/`) > recipe (`.recipe/{recipe-id}/skills/`) > dep (`.deps/{dep-id}/skills/`)
- [x] 3.3 Emit a warning when the same skill ID appears in multiple recipes or multiple deps (first-seen wins)
- [x] 3.4 Fail with an explicit error when a required skill ID is missing from all three sources
- [x] 3.5 Update `lib/sync-agent.sh` to import/invoke `skill-resolution.py` and use resolved skill paths for AGENTS.md rendering and per-agent config fan-out
- [x] 3.6 Update `lib/_internal/agents-md-render.py` to consume the resolved dict from `skill-resolution.py` instead of scanning directories itself
- [x] 3.7 Preserve existing recipe-recipe primitive conflict behavior (skills, commands, MCP presets) as a hard error before precedence resolution

## Phase D — Runtime Overrides for Recipe Skills

- [x] 4.1 Add override config loading in `skill-resolution.py` (or a dedicated override helper): merge `.recipe/{recipe-id}/overrides/config.toml` on top of bundled defaults when resolving recipe skills
- [x] 4.2 Add override template loading: when rendering templates, prefer identically-named files from `.recipe/{recipe-id}/overrides/templates/` over bundled versions
- [x] 4.3 Ensure overrides are scoped to their parent recipe and do not leak into other recipes or deps
- [x] 4.4 Handle missing override files gracefully (no error, no warning)

## Phase E — Testing

- [x] 5.1 Add test for `ai-specs init` creating `.recipe/` and `.deps/` directories
- [x] 5.2 Add test for `.gitignore` containing `.recipe/`, `.deps/`, and `.recipe/*/overrides/`
- [x] 5.3 Add test for `vendor-skills.py` writing skills to `.deps/{dep-id}/skills/`
- [x] 5.4 Add test for `recipe-materialize.py` writing bundled skills to `.recipe/{recipe-id}/skills/` and dep skills to `.deps/{dep-id}/skills/`
- [x] 5.5 Add test for sync-agent local > recipe > dep precedence resolution
- [x] 5.6 Add test for first-seen warning when the same skill exists in multiple recipes or deps
- [x] 5.7 Add test for missing-skill error when a skill is absent from all sources
- [x] 5.8 Add test for recipe-recipe primitive conflict still failing hard
- [x] 5.9 Add test for local skill silently overriding recipe/dep skills without error or warning
- [x] 5.10 Add test for runtime override config and template loading
- [x] 5.11 Add test for override isolation between recipes
- [x] 5.12 Add test for full re-sync idempotency with external directories
- [x] 5.13 Add test for orphan directory cleanup (disabled recipes / removed deps)
- [x] 5.14 Run `./tests/run.sh` and `./tests/validate.sh`; fix any regressions in existing tests

## Phase F — Documentation

- [x] 6.1 Update user-facing docs (`README.md` or `docs/external-dirs.md`) explaining the 3-tier skill layout and how to add local vs recipe vs dep skills
