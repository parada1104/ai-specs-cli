# Tasks: plan-build-depth-artifacts-verify

Depth: full

## Tracker

- card_id: `lxv2WQ5g`
- url: https://trello.com/c/lxv2WQ5g

## Notes

- Planning only until authorization. No production code in the plan commit.
- Sibling #59 (`plan-build-depth-adversarial`) owns adversarial classifier UX.
  **Do not** redesign it here. **Apply serializes after #59 merges.**
- Working hypothesis from explore/proposal/design: A1 minima, B1 explore
  criteria, C1 guardian-extended verify gate — subject to auth answers on
  open questions.

## Phase 0 — Auth gate

- [ ] **0.1** Human authorizes this plan (or amends open questions in
      proposal.md / design.md). **Acceptance:** Explicit go-ahead recorded;
      Depth remains `full` unless auth downgrades.

## Phase 1 — Guardian minima + verify (TDD)

- [ ] **1.1** RED: extend `tests/test_premerge_guardian.py` for Light requiring
      `proposal.md`; Standard requiring `proposal.md`; Light missing verify still
      OK; Standard/Full missing verify evidence blocked; Full PASS report OK.
      **Acceptance:** New tests fail against current guardian.
- [ ] **1.2** GREEN: update `lib/_internal/premerge_guardian.py` minima + staged
      verify checks per design §3 and §5. **Acceptance:** Focused guardian tests
      pass.
- [ ] **1.3** TRIANGULATE: edge cases (infer tier from Depth line; explicit
      `--tier`; failing verify-report verdict). **Acceptance:** Cases green.

## Phase 2 — Skill / recipe surface

- [ ] **2.1** Update `catalog/recipes/plan-build-flow/skills/plan-build-flow/SKILL.md`
      §2 minima table, Standard explore criteria + skip line, §7 verify gate +
      guardian blocker list. **Acceptance:** Matches delta spec; no #59 conflict
      language beyond a one-line deferral.
- [ ] **2.2** Update README + `recipe.toml` brief fragments (vocabulary-clean).
      **Acceptance:** `test_plan_build_flow_recipe` / vocabulary tests pass or
      are updated RED→GREEN.
- [ ] **2.3** RED/GREEN recipe/skill assertion tests for new markers (proposal
      for Light, explore criteria, verify advisory/enforcement/required).
      **Acceptance:** Focused recipe tests pass.

## Phase 3 — Spec promotion + docs

- [ ] **3.1** Promote
      `openspec/changes/plan-build-depth-artifacts-verify/specs/plan-build-flow/spec.md`
      into canonical `openspec/specs/plan-build-flow/spec.md` (merge MODIFIED /
      ADDED). **Acceptance:** Canonical spec reflects minima + explore + verify.
- [ ] **3.2** README grandfathering note for in-flight Light plans.
      **Acceptance:** One short migration paragraph present.

## Phase 4 — Verify this change

- [ ] **4.1** Run focused guardian + plan-build-flow recipe tests via
      `./tests/run.sh` patterns / unittest modules. **Acceptance:** PASS.
- [ ] **4.2** Run `./tests/validate.sh`. **Acceptance:** PASS.
- [ ] **4.3** Write `verify-report.md` (Full required). **Acceptance:** Overall
      PASS mapped to success criteria.

## Phase 5 — Ship hygiene

- [ ] **5.1** Confirm #59 merged (or rebase onto it) before opening apply PR.
      **Acceptance:** No divergent skill §2 edits vs #59.
- [ ] **5.2** PR only after planning files committed; archive-tail before merge.
      **Acceptance:** Guardian OK on archive.

## Non-goals (task level)

- Adversarial depth conflict UX (#59).
- Tier-aware `plan-build-gate.sh`.
- New recipe schema / materializer fields (unless auth reopens).
