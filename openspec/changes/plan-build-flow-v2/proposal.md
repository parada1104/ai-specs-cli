# Proposal: Plan-Build-Flow v2 (Ambient)

## Intent

Make the plan/build change ceremony fully invisible: remove `/plan` and `/build` slash commands and rely on ambient skill auto-invocation so agents plan and build using their native flow while silently maintaining the OpenSpec artifact trail.

## Scope

### In Scope
- Remove `plan` and `build` commands from the `plan-build-flow` recipe.
- Rewrite bundled skill + brief/README for ambient triggers (no slash verbs).
- Bump recipe to 2.0.0 with updated materialization tests.

### Out of Scope
- recipe-evals harness (card #40).
- Changes to gentle-ai orchestrator itself.

## Capabilities

### Modified Capabilities
- `plan-build-flow`: skill-only ambient workflow; same internal phase mapping, zero user-facing slash commands.

## Approach

Drop commands from manifest, rewrite skill auto_invoke and brief fragments, update tests to assert zero commands and ambient brief wording.

## Success Criteria
- [x] Sync materializes skill only (no `/plan` or `/build` commands).
- [x] Brief/README remain vocabulary-clean and slash-command-free.
- [x] `./tests/run.sh` and `./tests/validate.sh` pass.
