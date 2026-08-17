# Proposal: honor dated OpenSpec archives in the pre-merge guardian

## Tracker

- issue: `#216`
- card_id: `#79`
- url: https://trello.com/c/mOgt0uq6/79-fix-pre-merge-guardian-accepts-dated-openspec-archives

## Why

The pre-merge guardian blocks valid OpenSpec changes because it only checks the
legacy undated path `openspec/changes/archive/<slug>/`. The OpenSpec provider's
canonical archive path is `openspec/changes/archive/YYYY-MM-DD-<slug>/`, which
is already used by recent archive operations. Card #74 exposed the mismatch;
issue #216 tracks the follow-up after that delivery was intentionally completed
with the incompatible guardian bypassed.

The live `plan-build-flow` skill, recipe guidance, and canonical spec also
describe the undated path. The implementation and its documentation therefore
need one consistent provider contract.

## Outcome

Make the guardian resolve the canonical dated OpenSpec archive while preserving
exact undated archives as legacy compatibility. The resolver will never guess
among multiple candidates or accept malformed/near-match names. Existing active
folder, artifact-minimum, verification, tier, and planning-root gates remain
unchanged after a path is resolved.

## What changes

1. Update `lib/_internal/premerge_guardian.py` with a small, deterministic
   archive resolver used by `check_premerge`.
2. Recognize exactly `YYYY-MM-DD-<slug>` after validating both the ISO shape and
   the calendar date; use exact `<slug>` only as a legacy fallback.
3. Fail closed for multiple dated candidates, dated-plus-undated candidates,
   invalid date prefixes, and candidate-shaped near-match names. Do not use
   directory ordering or substring matching to select an archive.
4. Keep the existing inspection and evidence checks unchanged and run them
   against the resolved archive path.
5. Update the live plan-build-flow skill, recipe brief rule, README, and recipe
   catalog documentation to state the dated provider path, legacy fallback, and
   fail-closed ambiguity policy.
6. Add the corresponding normative delta under
   `specs/plan-build-flow/spec.md`; canonical promotion occurs through the
   normal OpenSpec archive flow.

## Existing regression context

The worktree already contains a pre-existing change to
`tests/test_premerge_guardian.py` covering dated pass, legacy fallback, dated
ambiguity, dated-plus-undated ambiguity, invalid calendar date, and active-folder
blocking. This planning pass does not modify that file or claim any RED/GREEN
result. Authorized apply must use the provided change as the RED input and record
the observed command output before implementing the smallest production fix.

## Affected surfaces

| Path | Planned action |
|---|---|
| `lib/_internal/premerge_guardian.py` | Implement exact dated/legacy archive resolution and fail-closed blockers |
| `tests/test_premerge_guardian.py` | Existing staged regression input only; untouched by this planning pass |
| `catalog/recipes/plan-build-flow/skills/plan-build-flow/SKILL.md` | Replace undated archive examples and document provider resolution rules |
| `catalog/recipes/plan-build-flow/recipe.toml` | Make the generated workflow rule name the dated archive contract |
| `catalog/recipes/plan-build-flow/README.md` | Document canonical dated path and compatibility/ambiguity behavior |
| `docs/recipes-catalog.md` | Keep the catalog-facing plan-build description aligned with the live contract |
| `openspec/specs/plan-build-flow/spec.md` | Promote the dated archive and guardian requirements through the change delta |
| `openspec/changes/archive/**` | Do not modify historical archive directories |

## Compatibility and non-goals

- Exact undated `<slug>` archives remain readable for historical compatibility.
- Canonical dated archives are preferred when they are the only valid candidate.
- Existing artifact minima and Standard/Full verification gates are not relaxed,
  reordered, or redesigned.
- The active-folder blocker remains independent of archive resolution.
- No archive directory is renamed or rewritten.
- No new CLI flag, provider abstraction, recipe configuration key, or archive
  migration is introduced.
- No test result is invented during planning, and no staged test change is
  rewritten as part of this artifact package.

## Success criteria

- A single valid `YYYY-MM-DD-<slug>` archive passes and is exposed as
  `GuardianResult.archive_path`.
- A single exact undated `<slug>` archive passes as a legacy fallback.
- Multiple dated archives and dated-plus-undated archives fail with an
  ambiguity blocker that identifies the candidates.
- Invalid calendar-date prefixes and near-match names are rejected rather than
  silently treated as missing or selected archives.
- A valid dated archive does not bypass the existing active-folder blocker.
- Existing tier-minimum and verification behavior remains unchanged.
- The live plan-build-flow recipe/docs and canonical spec all state the same
  dated provider contract.
- Historical archive directories remain unchanged.
- `./tests/validate.sh` passes after implementation and documentation updates.

## Rollback

Revert the resolver and the live contract documentation/spec delta together.
Because no historical archive is modified and no persisted format changes, the
rollback has no data migration step.
