# Tasks: relocate skill-frontmatter contract alongside skill-creator

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 120–250 (rename of ~101-line contract + 3 reference edits; git may count rename as delete+add ≈ 200) |
| Session review budget | 900 lines |
| 400-line budget risk | Low |
| Chained PRs recommended | No |
| Suggested split | single PR |
| Delivery strategy | single-pr |
| Chain strategy | size-exception (N/A — under budget; single PR) |

```text
Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: size-exception
400-line budget risk: Low
```

Notes:
- Option A baselines (D1–D5) from proposal/design are the accepted working baseline;
  final maintainer gate still applies before apply if anything drifts.
- No executable code, distribution machinery, doctor checks, or new project
  scaffolding. Pure asset relocate + reference rewires + empty-dir cleanup.
- Spec delta already lands the MODIFIED GIVEN path under
  `openspec/changes/relocate-skill-frontmatter-contract/specs/skill-frontmatter-contract/spec.md`;
  live `openspec/specs/...` promotion is archive-time (not apply).

## Planning depth

- **Classification**: domain_change (proposal → design → spec → tasks). Same
  tier as sibling `2026-07-24-relocate-bundled-commands`: deferred placement fix
  in the skill/contracts domain, low architectural ambiguity, design already
  resolved to Option A.
- **Authorization**: working baseline = Option A (D1–D5). No open design forks
  remain for apply. Do not expand into doctor checks, new distribution tiers,
  or per-project `ai-specs/contracts/` scaffolding (explicitly rejected).
- **Strict TDD**: `strict_tdd: true` in `openspec/config.yaml`, but this change
  has **no production code surface** and no path assertions in existing tests
  (`tests/test_skill_contract.py`, `tests/test_manifest_contract_docs.py`).
  Sequence is confirm-no-RED-needed → implement move/refs → validate suite
  still green (triangulate via link-resolution + grep hygiene).

## Implementation

### Phase 1 — Relocate canonical contract (git mv)

- [x] 1.1 Confirm preconditions: `ai-specs/contracts/skill-frontmatter.md` is
      the sole entry under `ai-specs/contracts/`; destination
      `bundled-skills/skill-creator/assets/skill-frontmatter-contract.md` does
      not yet exist; `bundled-skills/skill-creator/assets/SKILL-TEMPLATE.md`
      already present.
- [x] 1.2 GREEN: `git mv ai-specs/contracts/skill-frontmatter.md
      bundled-skills/skill-creator/assets/skill-frontmatter-contract.md`
      (basename rename is intentional per design — self-describing asset next
      to `SKILL-TEMPLATE.md`). Content body unchanged.
- [x] 1.3 GREEN: `rmdir ai-specs/contracts` if the empty directory lingers in
      the working tree (git drops the tracked path via the mv alone; no
      `.gitkeep`, no init/sync recreate path).

### Phase 2 — Rewire skill-creator references

Exact before/after from `design.md` (paths relative to each file's directory):

- [x] 2.1 GREEN (`bundled-skills/skill-creator/SKILL.md` ~L69): replace
      ```
      Canonical reference: [`../../contracts/skill-frontmatter.md`](../../contracts/skill-frontmatter.md).
      ```
      with
      ```
      Canonical reference: [`assets/skill-frontmatter-contract.md`](assets/skill-frontmatter-contract.md).
      ```
- [x] 2.2 GREEN (`bundled-skills/skill-creator/SKILL.md` ~L85): replace
      ```
      - [ ] Frontmatter matches `ai-specs/contracts/skill-frontmatter.md`
      ```
      with
      ```
      - [ ] Frontmatter matches `assets/skill-frontmatter-contract.md`
      ```
- [x] 2.3 GREEN (`bundled-skills/skill-creator/assets/SKILL-TEMPLATE.md` ~L42):
      replace
      ```
      - **Contract**: [../../contracts/skill-frontmatter.md](../../contracts/skill-frontmatter.md)
      ```
      with
      ```
      - **Contract**: [skill-frontmatter-contract.md](skill-frontmatter-contract.md)
      ```
      (same-directory basename; template portability caveat accepted in design —
      out of scope to fix generated-skill link rewrite).

### Phase 3 — Confirm parser / tests path-agnostic (no code change)

- [x] 3.1 Verify `lib/_internal/skill_contract.py` has zero matches for
      `contracts/`, `skill-frontmatter.md`, and `ai-specs/contracts` (path-
      agnostic parser; **no edit expected**).
- [x] 3.2 Verify `tests/test_skill_contract.py` and
      `tests/test_manifest_contract_docs.py` do not assert on the contract
      document path (**no test edits expected**).
- [x] 3.3 Verify `docs/`, `AGENTS.md`, `templates/`, `README.md` have no
      stale `contracts/skill-frontmatter` citations (**no doc edits expected**
      per proposal grep; skip CHANGELOG unless apply surfaces a user-facing
      path string that already documents the old location — design does not
      require CHANGELOG).

### Phase 4 — Link resolution + hygiene verification

- [x] 4.1 From `bundled-skills/skill-creator/`, confirm
      `assets/skill-frontmatter-contract.md` exists and is readable (covers
      SKILL.md L69/L85 targets). Example:
      `test -f bundled-skills/skill-creator/assets/skill-frontmatter-contract.md`.
- [x] 4.2 From `bundled-skills/skill-creator/assets/`, confirm
      `skill-frontmatter-contract.md` resolves as a same-dir sibling of
      `SKILL-TEMPLATE.md`.
- [x] 4.3 Grep hygiene (non-archive):
      `grep -rn "contracts/skill-frontmatter" bundled-skills/ lib/ docs/
      AGENTS.md templates/ tests/` → **zero** hits.
- [x] 4.4 Confirm no file under `openspec/changes/archive/` was modified
      (historical path citations stay as audit trail).
- [x] 4.5 Confirm `ai-specs/contracts/` is absent (no file, no empty dir left
      committed).
- [x] 4.6 Spec delta already correct: change-local
      `specs/skill-frontmatter-contract/spec.md` GIVEN cites
      `bundled-skills/skill-creator/assets/skill-frontmatter-contract.md`
      (MODIFIED scenario "Contract document describes required and generated
      fields"). Do **not** re-edit the delta during apply.
- [ ] 4.7 Optional smoke: after a local bundled-skill refresh path is available,
      confirm the asset appears under
      `{cache}/.bundled/skills/skill-creator/assets/skill-frontmatter-contract.md`
      so consumer resolution matches design data-flow (not required if refresh
      harness is awkward in the worktree; link checks 4.1–4.2 are sufficient
      for the relocate itself).

### Phase 5 — Validation suite

- [x] 5.1 `./tests/run.sh tests.test_skill_contract tests.test_manifest_contract_docs`
      (or equivalent focused discovery) still green — triangulation that the
      move did not disturb enforcement.
- [x] 5.2 `./tests/validate.sh` exit 0 (py_compile, bash -n, full unittest).
- [x] 5.3 Write `verify-report.md` at verify phase comparing against every
      scenario in
      `openspec/changes/relocate-skill-frontmatter-contract/specs/skill-frontmatter-contract/spec.md`
      (both MODIFIED scenarios: contract path GIVEN + generated-files-are-derived).

## Archive / follow-up (not apply)

- [x] 6.1 At archive: promote the spec delta into live
      `openspec/specs/skill-frontmatter-contract/spec.md` (replace GIVEN
      `ai-specs/contracts/skill-frontmatter.md` with
      `bundled-skills/skill-creator/assets/skill-frontmatter-contract.md`).
- [x] 6.2 No doctor WARN, no init scaffolding, no consumer leftover migration
      (D4 Option A) — explicitly out of scope forever for this change.

## Rollback (apply-time)

If apply must abort after partial edits:
1. `git mv` the asset back to `ai-specs/contracts/skill-frontmatter.md`.
2. Restore the three reference strings in `SKILL.md` (×2) and
   `SKILL-TEMPLATE.md` (×1).
3. No cache/lock/doctor state to unwind.

## Out of scope (do not implement)

- New `bundled-contracts/` distribution tier (D1-B).
- Pointing skill-creator at `openspec/specs/...` only (D2-B).
- Keeping a dogfood duplicate or symlink under `ai-specs/contracts/` (D3-B/C).
- Doctor leftover detection for hand-copied consumer contracts (D4-B).
- Rewriting archived OpenSpec artifacts.
- Any edit to `lib/_internal/skill_contract.py` or its unit tests unless
  Phase 3 unexpectedly finds a hardcoded path (then stop and re-open design).
