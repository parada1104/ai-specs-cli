# Proposal: Decouple plan-build readiness from artifact store assumptions

## Intent

`plan-build-flow` conflates `artifact_store_default` (a persistence preference rendered into the brief for external preflight) with plan-build readiness (what gates enforce). An external runtime may read `engram` as memory-only, while classifier, tier-minima, PR/archive, and verify guarantees require repository files. This change locks a two-layer contract without weakening guarantees.

## Scope

### In Scope
- Declare `openspec/changes/<slug>/` the fixed, file-backed readiness source for plan-build enforcement.
- Redefine `artifact_store_default` as external-session persistence preference; Engram may mirror, never replace.
- Update spec, skill, recipe brief rule, README, and docs mirror wording.
- Add cross-store invariance tests proving gate/guardian decisions are identical across `openspec|engram|both`.

### Out of Scope
- Store-aware readiness or new readiness config fields.
- Removing `artifact_store_default` or the external preflight boundary.
- Reviving the legacy `sdd-artifact-store` `[sdd].artifact_store` enum contract.
- Changing `plan-build-gate.sh` / `premerge_guardian.py` behavior.

## Capabilities

### New Capabilities
None.

### Modified Capabilities
- `plan-build-flow`: refine "Artifact store degradation and default" — configured store is persistence preference only; readiness always proven by canonical `openspec/changes/<slug>/` artifacts (`tasks.md`, tier minima, committed planning files, `verify-report.md`).

## Approach

Explicit two-layer contract: keep `artifact_store_default` and the preflight boundary, but stop presenting the value as machine-enforced readiness. Source-of-truth wording in spec/README/recipe brief; skill separates persistence from readiness; tests assert invariance. No runtime dependency added; enforcement code unchanged.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `openspec/specs/plan-build-flow/spec.md` | Modified | Artifact-store requirement states readiness invariant |
| `catalog/recipes/plan-build-flow/recipe.toml` | Modified | Brief rule + config help wording |
| `catalog/recipes/plan-build-flow/skills/plan-build-flow/SKILL.md` | Modified | Persistence vs readiness separation |
| `catalog/recipes/plan-build-flow/README.md`, `docs/recipes-catalog.md` | Modified | Delivery-contract wording, mirror consistency |
| `tests/test_plan_build_flow_recipe.py`, `tests/test_plan_build_gate_hook.py`, `tests/test_premerge_guardian.py`, `tests/evals/scenarios/plan-build-flow/ac_delivery_contract_artifact_store/` | Modified | Cross-store invariance; no-bypass proof |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| External runtime still treats `engram` as memory-only | Med | Brief states non-bypassable readiness invariant |
| Vocabulary-scan exemptions broken | Med | Preserve narrow exemptions and placeholder assertions |
| Second source of truth via new config | Low | No new fields or external queries |

## Rollback Plan

Revert wording in spec/recipe/README/docs and remove added tests via a revert commit. Hook and guardian never change, so enforcement is preserved throughout.

## Dependencies

- Card #72 (In Progress). No external runtime contract; preflight stays outside the recipe dependency graph.

## Success Criteria

- [ ] Gate and guardian decisions identical across `openspec|engram|both` (invariance tests green).
- [ ] Spec, brief, README, docs state readiness is always file-backed.
- [ ] Legacy `sdd-artifact-store` surface untouched; vocabulary exemptions preserved.
- [ ] `./tests/validate.sh` passes.

## Tracker

- **card_id**: 6a7cadebca8399185c842b27
- **shortLink**: 8MhzkTMn
- **url**: https://trello.com/c/8MhzkTMn
- **list**: In Progress
