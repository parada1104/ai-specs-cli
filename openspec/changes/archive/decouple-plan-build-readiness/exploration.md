## Exploration: Decouple plan-build readiness from artifact store assumptions

### Current State
`plan-build-flow` has two overlapping contracts that are not explicitly separated. The recipe declares `artifact_store_default` and renders it into the project brief as the place where planning artifacts live; an external session preflight may consume that value. In contrast, the repository-owned enforcement path is filesystem-based and independent of that configuration: `plan-build-gate.sh` looks for an active `openspec/changes/*/tasks.md`, while `premerge_guardian.py` reads the change folder, depth line, tier minima, and dedicated verification report. The canonical plan-build specification also falls back to files when no preflight store is resolved, but does not define the readiness source when a project declares `engram` or `both`.

The result is an ambiguity rather than a missing gate: an external preflight can interpret `artifact_store_default = "engram"` as memory-only persistence while the hard edit, artifact, archive, and verify guarantees still require repository files. The external preflight implementation is not present in this repository and the existing delivery-contract change explicitly keeps it outside the recipe dependency graph. Focused baseline evidence is green: the plan-build recipe suite ran 25 tests successfully and the gate suite ran 28 tests successfully. `./tests/validate.sh` was attempted but exceeded five minutes while running the broader suite; no failure was observed before timeout. The worktree remains clean.

### Affected Areas
- `catalog/recipes/plan-build-flow/recipe.toml` — currently describes `artifact_store_default` as the planning-artifact location and passes it to external consumers through a brief rule.
- `catalog/recipes/plan-build-flow/skills/plan-build-flow/SKILL.md` — owns classifier/depth planning, artifact minima, authorization, PR/archive gates, and verify sequencing; it needs an explicit separation between persistence preference and readiness evidence.
- `catalog/recipes/plan-build-flow/README.md` — documents the public delivery-contract wording and currently implies that the configured store is the artifact location without naming the independent readiness source.
- `docs/recipes-catalog.md` — mirrors the recipe configuration and must remain consistent with the source README and manifest.
- `openspec/specs/plan-build-flow/spec.md` — contains the normative classifier, depth minima, staged verify, hook, guardian, and artifact-store requirements; the artifact-store requirement is the key ambiguity to refine.
- `catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh` — hard enforcement reads the repository planning tree and must either remain unchanged as the readiness authority or receive a narrowly defined, independently verifiable readiness contract.
- `lib/_internal/premerge_guardian.py` — hard archive/merge enforcement reads filesystem artifacts and must continue enforcing tier minima and verify evidence regardless of external store selection.
- `tests/test_plan_build_flow_recipe.py`, `tests/test_plan_build_gate_hook.py`, and `tests/test_premerge_guardian.py` — existing seams cover brief/config materialization, active-plan gating, tier minima, and staged verification; add cross-store invariance assertions here.
- `tests/evals/scenarios/plan-build-flow/ac_delivery_contract_artifact_store/` — currently verifies brief consumption for `both`, but does not prove that an external store selection cannot bypass filesystem readiness guarantees.

### Approaches
1. **Explicit two-layer contract (recommended)** — define `artifact_store_default` as the external session's persistence/working-memory preference, while declaring the repository filesystem projection (`openspec/changes/<slug>/`) as the fixed readiness source for plan/build enforcement. Require every configured store mode, including `engram`, to preserve the file artifacts needed by classifier, tier minima, PR/archive, and verify gates; Engram may remain an additional mirror. Update recipe/skill/spec/docs and add tests proving gate and guardian decisions are invariant across store values.
   - Pros: preserves all existing hard guarantees; requires no external runtime dependency; matches the current hook and guardian implementation; makes the external preflight boundary explicit and testable.
   - Cons: changes ambiguous public wording and clarifies that `engram` is not a memory-only escape from file-backed readiness; external consumers must honor the rendered instruction.
   - Effort: Medium

2. **Make readiness store-aware** — teach the hook, guardian, and planning flow to inspect the selected artifact store, or query an external preflight/runtime for readiness.
   - Pros: the configured store would directly control readiness evaluation.
   - Cons: requires a new cross-runtime protocol or memory integration; archive and merge still need durable files; failure/timeout semantics could weaken hard gates; violates the existing orchestrator-agnostic boundary and substantially expands the change.
   - Effort: High

3. **Remove the repository artifact-store declaration** — delete `artifact_store_default` and let external preflight choose its own store, retaining the current file gates.
   - Pros: eliminates the conflicting declaration and minimizes runtime surface.
   - Cons: abandons the repository-owned delivery contract introduced by the prior change; external defaults can drift from project policy; does not solve the conceptual boundary for future store declarations.
   - Effort: Medium

### Recommendation
Use the explicit two-layer contract. Keep `artifact_store_default` and the external preflight boundary, but stop presenting that value as the machine-enforced readiness source. State that plan-build readiness is always proven by the canonical repository planning projection and that the configured store may add operational persistence but cannot replace `tasks.md`, tier minima, committed planning artifacts, or dedicated verify evidence. Leave `plan-build-gate.sh` and `premerge_guardian.py` behavior unchanged unless a narrowly scoped regression test demonstrates a real implementation gap; the likely fix is source-of-truth wording, normative spec clarification, and cross-store invariance coverage.

### Risks
- A wording-only change will not protect against an external runtime that still interprets `engram` as memory-only; the generated brief must state the non-bypassable readiness invariant clearly, and live eval coverage should check the response plus produced files where providers are available.
- Adding a new readiness configuration field or external query would create a second source of truth and could weaken fail-closed behavior when the preflight is unavailable; avoid it unless a concrete runtime protocol is supplied.
- The prior recipe tests intentionally exclude the config table and delivery-contract README section from vocabulary scans; changes to those sections must preserve the narrow exemptions and exact placeholder/materialization assertions.
- `openspec/specs/sdd-artifact-store/spec.md` contains an older `[sdd].artifact_store` contract with different enum names. The current plan-build spec and archived delivery-contract decisions explicitly reject reintroducing that surface; the proposal should identify the newer plan-build contract as authoritative and avoid reviving the legacy schema.
- Full validation remains unconfirmed in this exploration because `./tests/validate.sh` exceeded the five-minute execution window; focused plan-build and hook evidence is confirmed.

### Ready for Proposal
Yes — the repository-owned readiness source, external preflight boundary, and preservation invariants are sufficiently identified. The proposal should lock the two-layer contract, enumerate the exact wording/spec changes, and define cross-store tests before implementation authorization.
