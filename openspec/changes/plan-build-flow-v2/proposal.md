# Proposal: Plan-Build-Flow v2 (Ambient)

## Intent

Make the plan/build change ceremony fully invisible: remove `/plan` and `/build` slash commands and rely on ambient skill auto-invocation so agents plan and build using their native flow while silently maintaining the OpenSpec artifact trail.

v2.1 adds a **change depth classifier** (full / standard / light), a **PR artifact gate** (no PR without committed planning files), and an explicit **pre-merge archive gate** aligned with `vcs-pr-flow`.

## Scope

### In Scope
- Remove `plan` and `build` commands from the `plan-build-flow` recipe.
- Rewrite bundled skill + brief/README for ambient triggers (no slash verbs).
- Bump recipe to 2.1.0 with depth classifier and PR/archive gates.
- Add AC11–AC13 tests for classifier and gates.

### Out of Scope
- recipe-evals harness (card #40).
- Changes to gentle-ai orchestrator itself.

## Capabilities

### Modified Capabilities
- `plan-build-flow`: skill-only ambient workflow with depth tiers and hard PR/archive gates.

## Approach

Drop commands from manifest, rewrite skill `auto_invoke` and brief fragments, promote classifier requirements to canonical spec, update tests.

## Success Criteria
- [x] Sync materializes skill only (no `/plan` or `/build` commands).
- [x] Brief/README remain vocabulary-clean and slash-command-free.
- [x] Classifier tiers and PR/archive gates documented in skill + brief.
- [ ] `./tests/run.sh` and `./tests/validate.sh` pass.
