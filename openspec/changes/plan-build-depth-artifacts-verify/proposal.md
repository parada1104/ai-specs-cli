# Proposal: plan-build-depth-artifacts-verify

## Tracker

- card_id: `lxv2WQ5g`
- url: https://trello.com/c/lxv2WQ5g
- title: [Follow-up] plan-build-flow: ajustar artifacts mínimos por depth + gate de verify escalonado

## Why

- **Problem**: Depth minima under-specify Light context, leave Standard explore
  optional without criteria, and allow archive/merge without staged verification
  evidence — archive gate alone does not prove the change was verified.
- **Origin**: Follow-up from `plan-build-delivery-contracts` (#58); sibling of
  adversarial classifier (#59). Apply must wait for #59 to merge (same recipe
  surface); this proposal does not redesign the adversarial path.
- **Why now**: Card #60; planning-only until authorization.

## What Changes

| Area | Impact | Description |
|---|---|---|
| `catalog/recipes/plan-build-flow/skills/plan-build-flow/SKILL.md` | Modified | Raise Light/Standard minima; add Standard explore criteria; document staged verify gate |
| `catalog/recipes/plan-build-flow/README.md` + brief fragments | Modified | Mirror minima + verify staging in vocabulary-clean language |
| `openspec/specs/plan-build-flow/spec.md` | Modified (via delta) | Normative minima, explore criteria, verify gate modes |
| `lib/_internal/premerge_guardian.py` | Modified | Enforce new tier minima; depth-staged verify evidence before merge |
| `tests/test_premerge_guardian.py` (+ skill/recipe tests as needed) | Modified | RED/GREEN for minima + verify modes |
| Eval scenarios (optional if cheap) | Additive | Cover Light proposal requirement / Standard explore skip note |

## Capabilities

### Modified

- `plan-build-flow` — planning artifact minima by depth; Standard explore
  enforcement criteria; staged verify gate (advisory / enforcement / required)
  as counterweight to the existing pre-merge archive gate.

### Unchanged (explicit)

- Adversarial depth conflict handling → **#59**.
- Pre-tool-use `plan-build-gate.sh` semantics (any active `tasks.md`).
- Recipe schema / materializer (no new config fields unless design proves need).

## Approach (working hypothesis — validate at auth)

From explore **A1 / B1 / C1**:

1. **Light minimum** → `proposal.md` + `tasks.md` (short proposal allowed).
2. **Standard minimum** → `proposal.md` + `tasks.md` + at least one
   `specs/**/*.md`. **Explore** required when criteria match; when skipped,
   record `Explore: skipped — <reason>` in `tasks.md`.
3. **Full minimum** → unchanged structurally (`tasks` + `proposal|design` +
   specs); explore remains first in the Full chain.
4. **Verify gate** (before archive-tail / merge via guardian):
   - Light: **advisory** (warn; do not block).
   - Standard: **enforcement** (block without verify evidence).
   - Full: **required** (`verify-report.md` with PASS / ready_for_archive).

Guardian remains the machine check; skill carries agent-facing procedure.

## Non-Goals

- Redesigning the adversarial classifier or depth-conflict UX (#59).
- Making the pre-tool-use edit gate tier-aware.
- New slash commands or SDD vocabulary in user-facing briefs.
- Changing classic `openspec/config.yaml` decision_matrix levels.
- Blocking Light merges solely for missing verify reports.

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Light ceremony creep | Med | Cap proposal size guidance; keep Light signals tight |
| Explore criteria still argued | Med | Binary checklist + mandatory skip line in tasks.md |
| #59 / #60 apply conflict | High if parallel apply | Serialize Apply: #59 then #60 |
| Verify evidence shape ambiguity | Med | Spec accepts validate PASS note **or** verify-report for Standard; Full requires report |
| Guardian complexity | Low–Med | Prefer extending helper; split only if tests explode |

## Rollback

Revert skill/README/brief, reverse guardian + tests, restore prior canonical
spec. Opt-in recipe; consumers re-sync after revert.

## Success Criteria

- [ ] Skill §2 and guardian agree on Light/Standard/Full minima (incl. Light
      `proposal.md`).
- [ ] Standard explore criteria are explicit; skip path records a one-line reason.
- [ ] Verify modes: Light advisory, Standard enforcement, Full required —
      documented and tested in guardian (and skill text).
- [ ] No adversarial-classifier redesign; Apply notes #59 serialization.
- [ ] `./tests/run.sh` focused guardian/recipe tests + `./tests/validate.sh` PASS.
- [ ] Vocabulary hygiene preserved (no SDD/OpenSpec/slash plan|build leaks in
      brief/README).

## Open Questions (for auth)

1. Confirm **A1** Light = proposal + tasks (vs softer Why-in-tasks).
2. Confirm Standard always requires **proposal.md** (A1) vs explore-only (A2).
3. Confirm verify **enforcement** blocks archive/merge (C1) vs PR-creation only.
4. Accept Standard evidence as either `verify-report.md` **or** recorded
   validate PASS inside a short report stub?
5. Grandfathering: new minima apply only to changes planned after ship — OK?
