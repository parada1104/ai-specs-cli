# Proposal: plan-build-depth-artifacts-verify

## Tracker

- card_id: `lxv2WQ5g`
- url: https://trello.com/c/lxv2WQ5g
- title: [Follow-up] plan-build-flow: ajustar artifacts mínimos por depth + gate de verify escalonado

## Status

**Authorization-ready.** All open questions are closed by explicit human
decisions (D8–D13 in `design.md` §2). The only remaining prerequisite is
operational: rebase this branch onto #59's landed merge SHA before apply
(`tasks.md` task 0.2).

## Why

- **Problem**: Depth minima under-specify Light context, leave Standard explore
  optional without criteria, and allow archive/merge without staged verification
  evidence — the archive gate alone does not prove the change was verified.
- **Origin**: Follow-up from `plan-build-delivery-contracts` (#58); sibling of the
  adversarial classifier (#59). #59 is verified **PASS** and lands as
  `plan-build-flow` `1.5.0`; #60 applies on top of it and does not redesign the
  adversarial path.
- **Why now**: Card #60 is authorized in scope; apply waits only on #59 landing.

## Relationship to #59 (final contract)

#59 ships, as `plan-build-flow` `1.5.0`:

- the signal / explicit-request / decided classifier computation,
- the depth-conflict ask before the planning chain,
- the four annotation labels (`Requested depth`, `Signal depth`, `Decided depth`,
  `Decision source`) with the standalone-`Depth:`-line contract,
- "higher decided tier completes its chain",
- exactly seven brief `workflow_rules` with rule 1 rewritten and rule 7 still the
  submodule-topology rule.

**#60 MUST preserve all of it.** The canonical classifier requirement and the
annotation contract are never overwritten: #60 touches only the three chain
bullets, one Light scenario line, and adds a pointer sentence (delta spec header
and `design.md` §11.3 give the line-level procedure and verification commands).

#60 owns, and only owns: artifact minima per depth, Standard/Full explore
guidance, the staged verify gate, `premerge_guardian.py` plus its tests, and the
matching docs/spec fragments.

## What Changes

| Area | Impact | Description |
|---|---|---|
| `catalog/recipes/plan-build-flow/skills/plan-build-flow/SKILL.md` | Modified | Raise Light/Standard minima; add Standard explore criteria; document the two-point staged verify gate |
| `catalog/recipes/plan-build-flow/README.md` | Modified | Mirror minima, verify staging, grandfathering note; version example `1.6.0` |
| `catalog/recipes/plan-build-flow/recipe.toml` | Modified | `1.5.0` → `1.6.0`; extend brief rules 3 and 5 only (rules 1 and 7 untouched) |
| `docs/recipes-catalog.md` | Modified | Enable example → `1.6.0` |
| `CHANGELOG.md` | Modified | New `[Unreleased]` entry `1.5.0` → `1.6.0`; #59's entry untouched |
| `openspec/specs/plan-build-flow/spec.md` | Modified (via delta) | Depth artifact minima, explore criteria, staged verify gate, reconciled PR gate + guardian blockers |
| `lib/_internal/premerge_guardian.py` | Modified | Tier minima, verify-evidence helper, `--stage pre-archive\|pre-merge` |
| `tests/test_premerge_guardian.py` | Modified | RED/GREEN for minima + both verify stages |
| `tests/test_plan_build_flow_recipe.py` | Modified | Brief rules 3/5 pins, version pins `1.6.0`, new skill markers |

## Capabilities

### Modified

- `plan-build-flow` — planning artifact minima by depth; Standard/Full explore
  guidance; staged verify gate (advisory / enforcement / required) enforced both
  before archive-tail and again before merge, as counterweight to the existing
  pre-merge archive gate.

### Unchanged (explicit)

- Adversarial classifier, conflict ask, and depth annotation → **#59**, preserved
  byte-identical.
- Pre-tool-use `plan-build-gate.sh` semantics (any active `tasks.md`).
- Recipe schema / materializer — no new config fields.
- Classic `openspec/config.yaml` decision-matrix levels.

## Settled decisions (authorized)

1. **Light minimum** → `proposal.md` + `tasks.md` (short proposal allowed).
2. **Standard minimum** → `proposal.md` + `tasks.md` + ≥1 `specs/**/*.md`.
   `explore.md` required at plan time when the criteria fire; when skipped,
   `tasks.md` records `Explore: skipped — <reason>`.
3. **Full minimum** → unchanged file set; `explore.md` stays first in the chain.
4. **Standard verify evidence** → a **dedicated `verify-report.md` is required**,
   carrying auditable command, exit status, date, and commit SHA, with a
   non-failing verdict. Evidence inside `tasks.md` or under another filename does
   not count.
5. **Full verify evidence** → strict global `PASS` **and**
   `ready_for_archive: true`, mapped to every success criterion.
6. **Explore enforcement** → skill-only at Standard **and** Full; the guardian
   never blocks on a missing `explore.md`.
7. **Verify enforcement points** → **two**: block before archive-tail, and block
   again in the pre-merge guardian. No bypass flag.
8. **Grandfathering** → applies only to plans in flight when this ships.
   Historical archives are never rewritten; stale PRs are not retro-fixed, their
   owning agent adds the missing artifacts when it resumes them.
9. **Version ownership** → `1.5.0` belongs to #59. #60 bumps to `1.6.0` and
   updates every pinned surface together.

## Baseline (verified 2026-08-07)

- Branch `change/plan-build-depth-artifacts-verify` @ `f248433`, based on
  `development` @ `12afc3f`: **1 ahead, 9 behind** current `development`
  (`604a441`). The earlier note claiming base `604a441` was stale.
- `f248433` is planning-only and touches only this change folder, so the rebase
  of the plan commit cannot conflict.
- Between `12afc3f` and `604a441` the canonical `plan-build-flow` spec grew from
  14 to 20 requirements (topology / central-artifact work). The three
  requirements this delta modifies are byte-identical across that range.
- The real merge surface is #59's `1.5.0` content, which exists only once #59
  merges — hence the rebase-first task.

## Non-Goals

- Redesigning the adversarial classifier, conflict ask, or annotation (#59).
- Making the pre-tool-use edit gate tier-aware.
- New slash commands or SDD vocabulary in user-facing briefs.
- Changing classic `openspec/config.yaml` decision_matrix levels.
- Blocking Light merges solely for missing verify evidence.
- Rewriting historical archives or existing PRs.

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Overwriting #59 classifier/annotation during the spec/SKILL merge | High if regenerated from planning-time copies | `design.md` §11.3 line-level rules + §11.4 preservation checks; #59's own tests must stay green untouched |
| Pinned brief-rule test regressed to pre-#59 rule 1 text | Med | Copy rule 1 verbatim from the rebased `recipe.toml`; §11.4 check |
| Light ceremony creep | Med | Short-proposal template ceiling (≤15 lines); Light signals kept tight |
| Verify-report shape drift across projects | Med | One canonical labelled evidence block (design §5.2) with a small synonym set |
| Full "mapped to success criteria" contract drift | Med | Guardian derives 1-based top-level criteria ordinals from proposal/design and requires exactly one strict-PASS mapping row per criterion; skill and spec publish the same format |
| Guardian complexity | Low–Med | One evidence helper + one stage flag; existing signatures and CLI unchanged |
| Version collision with #59 | Low | `1.6.0` only; sweep for leftover `1.5.0` on #60-owned surfaces |

## Rollback

Revert skill/README/brief/docs/CHANGELOG, reverse guardian + tests, restore the
canonical spec to its post-#59 state (`1.5.0` content preserved). Opt-in recipe;
consumers re-sync after revert.

## Success Criteria

- [ ] Skill §2 and guardian agree on Light/Standard/Full minima, including Light
      `proposal.md`.
- [ ] Standard explore criteria are explicit; the skip path records a one-line
      reason; no machine gate blocks on `explore.md` at any depth.
- [ ] Verify modes documented and tested: Light advisory, Standard enforcement
      (dedicated report with command/exit/date/SHA and non-failing verdict), Full
      required (strict `PASS` + `ready_for_archive: true` mapped to success
      criteria).
- [ ] Verify enforcement fires at both points: before archive-tail and again in
      the pre-merge guardian; no bypass flag exists.
- [ ] #59 surfaces preserved: the four adversarial requirements, the four
      annotation labels, the standalone-`Depth:`-line contract, brief rules 1 and
      7, and seven `workflow_rules` — proven by `design.md` §11.4 checks and by
      #59's tests passing unmodified.
- [ ] Stale canonical *PR allowed with tier minimum files* scenario reconciled to
      the new minima, with ownership unchanged.
- [ ] Recipe version is `1.6.0` on every pinned surface; `1.5.0` is not
      re-claimed and #59's CHANGELOG entry is untouched.
- [ ] Grandfathering paragraph present in README; no historical archive or stale
      PR rewritten.
- [ ] `./tests/run.sh` focused guardian/recipe tests + `./tests/validate.sh` PASS.
- [ ] Vocabulary hygiene preserved (no SDD/OpenSpec/slash `plan`|`build` leaks in
      brief/README).

## Open Questions

**None.** All prior questions are settled above and in `design.md` §2. The single
remaining prerequisite is operational, not a decision: rebase onto #59's landed
merge SHA before apply.
