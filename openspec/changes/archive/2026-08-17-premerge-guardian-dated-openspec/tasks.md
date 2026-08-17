# Tasks: premerge-guardian-dated-openspec

Depth: standard

Explore: required - live plan-build-flow documentation conflicts with the canonical dated OpenSpec archive contract, and this is a retry/follow-up for issue #216.

## Tracker

- issue: `#216`
- card_id: `#79`
- url: https://trello.com/c/mOgt0uq6/79-fix-pre-merge-guardian-accepts-dated-openspec-archives

## Apply status

Authorized implementation in progress. Do not commit, push, open a PR, merge,
archive this active change, or modify historical directories under
`openspec/changes/archive/**`. Preserve unrelated worktree changes and keep the
existing staged regression intent intact.

## Review workload estimate

| Item | Estimate |
|---|---|
| Implementation and contract surfaces | One Python helper change plus four live/spec documentation surfaces |
| Focused behavior cases | 7 path-resolution/guard cases, with the staged RED change as input |
| Expected review size | Approximately 180-300 changed lines, under the 400-line split threshold |
| Review topology | One reviewable PR; no chained PR recommended |
| Verification cost | One focused guardian run plus `./tests/validate.sh` |

## TDD sequence

- [x] **1. RED (provided regression change)**: Inspect the existing regression
      change and run the configured focused suite (`./tests/run.sh`) before touching
      `lib/_internal/premerge_guardian.py`. Record the observed failing cases in
      apply evidence; do not invent a failure result in this planning package and
      do not rewrite the pre-existing test file.
- [x] **2. GREEN**: Implement the smallest exact archive resolver in
      `lib/_internal/premerge_guardian.py`. Validate ISO calendar dates with the
      existing date utilities, match the exact dated and legacy names only,
      reject invalid/near-match candidate names, and fail closed for multiple
      dated or dated-plus-undated candidates. Preserve the existing
      `GuardianResult` shape, CLI arguments, active-folder blocker, tier minima,
      verification evidence, and planning-root behavior.
- [x] **3. Regression and docs**: Re-run the staged guardian cases for dated
      pass, legacy fallback, multiple-dated ambiguity, dated-plus-undated
      ambiguity, invalid date, and active-folder blocking. Add no unrelated
      test edits. Update the live plan-build-flow skill, `recipe.toml` brief
      rule, recipe README, `docs/recipes-catalog.md`, and the canonical-spec
      delta so every surface names the dated provider contract and legacy
      compatibility without rewriting historical archives.
- [x] **4. Full validation**: Run `./tests/validate.sh` from the worktree root,
      verify the archive subtree is unchanged, and record the exact command,
      exit status, date, and revision in the implementation/verification
      evidence. Confirm no artifact-minimum or verification behavior regressed.
      The first run exposed two stale recipe-surface assertions, which were
      corrected. The final run passed with 1678 tests and 116 skips.

## Implementation detail tasks

- [x] **5. Candidate resolution**: Inspect only direct archive child
      directories for the requested slug. Prefer one exact valid
      `YYYY-MM-DD-<slug>` candidate; use exact `<slug>` only when no dated
      candidate exists; return explicit blockers for zero, ambiguous, invalid,
      or near-match candidates. Include candidate names in ambiguity errors.
- [x] **6. Contract documentation**: Replace undated path statements in
      `catalog/recipes/plan-build-flow/skills/plan-build-flow/SKILL.md`; state
      dated archive-tail behavior, legacy fallback, and fail-closed ambiguity
      in the recipe README and catalog docs; update the generated workflow rule
      source in `recipe.toml`.
- [x] **7. Spec promotion readiness**: Keep the delta in
      `specs/plan-build-flow/spec.md` limited to archive path resolution and
      preserve all existing artifact, verification, root, and active-folder
      requirements. The normal archive flow may later promote the delta to
      `openspec/specs/plan-build-flow/spec.md`; no historical archive directory
      may be changed.

## Acceptance checklist

- [x] A single valid dated archive passes and `archive_path` points to it.
- [x] An exact undated archive passes only as the legacy fallback.
- [x] Multiple dated candidates and dated-plus-undated candidates fail closed
      with named ambiguity blockers.
- [x] Invalid calendar-date prefixes and near-match names are rejected.
- [x] A valid dated archive does not bypass the active-folder blocker.
- [x] Existing tier-minimum and Standard/Full verification checks are unchanged.
- [x] Live recipe/docs and the canonical spec agree on the provider contract.
- [x] Historical archives remain untouched.
- [x] `./tests/validate.sh` passes after authorized implementation: 1678 tests
      passed and 116 were skipped on the final run.
