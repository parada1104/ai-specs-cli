# Tasks: plan-build-depth-artifacts-verify

Depth: full

## Tracker

- card_id: `lxv2WQ5g`
- url: https://trello.com/c/lxv2WQ5g

## Notes

- Planning only until authorization. No production code in the plan commit.
- Decisions are **settled** (`proposal.md` → Settled decisions; `design.md` §2
  D1–D13). No open questions remain.
- Sibling #59 (`plan-build-depth-adversarial`) is verified PASS and lands as
  `plan-build-flow` `1.5.0`. Its classifier, conflict ask, and the four
  annotation labels are **preserved, never overwritten** — see `design.md` §6 and
  §11.
- #60 owns: artifact minima, explore guidance, staged verify gate, guardian +
  tests, matching docs/spec fragments. Nothing else.
- Version: `1.5.0` is #59's. #60 bumps to `1.6.0`.
- Baseline is stale by design-time measurement: branch `f248433` is based on
  `development` @ `12afc3f`, 1 ahead / 9 behind `604a441`. Task 0.2 fixes it.

## Phase 0 — Auth gate and rebase

- [x] **0.1** Human authorizes this plan. **Acceptance:** Explicit go-ahead
      recorded; Depth remains `full`. No unresolved question is reopened.
- [x] **0.2** Confirm #59 merged at `e2774c430f4c2a35e9e9988b803793ff046ee717`, then
      `git fetch origin && git rebase e2774c430f4c2a35e9e9988b803793ff046ee717`.
      **Acceptance:** Branch is 0 behind that SHA; the planning commit rebases
      without conflict (it touches only
      `openspec/changes/plan-build-depth-artifacts-verify/**`); rebased tree
      contains `plan-build-flow` `1.5.0` and the four #59 requirements in
      `openspec/specs/plan-build-flow/spec.md`.
- [x] **0.3** Capture the post-rebase baseline of the three shared surfaces
      (canonical spec, SKILL.md, recipe.toml) and work only from it.
      **Acceptance:** Pre-#59 copies captured during planning are not used as
      an edit base anywhere in Phase 1–3.

## Phase 1 — Guardian minima + staged verify (TDD)

- [x] **1.1** RED: extend `tests/test_premerge_guardian.py` per `design.md` §9
      cases 1–7 — Light requires `proposal.md`; Standard requires `proposal.md`;
      Light missing evidence still OK; Standard missing `verify-report.md`
      blocked; Standard evidence only inside `tasks.md` blocked; Standard report
      missing `Exit`/`Date`/`Commit` or `Exit: 1` blocked; Full without `PASS` or
      without `ready_for_archive: true` blocked; Full conforming OK; Full without
      `explore.md` OK. **Acceptance:** New tests fail against current guardian.
- [x] **1.2** GREEN: implement `check_verify_evidence` and updated minima in
      `lib/_internal/premerge_guardian.py` per design §3.1 and §5.2/§5.4.
      **Acceptance:** Focused guardian tests pass; `DEPTH_RE` unchanged.
- [x] **1.3** RED→GREEN: add `check_prearchive` and CLI `--stage
      {pre-merge,pre-archive}` (default `pre-merge`) so enforcement fires before
      archive-tail as well (D12). **Acceptance:** `--stage pre-archive` enforces
      minima + evidence on the active folder without the "still active" blocker;
      every pre-existing invocation and signature still works; no bypass flag was
      added.
- [x] **1.4** TRIANGULATE: tier inference from the `Depth:` line, explicit
      `--tier`, unknown tier fallback, failing/qualified verdict strings, and a
      run whose sibling archives are non-conforming (only the slug under check).
      **Acceptance:** Cases green.

## Phase 2 — Skill / recipe surface

- [x] **2.1** Update `catalog/recipes/plan-build-flow/skills/plan-build-flow/SKILL.md`
      §2 tier table (chain + minimum columns), the trivial-fix row, the explore
      criteria block, the verify-gate subsection, the PR-gate line, and the
      guardian blocker list — editing in place per `design.md` §11.3.
      **Acceptance:** Matches the delta spec; the #59 adversarial block
      (explicit-depth phrasings, conflict ask, four annotation labels, deeper-tier
      rule) is byte-identical to the rebased baseline.
- [x] **2.2** `recipe.toml`: bump `version` to `1.6.0` and extend brief rules 3
      and 5 only. **Acceptance:** Seven `workflow_rules` in order; rule 1 and
      rule 7 unchanged; rule 6 keeps exactly one `{config.artifact_store_default}`;
      `tasks-only` still present in the rules.
- [x] **2.3** Update `catalog/recipes/plan-build-flow/README.md` (minima table,
      verify staging, grandfathering paragraph, version example) and
      `docs/recipes-catalog.md` enable example. **Acceptance:** Vocabulary tests
      pass; both show `1.6.0`.
- [x] **2.4** RED/GREEN `tests/test_plan_build_flow_recipe.py`: new skill markers
      (Light proposal, explore criteria, advisory/enforcement/required), brief
      rules 3/5 pins, and version pins `1.5.0` → `1.6.0`. **Acceptance:** Focused
      recipe tests pass; rule 1's pinned text was copied verbatim from the
      **rebased** `recipe.toml`, not from the pre-#59 baseline.
- [x] **2.5** CHANGELOG: add one `[Unreleased]` entry for `1.5.0` → `1.6.0`.
      **Acceptance:** #59's entry is untouched; no historical release entry
      rewritten.

## Phase 3 — Spec promotion + reconciliation

- [x] **3.1** Promote
      `openspec/changes/plan-build-depth-artifacts-verify/specs/plan-build-flow/spec.md`
      into canonical `openspec/specs/plan-build-flow/spec.md` following
      `design.md` §11.3. **Acceptance:** Canonical spec carries *Depth artifact
      minima*, *Standard explore enforcement criteria*, *Staged verify gate*,
      the reconciled *PR artifact gate* and *Pre-merge merge guardian*; the
      classifier requirement differs from its post-#59 text only in the three
      chain bullets, the Light `THEN` line, and the added pointer sentence.
- [x] **3.2** Reconcile the stale canonical scenario *PR allowed with tier minimum
      files* to the new Standard minima and add the Light-without-proposal
      scenario. **Acceptance:** No canonical scenario still implies Standard is
      satisfied by `tasks.md` + specs alone; the PR gate keeps its own
      requirement, so #60 ownership is unchanged.
- [x] **3.3** Run the `design.md` §11.4 preservation checks. **Acceptance:** All
      four annotation labels and the four #59 requirements present; brief has
      seven rules with topology last; #59 assertions pass unmodified; no `1.5.0`
      remains on #60-owned surfaces while the `worktree-flow` example in
      `docs/recipes-catalog.md` keeps its version.
- [x] **3.4** README grandfathering paragraph per D13. **Acceptance:** States
      in-flight plans add the missing `proposal.md` / evidence block, historical
      archives are never rewritten, and stale PRs are handled by their owning
      agent on resume.

## Phase 4 — Verify this change

- [x] **4.1** Run focused guardian + `plan-build-flow` recipe tests.
      **Acceptance:** PASS.
- [x] **4.2** Run `./tests/validate.sh`. **Acceptance:** exit 0.
- [x] **4.3** Write `verify-report.md` satisfying this change's own Full contract:
      the `design.md` §5.2 evidence block with `Verdict: PASS`, command, `Exit: 0`,
      date, commit SHA, and `ready_for_archive: true`, mapped to every success
      criterion in `proposal.md`. **Acceptance:** Report conforms to the gate it
      introduces and the guardian accepts it at `--tier full`.

## Phase 5 — Ship hygiene

- [x] **5.1** Re-confirm no divergence from #59 on shared surfaces immediately
      before PR (re-run §11.4). **Acceptance:** Clean.
- [x] **5.2** PR only after planning + implementation commits exist; verify gate
      before archive-tail; archive-tail on the review branch; guardian
      `--stage pre-merge` OK before merge. **Acceptance:** Both enforcement points
      exercised on this change itself.
      **Archive-tail executed 2026-08-07** by the owning review workflow: the
      canonical sync was completed (see `sync-report.md`), the change folder was
      moved to `openspec/changes/archive/plan-build-depth-artifacts-verify/`,
      and the pre-merge guardian passed on the archived tree (`--tier full
      --stage pre-merge` → OK). Stale-checkbox reconciliation: this task was the
      pending archive-tail work item; `apply-progress.md` (20/21, only 5.2 open),
      `verify-report.md` (PASS), and this archive run prove completion.
- [x] **5.3** The recipe bump does not require a generated brief refresh for this
      repo's dogfood project; no dogfood sync churn was mixed into the product
      changes. **Acceptance:** Explicitly recorded as not required.

## Non-goals (task level)

- Adversarial depth conflict UX, annotation, or brief rule 1 (#59).
- Tier-aware `hooks/plan-build-gate.sh`.
- New recipe schema / materializer fields.
- Rewriting historical archives or existing PRs.
