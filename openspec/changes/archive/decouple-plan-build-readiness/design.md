# Design: Decouple plan-build readiness from artifact store assumptions

## Technical Approach

Contract/wording change with invariance coverage; enforcement code unchanged. Verified from source: `plan-build-gate.sh` and `premerge_guardian.py` read only the filesystem planning tree (`openspec/changes/*/tasks.md`, tier minima, `verify-report.md`); neither consults `artifact_store_default` or any preflight. The conflation is documentary: brief/README present the store as "where planning artifacts live" while gates already require repository files. The change locks a two-layer contract (store = external-session persistence preference; readiness = file-backed canonical tree) in the delta spec (7 scenarios), recipe brief, skill, README, and docs mirror, and proves store-invariance with tests. No new config, no store-aware readiness, no `[sdd]` revival.

## Architecture Decisions

| Option | Tradeoff | Decision |
|---|---|---|
| Two-layer contract vs store-aware readiness vs drop `artifact_store_default` | Store-aware adds a runtime dependency and weakens fail-closed; dropping abandons the repo-owned delivery contract | Two-layer contract: store stays a persistence preference; readiness always file-backed |
| Leave hook/guardian untouched vs modify them | Enforcement is already store-blind; changing it risks the spec-tested normalized hook contract and topology resolver | No behavior change to `plan-build-gate.sh` / `premerge_guardian.py`; prove invariance in tests instead |
| Name `openspec/changes/<slug>/` in brief vs avoid forbidden vocab | `test_brief_and_readme_vocabulary_clean` forbids "openspec"/"sdd"/"spec-driven" in brief fragments and README outside the Delivery-contracts exemption (`_without_delivery_contracts_section`) | Brief + skill phrase it as "canonical change-folder tree"; README's exempt Delivery-contracts section names the path; SKILL.md names paths freely |
| Bump recipe 1.6.0 → 1.7.0 | Schema, enum, hook, and guardian are unchanged; bump forces CHANGELOG + pin-test churn (`test_readme_and_catalog_pin_recipe_1_6_0`) for wording only | Keep `1.6.0` |

## Data Flow

```
store preference (openspec|engram|both)
  └─> brief rule ──> AGENTS.md  (external session may mirror/persist)

readiness (unchanged, store-blind)
  plan-build-gate.sh    ──> openspec/changes/*/tasks.md        (fs only)
  premerge_guardian.py  ──> change folder + archive + verify   (fs only)
```

Engram MAY mirror artifacts; gates never consult it. A memory-only presence cannot satisfy any readiness check.

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `openspec/changes/decouple-plan-build-readiness/specs/plan-build-flow/spec.md` | Done (delta) | MODIFIED "Artifact store degradation and default" + 7 scenarios |
| `openspec/specs/plan-build-flow/spec.md` | Modify (via archive merge) | Delta lands in main spec at sdd-archive |
| `catalog/recipes/plan-build-flow/recipe.toml` | Modify | `help_text` + brief rule 6: persistence-preference wording + non-bypassable readiness invariant; keep single `{config.artifact_store_default}` placeholder; enum untouched |
| `catalog/recipes/plan-build-flow/skills/plan-build-flow/SKILL.md` | Modify | Section 6: separate persistence from readiness; Engram mirror never substitutes for files |
| `catalog/recipes/plan-build-flow/README.md` | Modify | Delivery-contracts section: store = preference; readiness always file-backed |
| `docs/recipes-catalog.md` | Modify | Config-table description + prose mirror |
| `tests/test_plan_build_flow_recipe.py` | Modify | Assert surface states the readiness invariant; vocabulary + placeholder assertions preserved |
| `tests/test_premerge_guardian.py` | Modify | Store-invariance: identical filesystem state + store contexts (`openspec\|engram\|both`) yield identical verdicts; missing files block under any store |
| `tests/test_plan_build_gate_hook.py` | Modify | Store-invariance: hook subprocess with store env set yields identical exit codes |
| `tests/evals/scenarios/plan-build-flow/ac_delivery_contract_artifact_store/` | Modify | `required_content`/transcript also require readiness-invariant phrasing |
| `catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh`, `lib/_internal/premerge_guardian.py` | No change | Enforcement is store-blind today; invariance tests lock that |

## Interfaces / Contracts

Normative invariant (delta): the preflight-resolved store MUST NOT be consulted for readiness and MUST NOT alter classifier, PR/archive gate, staged verify gate, or pre-merge guardian decisions. Brief rule shape: keep exactly one `{config.artifact_store_default}` occurrence (tests assert `count == 1`) and append the file-backed readiness sentence without forbidden vocabulary. No new config fields, CLI flags, or runtime interfaces.

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | Guardian verdicts invariant across store contexts; missing files block under `engram` | Parametrize `test_premerge_guardian.py` over STORE_ENUM with env/context fixture; reuse existing blocker assertions |
| Unit | Hook exit codes invariant across store contexts | Parametrize `test_plan_build_gate_hook.py` subprocess runs with store env set; assert same exit as baseline |
| Unit | Recipe surface wording (brief, skill Section 6, README delivery contracts, catalog) | Extend `test_plan_build_flow_recipe.py`; keep vocabulary/placeholder/exemption assertions green |
| E2E | Live brief states store preference AND readiness invariant | Extend eval scenario `ac_delivery_contract_artifact_store` |

Runner: `./tests/run.sh` for RED/GREEN; `./tests/validate.sh` final.

## Threat Matrix

Not applicable — no routing, shell-command, subprocess, VCS/PR automation, executable-file classification, or process-integration behavior changes. Rows: Documentation-like paths — N/A (no classification boundary touched); Git repository selection — N/A (`git -C` usage unchanged); Commit state — N/A; Push state — N/A; PR commands — N/A (PR gate is skill-level wording). Hook and guardian keep their spec-tested contracts.

## Migration / Rollout

No migration required; enforcement behavior is unchanged, and installed projects need no hook refresh (`ai-specs sync` only re-renders brief/README wording). Rollback: revert wording edits and remove added tests.

## Open Questions

- [ ] None blocking. Residual risk: an external preflight runtime may still treat `engram` as memory-only; mitigated by the rendered brief's invariant plus live-eval coverage.
